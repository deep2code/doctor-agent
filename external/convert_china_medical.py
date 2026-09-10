#!/usr/bin/env python3
"""
将外部下载的中国医学数据转换为知识库格式。

处理的数据源:
- china_stats: 卫生统计年鉴
- china_clinical_pathways: 临床路径
- china_cdc: 法定传染病数据
- china_tcm: 中医药知识库
- china_cso: CSCO肿瘤指南
- china_dietary: 居民膳食指南

输出: internal/knowledge/data/ 下的各个 JSON 文件
"""

import json
import re
from pathlib import Path
from typing import Any, Dict, List

ROOT = Path(__file__).parent.parent
EXTERNAL = ROOT / "external"
OUTPUT = ROOT / "internal" / "knowledge" / "data"

# 数据源目录
CHINA_STATS = EXTERNAL / "china_stats"
CHINA_CLINICAL_PATHWAYS = EXTERNAL / "china_clinical_pathways"
CHINA_CDC = EXTERNAL / "china_cdc"
CHINA_TCM = EXTERNAL / "china_tcm"
CHINA_CSO = EXTERNAL / "china_cso"
CHINA_DIETARY = EXTERNAL / "china_dietary"


def load_json(path: Path) -> Any:
    """加载JSON文件"""
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)


def save_json(data: Any, path: Path) -> None:
    """保存JSON文件"""
    path.parent.mkdir(parents=True, exist_ok=True)
    with open(path, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=2)


def convert_china_stats():
    """转换卫生统计年鉴数据"""
    print("转换卫生统计年鉴数据...")

    entries = []
    for year_file in sorted(CHINA_STATS.glob("*.json")):
        if year_file.name in ("metadata.json", "summary.json"):
            continue
        data = load_json(year_file)
        year = data.get("year", year_file.stem)

        entry = {
            "id": f"china_stats_{year}",
            "condition_zh": f"{year}年中国卫生健康统计",
            "condition_en": f"China Health Statistics {year}",
            "category": "health_statistics",
            "regions": ["全国"],
            "data": data.get("data", {}),
            "source": data.get("source", ""),
            "year": year,
            "keywords": ["卫生统计", "医疗机构", "卫生人员", "健康水平"],
            "body": json.dumps(data.get("data", {}), ensure_ascii=False)
        }
        entries.append(entry)

    output_path = OUTPUT / "china_stats.json"
    save_json(entries, output_path)
    print(f"  已保存 {len(entries)} 条记录到 {output_path}")
    return entries


def convert_china_clinical_pathways():
    """转换临床路径数据"""
    print("转换临床路径数据...")

    entries = []
    for pathway_file in sorted(CHINA_CLINICAL_PATHWAYS.glob("*.json")):
        if pathway_file.name in ("metadata.json", "summary.json"):
            continue
        data = load_json(pathway_file)

        # 从文件名提取疾病名称
        disease_name = pathway_file.stem

        # 提取ICD-10
        icd10 = ""
        if "ICD-10" in data.get("适用对象", ""):
            icd10 = data.get("适用对象", "").split("ICD-10：")[-1].split("）")[0]

        entry = {
            "id": f"clinical_pathway_{data.get('pathway_id', disease_name)}",
            "condition_zh": data.get("name", disease_name),
            "condition_en": "",
            "category": "clinical_pathway",
            "icd10": icd10,
            "regions": ["全国"],
            "pathway": {
                "category": data.get("category", ""),
                "version": data.get("version", ""),
                "source": data.get("source", ""),
                "适用对象": data.get("适用对象", ""),
                "诊断依据": data.get("诊断依据", ""),
                "治疗方案": data.get("治疗方案", ""),
                "标准住院日": data.get("标准住院日", ""),
                "进入路径标准": data.get("进入路径标准", ""),
                "排除标准": data.get("排除标准", ""),
                "出院标准": data.get("出院标准", "")
            },
            "keywords": ["临床路径", "诊疗规范", data.get("name", "")],
            "body": f"{data.get('name', '')} {data.get('诊断依据', '')} {data.get('治疗方案', '')}"
        }
        entries.append(entry)

    output_path = OUTPUT / "china_clinical_pathways.json"
    save_json(entries, output_path)
    print(f"  已保存 {len(entries)} 条记录到 {output_path}")
    return entries


