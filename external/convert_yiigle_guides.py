#!/usr/bin/env python3
"""Convert yiigle clinical guides (34 full-text Chinese guidelines from CMA) to KnowledgeEntry JSON.

Source: external/yiigle_guides/raw/*.txt (parsed from cmab.yiigle.com guide_html pages)
Format: # URL / # 标题 / # 发布方/日期 headers followed by full guideline text
Target: internal/knowledge/data/yiigle_clinical_guides.json

Each entry becomes a medical KnowledgeEntry with:
- id: ycg-{filename_without_ext}
- title_zh: extracted from header line 2
- condition_zh: short disease name extracted from title
- summary_zh: first 500 chars of body text
- details_zh: full body text
- keywords: [title, condition_zh] + category-specific symptom/treatment terms
- category: "clinical_guide"
- evidence: "national_guideline"
- citations: [{type: "national_guideline", journal: "", year: YYYY, url: URL, level: "official_guideline"}]

Zero LLM calls — pure text parsing.
"""

import json
import os
import re
from pathlib import Path

RAW_DIR = Path(__file__).parent / "yiigle_guides" / "raw"
OUTPUT = Path(__file__).parent.parent / "internal" / "knowledge" / "data" / "yiigle_clinical_guides.json"


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
            # Format: "中华医学会妇产科学分会 | 2020-06-25" or "... | 2020"
            parts = [p.strip() for p in rest.split("|")]
            if len(parts) >= 1:
                publisher = parts[0]
            if len(parts) >= 2:
                date_str = parts[1]
                # Extract year from various formats
                m = re.search(r"(20\d{2})", date_str)
                if m:
                    year = m.group(1)
            # Try to extract year from publisher line if not found
            if not year:
                m = re.search(r"(20\d{2})", rest)
                if m:
                    year = m.group(1)

    return url, title, publisher, year


def extract_condition(title):
    """Extract short disease name from title."""
    # Remove common suffixes
    condition = title
    for suffix in [
        "诊治指南",
        "诊疗指南",
        "防治指南",
        "诊断与治疗指南",
        "诊断和治疗指南",
        "专家共识",
        "共识意见",
        "基层诊疗指南",
        "管理指南",
        "治疗指南",
        "预防指南",
        "处理共识",
        "评估及处理中国专家共识",
        "中国专家共识",
        "共识报告",
    ]:
        condition = condition.replace(suffix, "")
    # Remove parenthetical years/versions
    condition = re.sub(r"\(.*?\)", "", condition)
    condition = re.sub(r"（.*?）", "", condition)
    # Clean up
    condition = condition.strip().rstrip(" ")
    return condition


def build_keywords(title, condition, body_text):
    """Build keyword list based on disease category."""
    keywords = [title, condition]

    # Cardiovascular
    if any(kw in condition for kw in ["高血压", "血脂", "心力衰竭", "心血管", "冠心病", "介入"]):
        keywords.extend(["血压控制", "降脂治疗", "心功能分级", "ACEI", "ARB", "他汀类药物", "β受体阻滞剂"])

    # Respiratory
    elif any(kw in condition for kw in ["哮喘", "肺炎", "慢阻肺", "COPD", "肺纤维化", "咳嗽"]):
        keywords.extend(["呼吸困难", "喘息", "肺功能检查", "吸入治疗", "支气管扩张剂", "糖皮质激素"])

    # Endocrine/Metabolic
    elif any(kw in condition for kw in ["糖尿病", "痛风", "高尿酸", "甲状腺", "肥胖"]):
        keywords.extend(["血糖控制", "尿酸达标", "胰岛素治疗", "二甲双胍", "饮食管理", "体重管理"])

    # Neurology
    elif any(kw in condition for kw in ["脑卒中", "脑出血", "失眠", "康复"]):
        keywords.extend(["急性期治疗", "溶栓治疗", "康复治疗", "睡眠障碍", "神经功能缺损"])

    # Gastroenterology
    elif any(kw in condition for kw in ["胃炎", "便秘", "炎症性肠病", "幽门螺杆菌", "胃食管反流", "肝病"]):
        keywords.extend(["腹痛", "消化不良", "内镜检查", "根除治疗", "质子泵抑制剂", "抗生素"])

    # Gynecology
    elif any(kw in condition for kw in ["子宫", "卵巢", "多囊", "肌瘤", "腺肌病"]):
        keywords.extend(["月经失调", "痛经", "不孕症", "激素治疗", "手术治疗"])

    # Pediatrics
    elif "儿童" in condition or "新生儿" in condition:
        keywords.extend(["儿科", "儿童用药", "生长发育", "疫苗接种"])

    # Infectious disease
    elif any(kw in condition for kw in ["狂犬病", "流感", "感染"]):
        keywords.extend(["暴露后预防", "疫苗接种", "抗病毒治疗", "隔离措施"])

    return keywords


def convert_file(filepath):
    """Convert a single yiigle guide file to KnowledgeEntry."""
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

    # Build keywords
    keywords = build_keywords(title, condition, body_text)

    # Build entry
    entry = {
        "id": f"ycg-{fname}",
        "title_zh": title,
        "condition_zh": condition,
        "summary_zh": body_text[:500],
        "details_zh": body_text,
        "keywords": keywords,
        "category": "clinical_guide",
        "evidence": "national_guideline",
        "citations": [
            {
                "type": "national_guideline",
                "journal": "",  # Official CMA publications, no DOI/PMID by design
                "year": int(year) if year else 2020,
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
    print(f"Processing {len(txt_files)} yiigle guide files...")

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
