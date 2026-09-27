#!/usr/bin/env python3
"""Convert China CDC science popularization articles to KnowledgeEntry JSON.

Source: external/chinacdc_science/raw/*.txt (parsed from chinacdc.cn)
Format: # URL / # 标题 / # 发布方/日期 headers followed by article body
Target: internal/knowledge/data/chinacdc_science.json

Each entry becomes a medical KnowledgeEntry with:
- id: ccs-{filename_without_ext}
- title_zh: extracted from header line 2
- condition_zh: short disease/topic name extracted from title
- summary_zh: first 500 chars of body text
- details_zh: full body text
- keywords: [title, condition_zh] + category-specific terms
- category: "cdc_health" (infectious diseases, environmental health, nutrition, etc.)
- evidence: "national_guideline"
- citations: [{type: "national_report", journal: "", year: YYYY, url: URL, level: "official_guideline"}]

Zero LLM calls — pure text parsing.
"""

import json
import os
import re
from pathlib import Path

RAW_DIR = Path(__file__).parent / "chinacdc_science" / "raw"
OUTPUT = Path(__file__).parent.parent / "internal" / "knowledge" / "data" / "chinacdc_science.json"


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
            # Format: "中国疾病预防控制中心，2025-12-30" or "... | 2025-04-24"
            parts = [p.strip() for p in re.split(r"[,|]", rest)]
            if len(parts) >= 1:
                publisher = parts[0]
            if len(parts) >= 2:
                date_str = parts[1]
                m = re.search(r"(20\d{4})[-/]", date_str)
                if m:
                    year = m.group(1)[:4]
            # Fallback: extract year from entire string
            if not year:
                m = re.search(r"(20\d{2})", rest)
                if m:
                    year = m.group(1)

    return url, title, publisher, year


def extract_condition(title):
    """Extract short disease/topic name from title."""
    # Remove common suffixes
    condition = title
    for suffix in [
        "你需要知道的",
        "这些事",
        "别忽视",
        "一定要警惕",
        "很多人都忽略了",
        "要注意",
        "守护",
        "提示",
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


def categorize_by_url(url):
    """Categorize article based on URL path structure."""
    if "/crb/" in url:  # Infectious diseases
        return "infectious_disease"
    elif "/mxfcrb/" in url:  # Cancer prevention
        return "cancer_prevention"
    elif "/yyjk/" in url:  # Nutrition
        return "nutrition"
    elif "/hjjk/" in url:  # Environmental health
        return "environmental_health"
    elif "/ggws/" in url:  # Public health alerts
        return "public_health_alert"
    else:
        return "general_health"


def build_keywords(title, condition, category, body_text):
    """Build keyword list based on article category."""
    keywords = [title, condition]

    if category == "infectious_disease":
        if "鼠疫" in title:
            keywords.extend(["鼠疫", "耶尔森菌", "跳蚤叮咬", "自然疫源地", "旱獭"])
        elif "流感" in title or "合胞病毒" in title:
            keywords.extend(["呼吸道传染病", "病毒感染", "隔离措施", "抗病毒治疗"])
        elif "手足口" in title:
            keywords.extend(["肠道病毒", "EV71", "儿童传染病", "疫苗接种"])
        elif "登革热" in title:
            keywords.extend(["蚊媒传染病", "伊蚊", "发热皮疹", "防蚊灭蚊"])
        else:
            keywords.extend(["传染病", "预防措施", "个人防护", "疫苗接种"])

    elif category == "cancer_prevention":
        keywords.extend(["癌症早期筛查", "肿瘤标志物", "高危人群", "定期体检"])
        if "结直肠癌" in title:
            keywords.extend(["肠镜检查", "便潜血", "息肉切除", "膳食纤维"])

    elif category == "nutrition":
        keywords.extend(["合理膳食", "营养均衡", "食品安全", "饮食指导"])
        if "肥胖" in title or "体重" in title:
            keywords.extend(["BMI", "腰围测量", "代谢综合征", "体重管理"])

    elif category == "environmental_health":
        keywords.extend(["饮用水安全", "游泳池卫生", "室内空气质量", "环境卫生"])
        if "水" in title:
            keywords.extend(["水质检测", "二次供水", "农村饮水", "消毒处理"])

    elif category == "public_health_alert":
        keywords.extend(["健康防护", "季节性疾病预防", "假期健康提示", "公共卫生"])

    # General keywords
    keywords.extend(["中国疾控中心", "健康教育", "科学预防", "权威指南"])

    return keywords


def convert_file(filepath):
    """Convert a single chinacdc science file to KnowledgeEntry."""
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
    category = categorize_by_url(url)

    # Build keywords
    keywords = build_keywords(title, condition, category, body_text)

    # Build entry
    entry = {
        "id": f"ccs-{fname}",
        "title_zh": title,
        "condition_zh": condition,
        "summary_zh": body_text[:500],
        "details_zh": body_text,
        "keywords": keywords,
        "category": "cdc_health",
        "evidence": "national_guideline",
        "citations": [
            {
                "type": "national_report",
                "journal": "",  # Official CDC publications, no DOI/PMID
                "year": int(year) if year else 2024,
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
    print(f"Processing {len(txt_files)} China CDC science files...")

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
