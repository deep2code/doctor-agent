#!/usr/bin/env python3
"""
中国居民膳食指南数据下载脚本
数据来源: 中国营养学会
目标: 收集膳食指南核心内容
"""

import os
import re
import json
import requests
from pathlib import Path

# 配置
OUT_DIR = Path(__file__).parent / "china_dietary"
OUT_DIR.mkdir(parents=True, exist_ok=True)


def create_dietary_guidelines():
    """创建膳食指南数据"""

    # 一般人群膳食指南 (2022版)
    general_guidelines = {
        "version": "2022",
        "source": "中国营养学会",
        "source_url": "http://www.cnsoc.org.cn",
        "category": "一般人群膳食指南",
        "guidelines": [
            {
                "number": 1,
                "title": "食物多样，合理搭配",
                "key_points": [
                    "每天的膳食应包括谷薯类、蔬菜水果类、畜禽鱼蛋奶类、大豆坚果类等食物",
                    "平均每天摄入12种以上食物，每周25种以上",
                    "每天摄入谷类食物200-300g，其中全谷物和杂豆类50-150g",
                    "每天摄入薯类50-100g"
                ],
                "practical_tips": [
                    "粗细搭配: 精制米面与全谷物、杂豆搭配",
                    "荤素搭配: 动物性食物与植物性食物配合",
                    "色彩搭配: 多选择不同颜色的食物"
                ]
            },
            {
                "number": 2,
                "title": "吃动平衡，健康体重",
                "key_points": [
                    "各年龄段人群都应天天进行身体活动",
                    "食不过量，保持能量平衡",
                    "每周至少进行5天中等强度身体活动，累计150分钟以上",
                    "鼓励适当进行高强度有氧运动，加强抗阻运动，每周2-3天",
                    "减少久坐时间，每小时起来动一动"
                ],
                "practical_tips": [
                    "主动身体活动每天6000步",
                    "坚持日常身体活动",
                    "特殊人群应在医生指导下运动"
                ]
            },
            {
                "number": 3,
                "title": "多吃蔬果、奶类、全谷、大豆",
                "key_points": [
                    "蔬菜水果是平衡膳食的重要组成部分",
                    "每天摄入蔬菜300-500g，其中深色蔬菜应占1/2",
                    "每天摄入新鲜水果200-350g",
                    "吃各种各样的奶制品，摄入量相当于每天300ml以上液态奶",
                    "经常吃全谷物、豆制品，适量吃坚果"
                ],
                "practical_tips": [
                    "餐餐有蔬菜，深色蔬菜占一半",
                    "天天吃水果，果汁不能代替鲜果",
                    "奶类豆类不能少"
                ]
            },
            {
                "number": 4,
                "title": "适量吃鱼、禽、蛋、瘦肉",
                "key_points": [
                    "鱼、禽、蛋类和瘦肉摄入要适量，平均每天120-200g",
                    "每周最好吃鱼2次或300-500g，蛋类300-350g，畜禽肉300-500g",
                    "少吃深加工肉制品",
                    "优先选择鱼，少吃肥肉、烟熏和腌制肉制品"
                ],
                "practical_tips": [
                    "吃鸡蛋不弃蛋黄",
                    "少吃肥肉、烟熏和腌制肉制品"
                ]
            },
            {
                "number": 5,
                "title": "少盐少油，控糖限酒",
                "key_points": [
                    "培养清淡饮食习惯，少吃高盐和油炸食品",
                    "成年人每天摄入食盐不超过5g，烹调油25-30g",
                    "控制添加糖的摄入量，每天不超过50g，最好控制在25g以下",
                    "儿童青少年、孕妇、乳母以及慢性病患者不应饮酒",
                    "成年人如饮酒，一天饮用的酒精量不超过15g"
                ],
                "practical_tips": [
                    "选用定量盐勺、油壶",
                    "减少在外就餐",
                    "少喝含糖饮料"
                ]
            },
            {
                "number": 6,
                "title": "规律进餐，足量饮水",
                "key_points": [
                    "合理安排一日三餐，定时定量，不漏食",
                    "天天吃早餐，保证营养充足",
                    "午餐要吃好，晚餐要适量",
                    "足量饮水，少量多次",
                    "在温和气候条件下，低身体活动水平的成年人每天饮水1500-1700ml"
                ],
                "practical_tips": [
                    "主动喝水，少量多次",
                    "推荐喝白水或茶水",
                    "不用饮料代替白水"
                ]
            },
            {
                "number": 7,
                "title": "会烹会选，会看标签",
                "key_points": [
                    "选择新鲜的、营养素密度高的食物",
                    "学会阅读食品营养标签，选择预包装食品时关注营养成分表",
                    "了解食物营养特点，学会合理搭配",
                    "学习烹饪技巧，传承中华民族优秀饮食文化"
                ],
                "practical_tips": [
                    "看配料表和营养成分表",
                    "选择低盐、低糖、低脂产品"
                ]
            },
            {
                "number": 8,
                "title": "公筷分餐，杜绝浪费",
                "key_points": [
                    "选择新鲜卫生的食物，不食野生动物",
                    "食物生熟分开，熟食二次加热要热透",
                    "讲究卫生，从我做起，饭前便后用流动水洗手",
                    "珍惜食物，按需备餐，提倡分餐不浪费"
                ],
                "practical_tips": [
                    "使用公勺公筷",
                    "按需备餐，珍惜食物"
                ]
            }
        ]
    }

    # 特定人群膳食指南
    special_groups = {
        "孕妇和乳母": {
            "孕期": [
                "孕早期: 与备孕期膳食相同，无需额外增加能量",
                "孕中期: 每天增加能量300kcal，蛋白质15g",
                "孕晚期: 每天增加能量450kcal，蛋白质30g"
            ],
            "关键营养素": [
                "叶酸: 整个孕期每天补充400μg",
                "铁: 孕中期24mg/d，孕晚期29mg/d",
                "钙: 1000mg/d",
                "碘: 230μg/d"
            ],
            "体重管理": [
                "孕前体重正常者: 孕期增重11.5-16kg",
                "孕前低体重者: 孕期增重12.5-18kg",
                "孕前超重者: 孕期增重7-11.5kg",
                "孕前肥胖者: 孕期增重5-9kg"
            ]
        },
        "婴幼儿": {
            "0-6月龄": [
                "纯母乳喂养",
                "出生后数日开始补充维生素D 400IU/d",
                "母乳充足的婴儿不需要额外喂水"
            ],
            "7-24月龄": [
                "继续母乳喂养，6月龄起添加辅食",
                "从富铁泥糊状食物开始，逐步添加多样",
                "辅食不加或少加盐、糖和调味品",
                "继续补充维生素D，400IU/d"
            ]
        },
        "儿童青少年": {
            "学龄前儿童(2-5岁)": [
                "每天3餐2点",
                "每天饮奶300-500ml",
                "足量饮水600-800ml",
                "规律运动，每天至少180分钟"
            ],
            "学龄儿童(6-17岁)": [
                "三餐合理，规律进餐",
                "每天饮奶300ml以上",
                "足量饮水800-1400ml",
                "每天累计至少60分钟中等强度运动"
            ]
        },
        "老年人": {
            "一般老年人(65-79岁)": [
                "食物多样，品种丰富",
                "积极户外活动，延缓肌肉衰减",
                "维持适宜体重",
                "摄入充足蛋白质: 1.0-1.5g/(kg·d)",
                "补充维生素D和钙"
            ],
            "高龄老年人(80岁以上)": [
                "食物细软，少量多餐",
                "优质蛋白质: 1.2-1.5g/(kg·d)",
                "补充维生素D和钙",
                "在医生指导下使用营养补充剂"
            ]
        }
    }

    # 平衡膳食模式
    balanced_diet = {
        "daily_structure": {
            "谷薯类": "200-300g",
            "蔬菜类": "300-500g",
            "水果类": "200-350g",
            "畜禽肉": "40-75g",
            "水产品": "40-75g",
            "蛋类": "40-50g",
            "奶类": "300-500ml",
            "大豆": "25-35g",
            "坚果": "10-15g",
            "烹调油": "25-30g",
            "盐": "<5g",
            "水": "1500-1700ml"
        },
        "meal_distribution": {
            "早餐": "25-30%",
            "午餐": "30-40%",
            "晚餐": "30-40%",
            "加餐": "可安排在两餐之间"
        }
    }

    # 保存数据
    with open(OUT_DIR / "general_guidelines.json", "w", encoding="utf-8") as f:
        json.dump(general_guidelines, f, ensure_ascii=False, indent=2)
    print("  已保存: general_guidelines.json")

    with open(OUT_DIR / "special_groups.json", "w", encoding="utf-8") as f:
        json.dump(special_groups, f, ensure_ascii=False, indent=2)
    print("  已保存: special_groups.json")

    with open(OUT_DIR / "balanced_diet.json", "w", encoding="utf-8") as f:
        json.dump(balanced_diet, f, ensure_ascii=False, indent=2)
    print("  已保存: balanced_diet.json")

    return {
        "general": general_guidelines,
        "special": special_groups,
        "balanced": balanced_diet
    }