def convert_china_cdc():
    """转换法定传染病数据"""
    print("转换法定传染病数据...")

    entries = []

    # 加载疾病列表
    disease_list = {}
    disease_list_path = CHINA_CDC / "disease_list.json"
    if disease_list_path.exists():
        disease_list = load_json(disease_list_path)

    # 加载年度数据
    for year_file in sorted(CHINA_CDC.glob("notifiable_diseases_*.json")):
        data = load_json(year_file)
        year = data.get("year", year_file.stem.split("_")[-1].replace(".json", ""))

        # 按疾病分类处理
        by_category = data.get("by_category", {})
        for category, category_data in by_category.items():
            diseases = category_data.get("diseases", {})
            for disease, stats in diseases.items():
                entry = {
                    "id": f"china_cdc_{year}_{disease}",
                    "condition_zh": disease,
                    "condition_en": disease_list.get(disease, {}).get("name_en", ""),
                    "category": "notifiable_disease",
                    "regions": ["全国"],
                    "year": year,
                    "statistics": {
                        "cases": stats.get("cases", 0),
                        "deaths": stats.get("deaths", 0)
                    },
                    "category_class": category,
                    "source": data.get("source", "中国疾病预防控制中心"),
                    "source_url": data.get("source_url", "https://www.chinacdc.cn"),
                    "keywords": ["传染病", "法定传染病", disease],
                    "body": f"{disease} {year}年发病{stats.get('cases', 0)}例，死亡{stats.get('deaths', 0)}例"
                }
                entries.append(entry)

    output_path = OUTPUT / "china_cdc.json"
    save_json(entries, output_path)
    print(f"  已保存 {len(entries)} 条记录到 {output_path}")
    return entries


def convert_china_tcm():
    """转换中医药知识库数据"""
    print("转换中医药知识库数据...")

    entries = []

    # 转换中药
    for herb_file in sorted(CHINA_TCM.glob("herb_*.json")):
        data = load_json(herb_file)
        entry = {
            "id": f"tcm_herb_{data.get('name_pinyin', herb_file.stem)}",
            "condition_zh": f"中药-{data.get('name', '')}",
            "condition_en": data.get("name_latin", ""),
            "category": "tcm_herb",
            "regions": ["全国"],
            "tcm": {
                "type": "herb",
                "name": data.get("name", ""),
                "name_pinyin": data.get("name_pinyin", ""),
                "name_latin": data.get("name_latin", ""),
                "category": data.get("category", ""),
                "properties": data.get("properties", ""),
                "effects": data.get("effects", ""),
                "indications": data.get("indications", ""),
                "dosage": data.get("dosage", ""),
                "contraindications": data.get("contraindications", ""),
                "source": data.get("source", "")
            },
            "keywords": ["中药", "中医药", data.get("name", ""), data.get("category", "")],
            "body": f"{data.get('name', '')} {data.get('properties', '')} {data.get('effects', '')}"
        }
        entries.append(entry)

    # 转换方剂
    for formula_file in sorted(CHINA_TCM.glob("formula_*.json")):
        data = load_json(formula_file)
        entry = {
            "id": f"tcm_formula_{data.get('name_pinyin', formula_file.stem)}",
            "condition_zh": f"方剂-{data.get('name', '')}",
            "condition_en": "",
            "category": "tcm_formula",
            "regions": ["全国"],
            "tcm": {
                "type": "formula",
                "name": data.get("name", ""),
                "name_pinyin": data.get("name_pinyin", ""),
                "composition": data.get("composition", ""),
                "effects": data.get("effects", ""),
                "indications": data.get("indications", ""),
                "dosage": data.get("dosage", ""),
                "contraindications": data.get("contraindications", ""),
                "source": data.get("source", "")
            },
            "keywords": ["方剂", "中医药", data.get("name", "")],
            "body": f"{data.get('name', '')} {data.get('composition', '')} {data.get('effects', '')}"
        }
        entries.append(entry)

    # 转换穴位
    for acupoint_file in sorted(CHINA_TCM.glob("acupoint_*.json")):
        data = load_json(acupoint_file)
        entry = {
            "id": f"tcm_acupoint_{data.get('name_pinyin', acupoint_file.stem)}",
            "condition_zh": f"穴位-{data.get('name', '')}",
            "condition_en": data.get("name_en", ""),
            "category": "tcm_acupoint",
            "regions": ["全国"],
            "tcm": {
                "type": "acupoint",
                "name": data.get("name", ""),
                "name_pinyin": data.get("name_pinyin", ""),
                "name_en": data.get("name_en", ""),
                "location": data.get("location", ""),
                "indications": data.get("indications", ""),
                "technique": data.get("technique", ""),
                "source": data.get("source", "")
            },
            "keywords": ["穴位", "针灸", data.get("name", "")],
            "body": f"{data.get('name', '')} {data.get('location', '')} {data.get('indications', '')}"
        }
        entries.append(entry)

    output_path = OUTPUT / "china_tcm.json"
    save_json(entries, output_path)
    print(f"  已保存 {len(entries)} 条记录到 {output_path}")
    return entries


