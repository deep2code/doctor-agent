#!/usr/bin/env python3
"""Merge the fetched AAP healthychildren.org pages (external/aap/pages/*.json)
into internal/knowledge/data/aap_articles.json — an English full-text
parenting layer, same shape as medlineplus.json. Pages with <200 chars body
are dropped. Idempotent.
"""
import json
import sys
from pathlib import Path

ROOT = Path(__file__).parent.parent
SRC = ROOT / "external" / "aap" / "pages"
OUT = ROOT / "internal" / "knowledge" / "data" / "aap_articles.json"
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
        entries.append({"title": title, "url": d.get("url", ""), "content": content})
    entries.sort(key=lambda e: e["title"])
    out = {"source": "aap", "updated": "2026-08", "entries": entries}
    OUT.write_text(json.dumps(out, ensure_ascii=False, indent=1), encoding="utf-8")
    total = sum(len(e["content"]) for e in entries)
    print(f"wrote {OUT} ({len(entries)} articles, {total/1024:.0f} KB text)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