def create_nutrition_standards():
    """创建营养素参考摄入量数据"""

    # 中国居民膳食营养素参考摄入量 (DRIs) 摘要
    dris = {
        "energy": {
            "成年男性": "2250-2400 kcal/d",
            "成年女性": "1800-1900 kcal/d",
            "孕妇(中晚期)": "+300-450 kcal/d",
            "乳母": "+500 kcal/d"
        },
        "protein": {
            "成年男性": "60g/d",
            "成年女性": "50g/d",
            "孕妇(中晚期)": "+15-30g/d",
            "乳母": "+25g/d",
            "老年人": "1.0-1.5g/(kg·d)"
        },
        "fat": {
            "占总能量": "20-30%",
            "饱和脂肪酸": "<10%",
            "多不饱和脂肪酸": "6-11%"
        },
        "carbohydrates": {
            "占总能量": "50-65%",
            "添加糖": "<25g/d"
        },
        "key_micronutrients": {
            "钙": {
                "成人": "800mg/d",
                "孕妇": "1000mg/d",
                "老年人": "1000mg/d"
            },
            "铁": {
                "成年男性": "12mg/d",
                "成年女性(18-49岁)": "20mg/d",
                "孕妇(中晚期)": "24-29mg/d",
                "老年人": "12mg/d"
            },
            "锌": {
                "成年男性": "12.5mg/d",
                "成年女性": "7.5mg/d",
                "孕妇": "9.5mg/d"
            },
            "碘": {
                "成人": "120μg/d",
                "孕妇": "230μg/d",
                "乳母": "240μg/d"
            },
            "维生素A": {
                "成年男性": "800μg RAE/d",
                "成年女性": "700μg RAE/d",
                "孕妇": "770μg RAE/d"
            },
            "维生素D": {
                "成人": "10μg/d(400IU)",
                "老年人": "15μg/d(600IU)",
                "孕妇": "10μg/d(400IU)"
            },
            "叶酸": {
                "成人": "400μg DFE/d",
                "孕妇": "600μg DFE/d"
            }
        }
    }

    with open(OUT_DIR / "nutrition_standards.json", "w", encoding="utf-8") as f:
        json.dump(dris, f, ensure_ascii=False, indent=2)
    print("  已保存: nutrition_standards.json")

    return dris


