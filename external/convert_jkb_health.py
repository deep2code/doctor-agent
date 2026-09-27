#!/usr/bin/env python3
"""Convert 健康报医学科普文章 (external/jkb_health/raw/) to KnowledgeEntry JSON.

Usage:
    python3 external/convert_jkb_health.py

Input:  external/jkb_health/index.tsv + raw/*.txt (only "医学科普" category)
Output: internal/knowledge/data/jkb_health.json

index.tsv format (tab-separated):
    filename<TAB>category<TAB>title<TAB>url

raw file format:
    来源: 健康报（国家卫生健康委员会主管）· 中医中药
    标题: ...
    发布: YYYY-MM-DD | 来源：...
    URL: ...
    ============================================================
    >
    <body text>
"""

import csv
import json
import re
import sys
from pathlib import Path

RAW_DIR = Path(__file__).parent / "jkb_health" / "raw"
INDEX = Path(__file__).parent / "jkb_health" / "index.tsv"
OUTPUT = Path(__file__).parent.parent / "internal" / "knowledge" / "data" / "jkb_health.json"


def parse_article(filepath: Path, title: str, url: str) -> dict | None:
    """Parse a single JKB article file into a KnowledgeEntry."""
    with open(filepath, encoding="utf-8") as f:
        content = f.read()

    # Extract body after the separator line
    if "============================================================" in content:
        body_text = content.split("============================================================", 1)[1].strip()
        # Remove leading "> " marker
        if body_text.startswith(">"):
            body_text = body_text[1:].strip()
    else:
        body_text = content.strip()

    if len(body_text) < 200:
        return None

    # Extract date from content header
    year = 2026
    date_match = re.search(r"(20\d{2})-\d{2}-\d{2}", content[:500])
    if date_match:
        year = int(date_match.group(1))

    # Extract section/category from first line
    section = ""
    first_line_match = re.search(r"来源:.*?·\s*(.+)", content.split('\n')[0])
    if first_line_match:
        section = first_line_match.group(1).strip()

    # Build keywords
    keywords = [title, "健康报", "医学科普"]
    if section:
        keywords.append(section)

    entry = {
        "id": f"jkb-{filepath.stem}",
        "title_zh": title,
        "condition_zh": title,
        "summary_zh": body_text[:500],
        "details_zh": body_text,
        "keywords": keywords,
        "category": "health_education",
        "evidence": "official_statement",
        "citations": [
            {
                "type": "national_guideline",
                "title": f"健康报 - {title}",
                "journal": "",
                "year": year,
                "url": url,
                "level": "official_guideline",
            }
        ],
    }

    return entry


def main():
    if not RAW_DIR.exists() or not INDEX.exists():
        print(f"Error: Required files not found", file=sys.stderr)
        sys.exit(1)

    # Read index to get metadata
    articles_meta = {}
    with open(INDEX, encoding="utf-8") as f:
        reader = csv.reader(f, delimiter='\t')
        for row in reader:
            if len(row) >= 4 and row[1] == "医学科普":
                filename = row[0]
                title = row[2]
                url = row[3]
                articles_meta[filename] = {"title": title, "url": url}

    print(f"Found {len(articles_meta)} '医学科普' articles in index")

    entries = []
    skipped_short = 0
    missing_files = 0

    for filename, meta in sorted(articles_meta.items()):
        filepath = RAW_DIR / filename
        if not filepath.exists():
            missing_files += 1
            continue

        entry = parse_article(filepath, meta["title"], meta["url"])
        if entry is None:
            skipped_short += 1
            continue

        entries.append(entry)

    # Write output
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    with open(OUTPUT, "w", encoding="utf-8") as f:
        json.dump(entries, f, ensure_ascii=False, indent=2)

    print(f"✅ Converted {len(entries)} 健康报医学科普文章")
    print(f"   Missing files: {missing_files}")
    print(f"   Skipped (too short): {skipped_short}")
    print(f"   Output: {OUTPUT}")
    print(f"   File size: {OUTPUT.stat().st_size / 1024:.1f} KB")


if __name__ == "__main__":
    main()
