#!/usr/bin/env python3
"""Convert Shanghai Children's Hospital sleep & child health articles to KnowledgeEntry JSON.

Source: external/sleep_child_redcross/raw/*.txt (parsed from shchildren.com.cn)
Format: # URL / # 标题 / # 发布方/日期 headers followed by article body
Target: internal/knowledge/data/sleep_child_redcross.json

Each entry becomes a medical KnowledgeEntry with:
- id: scr-{filename_without_ext}
- title_zh: extracted from header line 2
- condition_zh: short disease/topic name extracted from title
- summary_zh: first 500 chars of body text
- details_zh: full body text
- keywords: [title, condition_zh] + category-specific terms
- category: "pediatric_health" or "sleep_health"
- evidence: "official_guideline"
- citations: [{type: "national_report", journal: "", year: YYYY, url: URL, level: "official_guideline"}]

Zero LLM calls — pure text parsing.
"""

import json
import os
import re
from pathlib import Path

RAW_DIR = Path(__file__).parent / "sleep_child_redcross" / "raw"
OUTPUT = Path(__file__).parent.parent / "internal" / "knowledge" / "data" / "sleep_child_redcross.json"


def parse_header(lines):
    """Parse the three-line header (# URL / # 标题 / # 发布方/日期)."""
    url = ""
    title = ""
    publisher = ""
    year = ""

    for line in lines[:3]:
        line = line.strip()
        if line.startswith("# URL:"):
            url = line.split(":", 1)[1].strip()
        elif line.startswith("# 标题:"):
            title = line.split(":", 1)[1].strip()
        elif line.startswith("# 发布方/日期:"):
            rest = line.split(":", 1)[1].strip()
            # Format: "上海市儿童医院（科普教育栏目） 2023-11-07 发布人：龚紫兰"
            # Extract year from date pattern
            m = re.search(r"(20\d{4})[-/]", rest)
            if m:
                year = m.group(1)[:4]
            else:
                m = re.search(r"(20\d{2})", rest)
                if m:
                    year = m.group(1)
            # Extract publisher (before date)
            publisher_match = re.match(r"^([^\d]+?)(?:\s*\d{4}-)", rest)
            if publisher_match:
                publisher = publisher_match.group(1).strip()
            else:
                publisher = rest.split()[0] if rest else "上海市儿童医院"

    return url, title, publisher, year


def extract_condition(title):
    """Extract short disease/topic name from title."""
    # Remove common suffixes
    condition = title
    for suffix in [
        "怎么办",
        "支招",
        "攻略",
        "解读",
        "警示",
        "提醒",
        "警惕",
        "家长必读",
        "专家来支招",
        "！",
        "？",
    ]:
        condition = condition.replace(suffix, "")
    # Remove parenthetical content
    condition = re.sub(r"\(.*?\)", "", condition)
    condition = re.sub(r"（.*?）", "", condition)
    # Clean up
    condition = condition.strip()
    return condition


def categorize_article(title, body_text):
    """Categorize article as pediatric_health or sleep_health."""
    sleep_keywords = ["睡眠", "失眠", "嗜睡", "发作性睡病", "narcolepsy", "sleep"]
    if any(kw in title or kw in body_text[:500] for kw in sleep_keywords):
        return "sleep_health"
    return "pediatric_health"


def build_keywords(title, condition, category, body_text):
    """Build keyword list based on article category."""
    keywords = [title, condition]

    if category == "sleep_health":
        keywords.extend(["睡眠质量", "睡眠障碍", "作息规律", "睡眠卫生"])
        if "儿童" in title or "孩子" in title:
            keywords.extend(["儿童睡眠", "青少年睡眠"])
    else:
        # Pediatric health
        if "发热" in title or "发烧" in title:
            keywords.extend(["退烧", "对乙酰氨基酚", "布洛芬", "物理降温", "高热惊厥"])
        elif "过敏" in title:
            keywords.extend(["过敏性鼻炎", "过敏原检测", "抗组胺药", "脱敏治疗"])
        elif "疫苗" in title:
            keywords.extend(["预防接种", "免疫规划", "疫苗接种时间", "不良反应"])
        elif "流感" in title:
            keywords.extend(["甲流", "乙流", "奥司他韦", "抗病毒治疗", "隔离措施"])
        elif "腹泻" in title or "肠胃" in title:
            keywords.extend(["急性胃肠炎", "脱水", "口服补液盐", "益生菌"])
        elif "哮喘" in title or "喘息" in title:
            keywords.extend(["支气管哮喘", "吸入治疗", "雾化", "肺功能检查"])
        elif "自闭症" in title or "孤独症" in title:
            keywords.extend(["自闭症谱系障碍", "早期筛查", "行为干预", "社交障碍"])
        elif "水痘" in title:
            keywords.extend(["水痘带状疱疹病毒", "隔离期", "疫苗接种", "皮疹护理"])
        elif "骨折" in title or "外伤" in title:
            keywords.extend(["儿童骨折", "石膏固定", "康复训练", "创伤处理"])

        # General pediatric keywords
        keywords.extend(["儿科", "儿童健康", "育儿", "家庭护理", "就医时机"])

    return keywords


def convert_file(filepath):
    """Convert a single sleep/child redcross file to KnowledgeEntry."""
    fname = filepath.stem

    with open(filepath, "r", encoding="utf-8") as f:
        content = f.read()

    lines = content.split("\n")

    # Parse header
    url, title, publisher, year = parse_header(lines)

    if not title:
        print(f"  ⚠️  {fname}: no title found, skipping")
        return None

    # Extract body text (skip header lines)
    body_lines = []
    skip_header = True
    for line in lines:
        if skip_header and line.startswith("#"):
            continue
        elif skip_header:
            skip_header = False
            continue
        body_lines.append(line)

    body_text = "\n".join(body_lines).strip()

    if len(body_text) < 200:
        print(f"  ⚠️  {fname}: body too short ({len(body_text)} chars), skipping")
        return None

    # Extract condition
    condition = extract_condition(title)

    # Categorize
    category = categorize_article(title, body_text)

    # Build keywords
    keywords = build_keywords(title, condition, category, body_text)

    # Build entry
    entry = {
        "id": f"scr-{fname}",
        "title_zh": title,
        "condition_zh": condition,
        "summary_zh": body_text[:500],
        "details_zh": body_text,
        "keywords": keywords,
        "category": category,
        "evidence": "official_guideline",
        "citations": [
            {
                "type": "national_report",
                "journal": "",  # Official hospital publications, no DOI/PMID
                "year": int(year) if year else 2023,
                "url": url,
                "level": "official_guideline",
            }
        ],
    }

    return entry


def main():
    entries = []
    skipped = 0

    txt_files = sorted(RAW_DIR.glob("*.txt"))
    print(f"Processing {len(txt_files)} sleep/child redcross files...")

    for filepath in txt_files:
        entry = convert_file(filepath)
        if entry:
            entries.append(entry)
        else:
            skipped += 1

    print(f"\n✅ Converted {len(entries)} entries ({skipped} skipped)")

    # Write output
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    with open(OUTPUT, "w", encoding="utf-8") as f:
        json.dump(entries, f, ensure_ascii=False, indent=2)

    print(f"📄 Written to {OUTPUT}")
    print(f"   Size: {os.path.getsize(OUTPUT) / 1024:.1f} KB")


if __name__ == "__main__":
    main()
