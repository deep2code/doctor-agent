#!/usr/bin/env python3
"""Merge the fetched 香港卫生署家庭健康服务 pages (external/fhs/pages/*.json)
into internal/knowledge/data/fhs_guides.json — a Chinese full-text parenting
layer, same shape as nhc_guides.json (MSD-style search).

Pages with <200 chars of body (section index pages) are dropped.
Idempotent; re-run after fetch_fhs.py adds pages.
"""
import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).parent.parent
SRC = ROOT / "external" / "fhs" / "pages"
OUT = ROOT / "internal" / "knowledge" / "data" / "fhs_guides.json"
MIN_BODY = 200


def main() -> int:
    entries = []
    seen = set()
    for f in sorted(SRC.glob("*.json")):
        d = json.loads(f.read_text(encoding="utf-8"))
        content = (d.get("content") or "").strip()
        if len(content) < MIN_BODY:
            continue
        title = (d.get("title") or "").strip() or f.stem
        if title in seen:
            continue
        seen.add(title)
        entries.append({
            "title": title,
            "url": d.get("url", ""),
            "content": content,
            "source": "fhs",
        })
    entries.sort(key=lambda e: e["title"])
    out = {"source": "fhs", "updated": "2026-08", "entries": entries}
    OUT.write_text(json.dumps(out, ensure_ascii=False, indent=1), encoding="utf-8")
    total = sum(len(e["content"]) for e in entries)
    print(f"wrote {OUT} ({len(entries)} pages, {total/1024:.0f} KB text)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