def create_food_guide():
    """创建中国居民膳食宝塔"""

    food_pyramid = {
        "name": "中国居民平衡膳食宝塔",
        "version": "2022",
        "source": "中国营养学会",
        "levels": [
            {
                "level": 1,
                "name": "谷薯类",
                "amount": "200-300g",
                "recommendations": "全谷物和杂豆50-150g，薯类50-100g"
            },
            {
                "level": 2,
                "name": "蔬菜水果类",
                "amount": "300-500g蔬菜+200-350g水果",
                "recommendations": "深色蔬菜占1/2，天天吃水果"
            },
            {
                "level": 3,
                "name": "动物性食物",
                "amount": "120-200g",
                "recommendations": "每天一个蛋，适量选择鱼禽肉"
            },
            {
                "level": 4,
                "name": "奶类大豆坚果",
                "amount": "300-500ml奶+25-35g大豆+10-15g坚果",
                "recommendations": "吃各种各样的奶制品"
            },
            {
                "level": 5,
                "name": "烹调油盐",
                "amount": "25-30g油+<5g盐",
                "recommendations": "培养清淡口味"
            },
            {
                "level": 6,
                "name": "身体活动和水",
                "amount": "1500-1700ml水+6000步",
                "recommendations": "主动身体活动"
            }
        ]
    }

    with open(OUT_DIR / "food_pyramid.json", "w", encoding="utf-8") as f:
        json.dump(food_pyramid, f, ensure_ascii=False, indent=2)
    print("  已保存: food_pyramid.json")

    return food_pyramid