def convert_china_cso():
    """转换CSCO肿瘤指南数据"""
    print("转换CSCO肿瘤指南数据...")

    entries = []

    for cso_file in sorted(CHINA_CSO.glob("cso_*.json")):
        if cso_file.name in ("metadata.json", "summary.json"):
            continue
        data = load_json(cso_file)

        cancer_type = data.get("cancer_type", cso_file.stem.replace("cso_", ""))

        entry = {
            "id": f"cso_guideline_{cancer_type}",
            "condition_zh": cancer_type,
            "condition_en": "",
            "category": "cso_guideline",
            "regions": ["全国"],
            "guideline": {
                "cancer_type": data.get("cancer_type", ""),
                "version": data.get("version", ""),
                "source": data.get("source", ""),
                "source_url": data.get("source_url", ""),
                "key_points": data.get("key_points", {})
            },
            "keywords": ["CSCO", "肿瘤指南", "诊疗指南", cancer_type],
            "body": f"{cancer_type} {json.dumps(data.get('key_points', {}), ensure_ascii=False)}"
        }
        entries.append(entry)

    output_path = OUTPUT / "china_cso.json"
    save_json(entries, output_path)
    print(f"  已保存 {len(entries)} 条记录到 {output_path}")
    return entries


def convert_china_dietary():
    """转换居民膳食指南数据"""
    print("转换居民膳食指南数据...")

    entries = []

    # 一般人群膳食指南
    general_path = CHINA_DIETARY / "general_guidelines.json"
    if general_path.exists():
        data = load_json(general_path)
        for i, guideline in enumerate(data.get("guidelines", [])):
            entry = {
                "id": f"dietary_general_{i+1}",
                "condition_zh": f"膳食指南-{guideline.get('number', i+1)}",
                "condition_en": "Dietary Guidelines",
                "category": "dietary_guideline",
                "regions": ["全国"],
                "dietary": {
                    "type": "general",
                    "number": guideline.get("number", i+1),
                    "title": guideline.get("title", ""),
                    "description": guideline.get("description", ""),
                    "recommendations": guideline.get("recommendations", []),
                    "source": data.get("source", ""),
                    "year": data.get("year", "")
                },
                "keywords": ["膳食指南", "营养", f"准则{guideline.get('number', i+1)}"],
                "body": f"{guideline.get('title', '')} {guideline.get('description', '')}"
            }
            entries.append(entry)

    # 膳食宝塔
    pyramid_path = CHINA_DIETARY / "food_pyramid.json"
    if pyramid_path.exists():
        data = load_json(pyramid_path)
        entry = {
            "id": "dietary_pyramid",
            "condition_zh": "中国居民膳食宝塔",
            "condition_en": "Food Pyramid",
            "category": "dietary_pyramid",
            "regions": ["全国"],
            "dietary": {
                "type": "pyramid",
                "levels": data.get("levels", []),
                "source": data.get("source", ""),
                "year": data.get("year", "")
            },
            "keywords": ["膳食宝塔", "平衡膳食", "食物推荐"],
            "body": json.dumps(data.get("levels", []), ensure_ascii=False)
        }
        entries.append(entry)

    # 营养素参考摄入量
    nutrition_path = CHINA_DIETARY / "nutrition_standards.json"
    if nutrition_path.exists():
        data = load_json(nutrition_path)
        entry = {
            "id": "dietary_nutrition_standards",
            "condition_zh": "中国居民膳食营养素参考摄入量",
            "condition_en": "Dietary Reference Intakes",
            "category": "nutrition_standards",
            "regions": ["全国"],
            "dietary": {
                "type": "nutrition",
                "standards": data.get("standards", []),
                "source": data.get("source", ""),
                "year": data.get("year", "")
            },
            "keywords": ["营养素", "参考摄入量", "DRIs"],
            "body": json.dumps(data.get("standards", []), ensure_ascii=False)
        }
        entries.append(entry)

    # 特定人群膳食指南
    special_path = CHINA_DIETARY / "special_groups.json"
    if special_path.exists():
        data = load_json(special_path)
        for group, content in data.get("groups", {}).items():
            entry = {
                "id": f"dietary_special_{group}",
                "condition_zh": f"特定人群膳食指南-{group}",
                "condition_en": "",
                "category": "dietary_special",
                "regions": ["全国"],
                "dietary": {
                    "type": "special",
                    "group": group,
                    "guidelines": content.get("guidelines", []),
                    "nutrient_needs": content.get("nutrient_needs", {}),
                    "source": data.get("source", ""),
                    "year": data.get("year", "")
                },
                "keywords": ["膳食指南", "特定人群", group],
                "body": f"{group} {json.dumps(content.get('guidelines', []), ensure_ascii=False)}"
            }
            entries.append(entry)

    output_path = OUTPUT / "china_dietary.json"
    save_json(entries, output_path)
    print(f"  已保存 {len(entries)} 条记录到 {output_path}")
    return entries


def main():
    """主函数"""
    print("=" * 60)
    print("开始转换中国医学数据到知识库格式")
    print("=" * 60)

    # 确保输出目录存在
    OUTPUT.mkdir(parents=True, exist_ok=True)

    # 转换各数据源
    convert_china_stats()
    convert_china_clinical_pathways()
    convert_china_cdc()
    convert_china_tcm()
    convert_china_cso()
    convert_china_dietary()

    print("=" * 60)
    print("转换完成!")
    print("=" * 60)


if __name__ == "__main__":
    main()