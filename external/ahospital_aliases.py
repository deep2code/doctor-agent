"""Harvest 别名↔正式名 pairs from the A+医学百科 crawl — vocabulary only, never body text.

Why this is the usable product of that site: a MediaWiki redirect page is served as the
canonical article with the line 「（重定向自 <source>）」, so one fetched page yields a
synonym pair (source -> canonical) without a single character of prose entering the
knowledge base. That pair feeds `internal/knowledge/alias_map.json`, which `ExpandQuery`
consumes and which never places text into an answer or a citation — the same role
`convert_medical_terminology.py` already plays.

Direction (verified against the raw HTML, not assumed): the saved file name and the anchor
text are the *requested* title A; the served `<h1>` is the redirect *target* B.

Candidate quality gates, in order:
  1. drop institution/award/taxonomy titles (a page about 妇幼保健院 is not a medical synonym);
  2. drop pairs whose two sides normalize to each other (pure spelling variants carry no recall);
  3. both sides carry at least the alias_map house minimum (>=2 CJK runes), because the map is
     written bidirectionally and every accepted pair becomes two ExpandQuery keys; the candidate
     file records `min_runes` and is sorted by it so short keys are reviewed first;
  4. annotate which side already exists in our official name tables, so a human can see what
     the pair actually buys.

This script deliberately does NOT write alias_map.json. Pairs are candidates for 人工整理
(both directions must then be written into the map, per its own `_comment`), matching the
established precedent for that file.

    python3 external/ahospital_aliases.py      # harvest from raw pages, write candidates

Read-only w.r.t. the knowledge base.
"""
from __future__ import annotations

import argparse
import glob
import json
import re
import sys
import unicodedata
from collections import Counter
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
RAW_DIR = REPO / "external/ahospital/raw/pages"
OUT_JSON = REPO / "external/ahospital_aliases.json"
ALIAS_MAP = REPO / "internal/knowledge/alias_map.json"

REDIRECT_RE = re.compile(r'（重定向自\s*<a href="[^"]*"[^>]*>([^<]+)</a>')
H1_RE = re.compile(r"<h1[^>]*>(?:<firstHeading>)?([^<]+)")

# 机构/奖项/生物分类页：站方确实把它们当条目，但它们不是医学同义词。
JUNK = re.compile(
    r"(医院|保健院|卫生院|卫生室|卫生所|门诊部|制药|药业|药业公司|公司|厂|宾馆|饭店|报社|出版社|"
    r"杂志社|编辑部|大学|学院|学校|学会|协会|研究会|疾病预防控制中心)$"
    r"|诺贝尔|奥斯卡|维基百科|^Wikipedia|/|_|[A-Za-z]{9,}")

# 我们已有的正式名表：只取名称主字段，用来标注「这个别名接到哪个我们已经覆盖的正题上」。
NAME_FIELDS = {
    "icd10": ("name_zh",),
    "icd11": ("title_zh", "name_zh"),
    "nmpa": ("name_zh",),
    "orphanet": ("name_zh",),
    "hpo": ("name_zh",),
    "diseaseenc": ("name_zh",),
    "msd": ("title",),
    "medical": ("condition_zh",),
}
# alias_map 自己的家规是「key 须 >=2 字」（现存 1780 键里 558 个正是 2 字，如 甲亢/脑瘫/性病），
# 所以这里不额外抬门槛——磁铁教训针对的是条目 keywords（每次命中 +3.0 分），
# ExpandQuery 的键只是往查询里加词，性质不同。短键靠 min_runes 字段交人工过目。
MIN_KEY_RUNES = 2


def norm(s: object) -> str:
    t = unicodedata.normalize("NFKC", str(s)).strip().lower()
    return re.sub(r"[\s\u3000]+", "", t)


def cjk_count(s: str) -> int:
    return len(re.findall(r"[一-鿿]", s))