def create_metadata():
    """创建元数据"""
    metadata = {
        "name": "中国居民膳食指南",
        "name_en": "Chinese Dietary Guidelines",
        "source": "中国营养学会",
        "source_url": "http://www.cnsoc.org.cn",
        "version": "2022",
        "description": "中国居民膳食指南(2022)核心内容",
        "categories": [
            "一般人群膳食指南",
            "特定人群膳食指南",
            "平衡膳食模式",
            "膳食营养素参考摄入量"
        ],
        "key_recommendations": [
            "食物多样，合理搭配",
            "吃动平衡，健康体重",
            "多吃蔬果、奶类、全谷、大豆",
            "适量吃鱼、禽、蛋、瘦肉",
            "少盐少油，控糖限酒",
            "规律进餐，足量饮水",
            "会烹会选，会看标签",
            "公筷分餐，杜绝浪费"
        ]
    }

    meta_path = OUT_DIR / "metadata.json"
    with open(meta_path, "w", encoding="utf-8") as f:
        json.dump(metadata, f, ensure_ascii=False, indent=2)
    print(f"元数据已保存: {meta_path}")


def create_summary():
    """创建汇总文件"""
    dietary = create_dietary_guidelines()
    standards = create_nutrition_standards()
    pyramid = create_food_guide()

    summary = {
        "name": "中国居民膳食指南数据汇总",
        "version": "2022",
        "total_files": 5,
        "files": [
            "general_guidelines.json - 一般人群膳食指南",
            "special_groups.json - 特定人群膳食指南",
            "balanced_diet.json - 平衡膳食模式",
            "nutrition_standards.json - 营养素参考摄入量",
            "food_pyramid.json - 膳食宝塔"
        ]
    }

    out_path = OUT_DIR / "summary.json"
    with open(out_path, "w", encoding="utf-8") as f:
        json.dump(summary, f, ensure_ascii=False, indent=2)
    print(f"汇总已保存: {out_path}")


def main():
    print("=" * 60)
    print("中国居民膳食指南数据下载")
    print("=" * 60)

    print("\n[1/4] 创建元数据...")
    create_metadata()

    print("\n[2/4] 创建膳食指南数据...")
    create_dietary_guidelines()

    print("\n[3/4] 创建营养素参考摄入量...")
    create_nutrition_standards()

    print("\n[4/4] 创建膳食宝塔...")
    create_food_guide()

    print("\n[5/5] 创建汇总...")
    create_summary()

    print("\n" + "=" * 60)
    print("完成! 数据已保存到:", OUT_DIR)
    print("=" * 60)


if __name__ == "__main__":
    main()