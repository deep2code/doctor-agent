#!/usr/bin/env python3
"""Convert China Nutrition Society articles (external/heart_nutrition_orgs/raw/) to KnowledgeEntry JSON.

Usage:
    python3 external/convert_heart_nutrition_orgs.py

Input:  external/heart_nutrition_orgs/raw/*.txt (cnsoc-*.txt)
Output: internal/knowledge/data/heart_nutrition_orgs.json

Format per article file:
    # URL: https://www.cnsoc.org/...
    # 标题: ...
    # 发布方/日期: 中国营养学会（www.cnsoc.org）YYYY-MM-DD

    <body text in paragraphs>
"""

import json
import re
import sys
from pathlib import Path

RAW_DIR = Path(__file__).parent / "heart_nutrition_orgs" / "raw"
OUTPUT = Path(__file__).parent.parent / "internal" / "knowledge" / "data" / "heart_nutrition_orgs.json"


def parse_article(filepath: Path) -> dict | None:
    """Parse a single CNSOC article file into a KnowledgeEntry."""
    with open(filepath, encoding="utf-8") as f:
        lines = f.readlines()

    # Parse header metadata
    url = ""
    title = ""
    date_str = ""

    for line in lines[:5]:
        if line.startswith("# URL:"):
            url = line.split(":", 1)[1].strip()
        elif line.startswith("# 标题:"):
            title = line.split(":", 1)[1].strip()
        elif line.startswith("# 发布方/日期:"):
            date_str = line.split(":", 1)[1].strip()

    if not title:
        return None

    # Extract body (skip header lines starting with #)
    body_lines = [line.strip() for line in lines if not line.startswith("#")]
    body_text = "\n".join(line for line in body_lines if line)

    if len(body_text) < 200:
        # Too short, skip
        return None

    # Determine category based on filename prefix
    fname = filepath.stem
    if fname.startswith("cnsoc-acadconfn"):
        category = "nutrition_conference"
    elif fname.startswith("cnsoc-scienpopuln"):
        category = "nutrition_science_popularization"
    elif fname.startswith("cnsoc-publicac"):
        category = "nutrition_publication"
    elif fname.startswith("cnsoc-othernews"):
        category = "nutrition_news"
    else:
        category = "nutrition_info"

    condition_zh = title

    # Extract year from date string
    year = 2026  # default
    year_match = re.search(r"(20\d{2})", date_str)
    if year_match:
        year = int(year_match.group(1))

    # Build keywords from title
    keywords = [title, "营养", "中国营养学会"]

    entry = {
        "id": f"cnsoc-{fname}",
        "title_zh": title,
        "condition_zh": condition_zh,
        "summary_zh": body_text[:500],  # First 500 chars as summary
        "details_zh": body_text,
        "keywords": keywords,
        "category": category,
        "evidence": "national_guideline",
        "citations": [
            {
                "type": "national_guideline",
                "title": f"中国营养学会 - {title}",
                "journal": "",  # Leave empty for official publications
                "year": year,
                "url": url,
                "level": "official_guideline",
            }
        ],
    }

    return entry


def main():
    if not RAW_DIR.exists():
        print(f"Error: {RAW_DIR} does not exist", file=sys.stderr)
        sys.exit(1)

    entries = []
    skipped_short = 0
    skipped_empty = 0

    for txt_file in sorted(RAW_DIR.glob("*.txt")):
        entry = parse_article(txt_file)
        if entry is None:
            skipped_empty += 1
            continue

        # Check minimum length
        if len(entry["details_zh"]) < 200:
            skipped_short += 1
            continue

        entries.append(entry)

    # Write output
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    with open(OUTPUT, "w", encoding="utf-8") as f:
        json.dump(entries, f, ensure_ascii=False, indent=2)

    print(f"✅ Converted {len(entries)} China Nutrition Society articles")
    print(f"   Skipped (too short): {skipped_short}")
    print(f"   Skipped (empty/parse error): {skipped_empty}")
    print(f"   Output: {OUTPUT}")
    print(f"   File size: {OUTPUT.stat().st_size / 1024:.1f} KB")


if __name__ == "__main__":
    main()