def load_official_names() -> dict[str, set[str]]:
    """normalized name -> which datasets carry it."""
    out: dict[str, set[str]] = {}
    for dataset, fields in NAME_FIELDS.items():
        for path in glob.glob(str(REPO / f"internal/knowledge/data/{dataset}/*.json")):
            with open(path, encoding="utf-8") as fh:
                data = json.load(fh)
            if isinstance(data, dict):
                data = next(iter(data.values()), [])
            if not isinstance(data, list):
                continue
            for row in data:
                if not isinstance(row, dict):
                    continue
                for field in fields:
                    value = row.get(field)
                    if value:
                        out.setdefault(norm(value), set()).add(dataset)
    return out


def extract_pairs() -> list[tuple[str, str]]:
    pairs = []
    for path in sorted(glob.glob(str(RAW_DIR / "*"))):
        try:
            html = open(path, encoding="utf-8", errors="ignore").read()
        except OSError as exc:  # a truncated fetch is a real condition, not a bug to hide
            print(f"  ! unreadable {Path(path).name}: {exc}", file=sys.stderr)
            continue
        red = REDIRECT_RE.search(html)
        h1 = H1_RE.search(html)
        if not red or not h1:
            continue
        src = re.sub(r"\s+", "", red.group(1)).strip()
        tgt = re.sub(r"\s+", "", h1.group(1)).strip()
        # <title> carries the marketing suffix; <h1> is the article name. Guard anyway.
        tgt = tgt.split(" - A+")[0]
        if src and tgt:
            pairs.append((src, tgt))
    return pairs


def build_candidates(pairs, official, existing_alias):
    stats: Counter[str] = Counter()
    seen: set[tuple[str, str]] = set()
    out = []
    for src, tgt in pairs:
        stats["pairs"] += 1
        if norm(src) == norm(tgt):
            stats["same-normalized"] += 1
            continue
        if JUNK.search(src) or JUNK.search(tgt):
            stats["junk"] += 1
            continue
        # alias_map 是双向成对写的，所以两侧都会成为 ExpandQuery 的键 —— 门槛按 min 判定，
        # 短键不删但打标，交人工过目。
        runes = min(cjk_count(src), cjk_count(tgt))
        if runes < MIN_KEY_RUNES:
            stats["too-short"] += 1
            continue
        key = (norm(src), norm(tgt))
        if key in seen or key[::-1] in seen:
            stats["dup"] += 1
            continue
        seen.add(key)
        src_hit = sorted(official.get(norm(src), set()))
        tgt_hit = sorted(official.get(norm(tgt), set()))
        if not src_hit and not tgt_hit:
            stats["neither-known"] += 1
            continue
        if norm(src) in existing_alias and norm(tgt) in existing_alias.get(norm(src), set()):
            stats["already-in-alias-map"] += 1
            continue
        out.append({
            "alias": src,
            "canonical": tgt,
            "min_runes": runes,
            "alias_in_ours": src_hit,
            "canonical_in_ours": tgt_hit,
        })
    out.sort(key=lambda c: c["min_runes"])
    return out, stats


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description="Harvest alias pairs from the raw crawl.")
    ap.parse_args(argv)

    official = load_official_names()
    alias = json.load(open(ALIAS_MAP, encoding="utf-8"))
    existing = {norm(k): {norm(v) for v in vs} for k, vs in alias.items() if k != "_comment"}

    pairs = extract_pairs()
    print(f"redirect pairs from {len(list(glob.glob(str(RAW_DIR / '*'))))} raw pages: {len(pairs)}")
    candidates, stats = build_candidates(pairs, official, existing)
    print("filter chain:", dict(stats))
    print(f"candidates: {len(candidates)} (every pair has at least one side already in our tables)")

    OUT_JSON.write_text(json.dumps(candidates, ensure_ascii=False, indent=1), encoding="utf-8")
    print(f"wrote {OUT_JSON.relative_to(REPO)}")
    for c in candidates[:30]:
        tag = ",".join(c["canonical_in_ours"] or c["alias_in_ours"])
        print(f"   {c['alias']} <-> {c['canonical']}   [{tag}]")
    return 0


if __name__ == "__main__":
    sys.exit(main())
