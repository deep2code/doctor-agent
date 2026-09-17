#!/usr/bin/env python3
"""Clean the FDA-label Chinese entries (fda_drug_labels.json and
external/dailymed_structured/*.json):

  1. Split comma-joined keywords into separate entries (LLM quirk).
  2. Replace dead DailyMed URLs (empty setid) with a DailyMed search URL.

Run AFTER a structurize run finishes (structurize rewrites the merged file).
"""
import json
import re
import urllib.parse
from pathlib import Path

ROOT = Path(__file__).parent.parent
MERGED = ROOT / "internal" / "knowledge" / "data" / "fda_drug_labels.json"
STRUCT = Path(__file__).parent / "dailymed_structured"


def split_keywords(kws) -> list:
    if isinstance(kws, str):
        kws = [kws]
    out = []
    for k in kws or []:
        for part in re.split(r"[，,;；]", k):
            part = part.strip()
            if part:
                out.append(part)
    return list(dict.fromkeys(out))


def fix_url(entry: dict) -> str:
    url = entry.get("source_url", "")
    if url and not url.endswith("setid="):
        return url
    q = urllib.parse.quote(entry.get("name_en") or entry.get("name_zh") or "")
    return f"https://dailymed.nlm.nih.gov/dailymed/search.cfm?labeltype=all&query={q}"


def clean_entry(entry: dict) -> dict:
    entry["keywords"] = split_keywords(entry.get("keywords", []))
    entry["source_url"] = fix_url(entry)
    return entry


def main() -> int:
    # clean the per-file structured outputs too (they are the source of truth)
    # note: each dailymed_structured/{slug}.json holds ONE entry (a dict)
    n_files = 0
    for f in STRUCT.glob("*.json"):
        entry = json.loads(f.read_text(encoding="utf-8"))
        clean_entry(entry)
        f.write_text(json.dumps(entry, ensure_ascii=False, indent=1), encoding="utf-8")
        n_files += 1

    d = json.loads(MERGED.read_text(encoding="utf-8"))
    n_drugs = len(d["drugs"])
    n_kw = sum(1 for x in d["drugs"] if len(x.get("keywords", [])) == 1 and "，" in x["keywords"][0])
    n_url = sum(1 for x in d["drugs"] if not x.get("source_url") or x["source_url"].endswith("setid="))
    for x in d["drugs"]:
        clean_entry(x)
    MERGED.write_text(json.dumps(d, ensure_ascii=False, indent=1), encoding="utf-8")
    print(f"清洗 {n_drugs} 条: keywords单串 {n_kw} -> 已拆分, source_url 空 {n_url} -> 已修复; 同步 {n_files} 个结构化文件")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
