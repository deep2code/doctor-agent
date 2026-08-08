#!/usr/bin/env python3
"""Merge external/msd_manual/ (consumer) + external/msd_manual_prof/
(professional) into internal/knowledge/data/msd_manual.json."""
import json
from pathlib import Path

ROOT = Path(__file__).parent.parent
OUT = ROOT / "internal" / "knowledge" / "data" / "msd_manual.json"

def main():
    entries = []
    for d, source in [("msd_manual", "consumer"), ("msd_manual_prof", "professional")]:
        dpath = Path(__file__).parent / d
        if not dpath.exists():
            continue
        for f in sorted(dpath.glob("*.json")):
            rec = json.load(open(f))
            rec["source"] = source
            entries.append(rec)
    doc = {"source": "msd_manual", "updated": "2026-08-08", "entries": entries}
    OUT.write_text(json.dumps(doc, ensure_ascii=False), encoding="utf-8")
    size_mb = OUT.stat().st_size / 1048576
    print(f"✅ {OUT}: {len(entries)} 页, {size_mb:.1f} MB")

if __name__ == "__main__":
    main()
