#!/usr/bin/env python3
"""Expand piyao_selected.json from 535 to ~1000 entries with balanced coverage.

Strategy:
1. Keep all existing 535 entries (preserve current selection)
2. Add entries from underrepresented categories (food safety, life questions, beauty/fitness)
3. Balance across source batches (py1/py2/py6/py7/py15/py16)
4. Prioritize entries with clear rumor + scientific explanation structure

Usage:
    python3 external/expand_piyao_selection.py
"""

import csv
import json
import random
from pathlib import Path
from collections import Counter

INDEX = Path(__file__).parent / "piyao_health" / "index.tsv"
CURRENT = Path(__file__).parent.parent / "internal" / "knowledge" / "data" / "piyao_selected.json"
OUTPUT = CURRENT  # Will overwrite in place

random.seed(42)  # Reproducible selection


def load_current_ids():
    """Load currently selected entry IDs."""
    data = json.load(open(CURRENT))
    return {e['id'] for e in data}


def parse_index():
    """Parse index.tsv and return list of article metadata."""
    articles = []
    with open(INDEX, encoding='utf-8') as f:
        reader = csv.reader(f, delimiter='\t')
        for row in reader:
            if len(row) >= 4 and row[0].startswith('py'):
                filename = row[0]
                category = row[1]
                title = row[2]
                url = row[3]

                # Extract prefix (batch indicator)
                prefix = filename.split('_')[0] if '_' in filename else filename[:5]

                articles.append({
                    'filename': filename,
                    'category': category,
                    'title': title,
                    'url': url,
                    'prefix': prefix,
                    'id': filename.replace('.txt', '').replace('py', 'py'),  # Will be set properly when loading
                })
    return articles


def select_additional(current_ids, all_articles, target_total=1000):
    """Select additional articles to reach target total."""
    current_count = len(current_ids)
    need = target_total - current_count

    if need <= 0:
        print(f"Already have {current_count} entries, no expansion needed")
        return []

    print(f"Need {need} more entries to reach {target_total}")

    # Filter out already selected
    candidates = [a for a in all_articles if a['filename'].replace('.txt', '') not in current_ids]

    # Strategy: prioritize by category balance
    # Current distribution shows heavy py1/py15, let's boost py2/py6/py16
    category_priority = {
        '食品安全': 3.0,      # Underrepresented, high value
        '生活解惑': 2.5,       # Everyday questions
        '美容健身': 2.5,       # Lifestyle health
        '营养健康': 2.0,       # Important but already well-covered
        '疾病防治': 1.5,       # Core but large volume
        '生物': 1.0,           # Specialized
    }

    prefix_priority = {
        'py2': 2.5,   # Food safety mostly
        'py6': 2.0,   # Life questions
        'py16': 2.0,  # Beauty/fitness
        'py7': 1.5,   # Biology
        'py1': 1.0,   # Disease prevention (already well-covered)
        'py15': 1.0,  # Nutrition (already well-covered)
    }

    # Score candidates
    scored = []
    for art in candidates:
        cat_score = category_priority.get(art['category'], 1.0)
        prefix_score = prefix_priority.get(art['prefix'], 1.0)

        # Prefer titles that are clear questions or myth statements
        title = art['title']
        has_question = any(c in title for c in ['？', '?', '吗', '呢', '吧'])
        has_myth_marker = any(w in title for w in ['真的', '其实', '未必', '不一定', '误区', '真相'])

        quality_bonus = 1.2 if (has_question or has_myth_marker) else 1.0

        score = cat_score * prefix_score * quality_bonus
        scored.append((score, art))

    # Sort by score descending
    scored.sort(key=lambda x: -x[0])

    # Select top N
    selected = [art for _, art in scored[:need]]

    print(f"Selected {len(selected)} additional entries")
    print(f"  Category breakdown:")
    cats = Counter(a['category'] for a in selected)
    for cat, count in cats.most_common():
        print(f"    {cat}: {count}")

    return selected


def main():
    current_ids = load_current_ids()
    print(f"Current selection: {len(current_ids)} entries")

    all_articles = parse_index()
    print(f"Total available: {len(all_articles)} entries")

    additional = select_additional(current_ids, all_articles, target_total=1000)

    # Load current data
    current_data = json.load(open(CURRENT))

    # Build new entries (minimal structure - full data will come from raw files)
    new_entries = []
    for art in additional:
        entry_id = art['filename'].replace('.txt', '')
        new_entries.append({
            'id': entry_id,
            'title_zh': art['title'],
            'condition_zh': art['title'].rstrip('？?'),
            'summary_zh': '',  # Will be filled from raw file
            'details_zh': '',  # Will be filled from raw file
            'keywords': [art['title'], art['category']],
            'category': 'rumor_debunking',
            'evidence': 'official_statement',
            'citations': [{
                'type': 'national_guideline',
                'title': f"科学辟谣平台 - {art['title']}",
                'journal': '',
                'year': 2021,  # Default, will be updated
                'url': art['url'],
                'level': 'official_guideline',
            }]
        })

    # Combine and save
    combined = current_data + new_entries

    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    with open(OUTPUT, 'w', encoding='utf-8') as f:
        json.dump(combined, f, ensure_ascii=False, indent=2)

    print(f"\n✅ Expanded to {len(combined)} entries (added {len(new_entries)})")
    print(f"   Output: {OUTPUT}")
    print(f"   File size: {OUTPUT.stat().st_size / 1024:.1f} KB")


if __name__ == "__main__":
    main()
