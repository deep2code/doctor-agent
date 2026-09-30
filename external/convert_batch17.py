#!/usr/bin/env python3
"""Batch 17 converters: 中华医学会科普 / 数字科技馆流言榜 / 康复医学会 / 医师报 / yiigle 指南续抓.

All raw dirs share the established 3-line header format
(`# URL` / `# 标题` / `# 发布方/日期`, blank line, then body paragraphs).

Outputs land in internal/knowledge/data/*.json and are classified into the
single `medical` dataset by the seed.go DSMedical branch — zero retrieval-layer
change. Pure text parsing, no LLM calls.

Keyword policy (magnet rule from batches 4-6): keywords are [title, condition]
only. A generic functional word (多久/怎么办/检查/孩子…) pays +3 through rule 2b
on every query that contains it, so it would steal other families' recall.

Citation policy: `title` MUST be set (verify-knowledge errors without it) and
`journal` stays empty (these sources have no DOI/PMID by design).
"""

import html
import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
RAW = ROOT / "external"
DATA = ROOT / "internal" / "knowledge" / "data"

GUIDE_RE = re.compile(r"(诊疗指南|防治指南|管理指南|技术规范|指导原则|专家共识|共识|指南|规范|标准)")
# yiigle pages publish "<指南全名> - 中华XX杂志"; the journal suffix is provenance,
# not part of the title, and leaving it in poisons condition_zh.
JOURNAL_SUFFIX_RE = re.compile(r"\s*[-–—]\s*(中华|中国|国际|实用|临床|北大|协和)?.{0,10}(杂志|学报|期刊)\s*$")
# 2020 疫情-era prose the site re-stamps with a 2026 publish date; a patient-facing
# assistant in 2026 should not surface 「刷疫情消息刷到焦虑」 as current advice.
PANDEMIC_RE = re.compile(r"(疫情消息|封控|核酸检测|武汉|方舱|戴口罩就医)")

# (dir, id_prefix, category, out_name, evidence, reject_title_re, drop_url_year_before)
SOURCES = [
    ("cma_health", "cmap", "society_popular", "cma_society_popular.json", "official_guideline", None, None),
    ("carm_rehab", "kf", "rehab_medicine", "rehab_society.json", "official_guideline", None, None),
    ("mdweekly", "mw", "health_newspaper", "mdweekly_popular.json", "official_guideline", None, None),
]
# cdstm_health is NOT in SOURCES: its pages are monthly 流言榜 bundles, converted
# per-rumor by convert_rumor_boards() below (a month-titled entry recalls nothing).

# yiigle continuation: only files not already converted in batch 14 (ycg-*).
YIIGLE_DIR = RAW / "yiigle_guides" / "raw"
YIIGLE_PREV = DATA / "yiigle_clinical_guides.json"
YIIGLE_OUT = DATA / "yiigle_guides_more.json"
YIIGLE_PREFIX = "yg2"


def raw_dir(sub):
    d = RAW / sub / "raw"
    return d if d.is_dir() else RAW / sub


def parse_header(lines):
    url = title = publisher = year = ""
    for line in lines[:4]:
        s = line.strip()
        if s.startswith("# URL:"):
            url = s.split(":", 1)[1].strip()
        elif s.startswith("# 标题:"):
            title = html.unescape(s.split(":", 1)[1].strip())
        elif s.startswith("# 发布方/日期:"):
            rest = html.unescape(s.split(":", 1)[1].strip())
            publisher = re.split(r"[|，,]", rest)[0].strip()
            m = re.search(r"(20\d{2})", rest)
            year = m.group(1) if m else ""
    return url, title, publisher, year


def extract_body(lines):
    out, seen = [], False
    for line in lines:
        if line.startswith("#"):
            continue
        if not seen:
            if line.strip() == "":
                seen = True
            continue
        out.append(line)
    body = "\n".join(out).strip()
    body = re.sub(r"^□\s*[^\n]*\n", "", body)
    # strip trailing site widgets the fetchers cannot always cut
    body = re.sub(r"\n(文件附件|相关新闻|责任编辑|来源网站|扫一扫|关注我们|分享到).*", "", body, flags=re.S)
    return body


