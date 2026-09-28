#!/usr/bin/env python3
"""Convert 健康报续抓 + 省级疾控科普 articles to KnowledgeEntry JSON (batch 15).

Two sources share the header format (# URL / # 标题 / # 发布方/日期 + blank + body):
  - external/jkb_more/raw/*.txt       (健康报网 jkb.com.cn, 国家卫健委主管)
  - external/provincial_cdc/raw/*.txt (四川/贵州/湖北/湖南/天津 等省级疾控)

Outputs (dataset = medical via the DSMedical branch, zero retrieval-layer change):
  - internal/knowledge/data/jkb_more_popular.json      ids jkbm-*
  - internal/knowledge/data/provincial_cdc_health.json ids pcdc-*

Pure text parsing, no LLM. HTML entities are unescaped; 健康报 leading
「□ 单位 姓名」 author token is stripped.
"""

import html
import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
RAW = ROOT / "external"
DATA = ROOT / "internal" / "knowledge" / "data"

# (dir, id_prefix, category, out_name)
SOURCES = [
    ("jkb_more", "jkbm", "health_newspaper", "jkb_more_popular.json"),
    ("provincial_cdc", "pcdc", "provincial_cdc", "provincial_cdc_health.json"),
]


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
            publisher = re.split(r"[|·]", rest)[0].strip()
            m = re.search(r"(20\d{2})", rest)
            year = m.group(1) if m else ""
    return url, title, publisher, year


def extract_body(lines):
    out, seen_blank = [], False
    for line in lines:
        if line.startswith("#"):
            continue
        if not seen_blank:
            if line.strip() == "":
                seen_blank = True
            continue
        out.append(line)
    body = "\n".join(out).strip()
    body = re.sub(r"^□\s*[^\n]*\n", "", body)
    return body


def extract_condition(title):
    c = re.sub(r"[！!？?＞><]+", "", title)
    c = re.sub(r"【[^】]*】", "", c)
    c = re.sub(r"\(.*?\)|（.*?）", "", c)
    c = c.split("——")[0]
    c = c.strip(" 　，。、：:「」\"'")
    return c[:28] if c else title[:28]


def build_keywords(title, condition):
    # Minimal set: full title + short condition only. No generic functional
    # words (多久/怎么办/检查…) — magnet rule established in prior batches.
    return [k for k in dict.fromkeys([title, condition]) if k]


def convert_dir(raw_dir, prefix, category):
    entries, skipped = [], 0
    for filepath in sorted(Path(raw_dir).glob("*.txt")):
        if filepath.stem == "MANIFEST":
            continue
        lines = filepath.read_text(encoding="utf-8").split("\n")
        url, title, publisher, year = parse_header(lines)
        if not title:
            skipped += 1
            continue
        body = extract_body(lines)
        if len(body) < 150:
            skipped += 1
            continue
        condition = extract_condition(title)
        entries.append({
            "id": f"{prefix}-{filepath.stem}",
            "title_zh": title,
            "condition_zh": condition,
            "summary_zh": body[:500],
            "details_zh": body,
            "keywords": build_keywords(title, condition),
            "category": category,
            "evidence": "official_guideline",
            "citations": [{
                "type": "national_report",
                "journal": "",
                "year": int(year) if year else 2025,
                "url": url,
                "level": "official_guideline",
            }],
        })
    return entries, skipped


def main():
    DATA.mkdir(parents=True, exist_ok=True)
    for sub, prefix, category, out_name in SOURCES:
        raw_dir = RAW / sub / "raw"
        if not raw_dir.is_dir():
            raw_dir = RAW / sub
        entries, skipped = convert_dir(raw_dir, prefix, category)
        out = DATA / out_name
        out.write_text(json.dumps(entries, ensure_ascii=False, indent=2), encoding="utf-8")
        print(f"{sub}: {len(entries)} entries ({skipped} skipped) -> {out_name}")


if __name__ == "__main__":
    main()
