#!/usr/bin/env python3
"""Convert 科学辟谣平台 raw articles → KnowledgeEntry JSON.

Format of raw files (verified):
  Line 1: 来源: 科学辟谣平台（中国科协/中央网信办）· 辟谣文章「标题」
  Line 2: 分类: 疾病防治
  Line 3: 发布: 2022-04-13
  Line 4: URL: https://piyao.kepuchina.cn/rumor/rumordetail?id=XXX
  Line 5: 抓取方式: ...
  Line 6: ===...
  Line 7+: 正文（流言/真相结构）
  Last few lines: 关键词 / --> / keywords... / 所属分类 / 辟谣 / author / reviewer / time

Output: external/piyao_health/piyao_articles.json (list of KnowledgeEntry)
"""
import json
import re
import pathlib
from typing import List, Dict, Any

RAW_DIR = pathlib.Path(__file__).resolve().parent / "piyao_health" / "raw"
OUTPUT_FILE = pathlib.Path(__file__).resolve().parent / "piyao_health" / "piyao_articles.json"


def parse_article(filepath: pathlib.Path) -> Dict[str, Any]:
    """Parse a single raw article file."""
    with open(filepath, 'r', encoding='utf-8') as f:
        lines = f.readlines()

    if not lines:
        return None

    # Header parsing
    title_match = re.search(r'辟谣文章「(.+?)」', lines[0])
    title = title_match.group(1) if title_match else filepath.stem.split('_', 1)[1]

    category = ""
    pub_date = ""
    url = ""

    for line in lines[:10]:
        if line.startswith('分类:'):
            category = line.split(':', 1)[1].strip()
        elif line.startswith('发布:'):
            pub_date = line.split(':', 1)[1].strip()
        elif line.startswith('URL:'):
            url = line.split(':', 1)[1].strip()

    # Body extraction (between header separator and footer keywords)
    body_start = None
    body_end = None
    for i, line in enumerate(lines):
        if line.strip().startswith('==='):
            body_start = i + 1
        if line.strip() == '关键词':
            body_end = i
            break
        if line.strip() == '-->':
            body_end = i
            break

    if body_start is None or body_end is None:
        return None

    body_lines = lines[body_start:body_end]
    # Clean up
    body = '\n'.join(l.strip() for l in body_lines if l.strip() and not l.strip().startswith('>'))
    body = re.sub(r'\n{3,}', '\n\n', body)

    if len(body) < 100:
        return None

    # Extract rumor claim and truth
    rumor_claim = ""
    truth = ""
    for line in body_lines:
        if line.strip().startswith('流言：'):
            rumor_claim = line.strip()[4:]
        elif line.strip().startswith('真相：'):
            truth_start = body_lines.index(line)
            truth = '\n'.join(l.strip() for l in body_lines[truth_start:] if l.strip())
            break

    # Keywords from filename or body
    keywords = [title]
    if category:
        keywords.append(category)

    # Build KnowledgeEntry
    entry = {
        "id": filepath.stem,  # Full filename without extension (e.g., py1_Abej_提肛能强身健体)
        "title_zh": title,
        "condition_zh": title,  # Short topic phrase for rule-3 scoring
        "summary_zh": truth[:200] if truth else body[:200],
        "details_zh": body,
        "keywords": keywords,
        "category": "rumor_debunking",
        "evidence": "official_statement",
        "citations": [{
            "type": "national_guideline",
            "title": f"科学辟谣平台 - {title}",
            "journal": "",
            "year": int(pub_date[:4]) if pub_date else 2024,
            "url": url,
            "level": "official_guideline"
        }]
    }

    return entry


def main():
    print("=" * 70)
    print("科学辟谣平台文章转换器")
    print("=" * 70)

    txt_files = sorted(RAW_DIR.glob("*.txt"))
    print(f"发现 {len(txt_files)} 篇raw文章\n")

    entries = []
    skipped = 0

    for idx, filepath in enumerate(txt_files, 1):
        if idx % 500 == 0:
            print(f"  处理进度: {idx}/{len(txt_files)}")

        entry = parse_article(filepath)
        if entry:
            entries.append(entry)
        else:
            skipped += 1

    print(f"\n成功转换 {len(entries)} 篇")
    if skipped:
        print(f"跳过 {skipped} 篇（正文过短或格式异常）")

    # Save
    with open(OUTPUT_FILE, 'w', encoding='utf-8') as f:
        json.dump(entries, f, ensure_ascii=False, indent=2)

    size_mb = OUTPUT_FILE.stat().st_size / (1024 * 1024)
    print(f"输出文件: {OUTPUT_FILE} ({size_mb:.1f}MB)")
    print(f"平均每条: {sum(len(e['details_zh']) for e in entries) / len(entries):.0f} 字符")


if __name__ == '__main__':
    main()