def extract_condition(title):
    c = re.sub(r"[！!？?＞><]+", "", title)
    c = re.sub(r"【[^】]*】|（[^）]*）|\([^)]*\)", "", c)
    c = re.sub(r"^\s*(科普|健康科普|疾病科普|辟谣|流言榜|科学辟谣)[|\s·|：:]+", "", c)
    c = c.split("——")[0].split("：")[0]
    c = c.strip(" 　，。、：:「」\"'")
    return c[:28] if c else title[:28]


def guide_condition(title):
    """Guideline titles are long; the topical core (disease phrase) is what earns
    the +5.0 containment hit, so strip the 指南/共识 framing words."""
    c = re.sub(r"（[^）]*）|\([^)]*\)", "", title)
    c = c.split("-")[0]
    c = re.sub(r"杂志|学报|期刊|全科医师|内科|外科|科和|专业", "", c)
    c = re.sub(r"中国|中华|成人|儿童|患者|人群|基层|全国|实用|专家|推荐|相关", "", c)
    c = GUIDE_RE.sub("", c)
    c = re.sub(r"[\d\s第版年月日要点指南，、。的与和及]", "", c)
    return c[:20] if len(c) >= 4 else re.sub(r"（[^）]*）|\([^)]*\)", "", title)[:20]


def entry(stem, title, condition, body, url, year, prefix, category, evidence,
          cond_is_guide=False, citation_title=None):
    return {
        "id": f"{prefix}-{stem}",
        "title_zh": title,
        "condition_zh": condition,
        "summary_zh": body[:500],
        "details_zh": body,
        "keywords": [k for k in dict.fromkeys([title, condition]) if k],
        "category": category,
        "evidence": evidence,
        "citations": [{
            "type": "national_guideline" if cond_is_guide else "national_report",
            "title": citation_title or title,
            "journal": "",
            "year": int(year) if year else 2025,
            "url": url,
            "level": "official_guideline",
        }],
    }


def convert_dir(sub, prefix, category, evidence, min_body=150, reject=None, drop_url_year_before=None):
    d = raw_dir(sub)
    if not d.is_dir():
        return [], f"missing {d}"
    entries, skipped, nav_skipped = [], 0, 0
    for fp in sorted(d.glob("*.txt")):
        if fp.stem == "MANIFEST":
            continue
        lines = fp.read_text(encoding="utf-8").split("\n")
        url, title, _pub, year = parse_header(lines)
        body = extract_body(lines)
        # A 数字报 index page (masthead + 「标题导航」 headline list) is long enough to
        # clear the 400-rune bar while containing none of the article, so a fetcher that
        # ever lands on that layout must drop the entry loudly instead of shipping nav
        # text as medical content.
        if "标题导航" in body:
            nav_skipped += 1
            continue
        if not title or len(re.sub(r"[^\u4e00-\u9fff]", "", body)) < 400 or len(body) < min_body:
            skipped += 1
            continue
        if reject and reject.search(title):
            skipped += 1
            continue
        if drop_url_year_before:
            m = re.search(r"/art/(20\d{2})/", url)
            if m and int(m.group(1)) < drop_url_year_before:
                skipped += 1
                continue
        entries.append(entry(fp.stem, title, extract_condition(title), body, url, year,
                             prefix, category, evidence))
    if nav_skipped:
        print(f"  WARN {sub}: {nav_skipped} 数字报 index pages carry no article body -> skipped",
              file=sys.stderr)
    return entries, skipped


def convert_rumor_boards(min_cjk=120):
    """每月「科学」流言榜 pages bundle 6-10 rumors each; a month-titled entry is
    unrecallable for any real question, so split on the numbered item lines and
    key each rumor by its own claim."""
    d = raw_dir("cdstm_health")
    if not d.is_dir():
        return [], "missing cdstm raw"
    item_re = re.compile(r"(?m)^\s*(\d{1,2})\s*[\.、．]\s*(\S[^\n]{4,60})\s*$")
    entries, skipped = [], 0
    for fp in sorted(d.glob("*.txt")):
        if fp.stem == "MANIFEST":
            continue
        lines = fp.read_text(encoding="utf-8").split("\n")
        url, title, _pub, year = parse_header(lines)
        body = extract_body(lines)
        if not url or "/art/20" in url and re.search(r"/art/20(1[0-9]|2[01])/", url):
            skipped += 1  # stale pandemic-era frontier pages, not rumor boards
            continue
        marks = list(item_re.finditer(body))
        if len(marks) < 3:
            skipped += 1
            continue
        for i, m in enumerate(marks):
            claim = html.unescape(m.group(2).strip().rstrip("：:，,。"))
            end = marks[i + 1].start() if i + 1 < len(marks) else len(body)
            text = body[m.end():end].strip()
            if PANDEMIC_RE.search(claim) or PANDEMIC_RE.search(text[:120]):
                skipped += 1  # stale pandemic-era rumor re-stamped with a current date
                continue
            if len(re.sub(r"[^\u4e00-\u9fff]", "", text)) < min_cjk:
                skipped += 1
                continue
            entries.append(entry(
                f"{fp.stem}-{i + 1}", f"{claim}（{title}）", claim, text, url, year,
                "dsc", "rumor_debunking", "official_statement", citation_title=f"{title} - {claim}",
            ))
    return entries, skipped


def convert_yiigle_more():
    if not YIIGLE_DIR.is_dir():
        return [], "missing yiigle raw"
    prev = {e["id"].split("-", 1)[1] for e in json.loads(YIIGLE_PREV.read_text(encoding="utf-8"))}
    entries, skipped = [], 0
    for fp in sorted(YIIGLE_DIR.glob("*.txt")):
        if fp.stem == "MANIFEST" or fp.stem in prev:
            continue
        lines = fp.read_text(encoding="utf-8").split("\n")
        url, title, _pub, year = parse_header(lines)
        title = JOURNAL_SUFFIX_RE.sub("", title).strip()
        body = extract_body(lines)
        if not title or len(body) < 1500:
            skipped += 1
            continue
        entries.append(entry(fp.stem, title, guide_condition(title), body, url, year,
                             YIIGLE_PREFIX, "clinical_guide", "national_guideline", cond_is_guide=True))
    return entries, skipped


def write(out_name, entries):
    path = DATA / out_name
    path.write_text(json.dumps(entries, ensure_ascii=False, indent=2), encoding="utf-8")
    return path


def main():
    DATA.mkdir(parents=True, exist_ok=True)
    report = []
    for sub, prefix, category, out_name, evidence, reject, stale_year in SOURCES:
        entries, skipped = convert_dir(sub, prefix, category, evidence, reject=reject,
                                       drop_url_year_before=stale_year)
        if not entries:
            report.append(f"{sub}: nothing to convert ({skipped})")
            continue
        write(out_name, entries)
        report.append(f"{sub}: {len(entries)} entries -> {out_name} (skipped {skipped})")
    entries, boards_skipped = convert_rumor_boards()
    if entries:
        write("cdstm_rumor_board.json", entries)
        report.append(f"cdstm_rumor_board: {len(entries)} entries (skipped {boards_skipped})")
    entries, skipped = convert_yiigle_more()
    if entries:
        write("yiigle_guides_more.json", entries)
        report.append(f"yiigle_guides_more: {len(entries)} entries (skipped {skipped})")
    print("\n".join(report))
    if "--stats" in sys.argv:
        for sub, *_ in SOURCES:
            d = raw_dir(sub)
            if d.is_dir():
                print(f"  raw {sub}: {len([p for p in d.glob('*.txt') if p.stem != 'MANIFEST'])} files")


if __name__ == "__main__":
    main()
