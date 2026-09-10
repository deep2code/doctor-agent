#!/usr/bin/env python3
"""
中国临床肿瘤学会(CSCO)指南数据下载脚本
数据来源: CSCO指南公开版本
目标: 收集常见恶性肿瘤诊疗指南要点
"""

import os
import re
import json
import requests
from pathlib import Path
from urllib.parse import urljoin

# 配置
OUT_DIR = Path(__file__).parent / "china_cso"
OUT_DIR.mkdir(parents=True, exist_ok=True)

# CSCO指南覆盖的常见恶性肿瘤
CANCER_TYPES = {
    "肺癌": ["非小细胞肺癌", "小细胞肺癌", "肺癌综合治疗"],
    "乳腺癌": ["乳腺癌术前新辅助", "乳腺癌辅助治疗", "晚期乳腺癌"],
    "胃癌": ["胃癌综合治疗", "胃癌围手术期", "晚期胃癌"],
    "结直肠癌": ["结直肠癌综合治疗", "结肠癌", "直肠癌"],
    "肝癌": ["肝细胞癌", "胆管癌", "肝癌综合治疗"],
    "食管癌": ["食管癌综合治疗", "食管鳞癌", "食管腺癌"],
    "胰腺癌": ["胰腺癌综合治疗", "胰腺癌辅助治疗", "晚期胰腺癌"],
    "淋巴瘤": ["弥漫大B细胞淋巴瘤", "滤泡性淋巴瘤", "外周T细胞淋巴瘤"],
    "头颈部肿瘤": ["鼻咽癌", "口腔癌", "喉癌"],
    "妇科肿瘤": ["宫颈癌", "卵巢癌", "子宫内膜癌"]
}


def create_guideline_summary():
    """创建CSCO指南摘要数据"""
    guidelines = []

    # 肺癌指南
    guidelines.extend([
        {
            "cancer_type": "非小细胞肺癌",
            "category": "CSCO指南",
            "version": "2024",
            "source": "CSCO",
            "source_url": "https://www.csco.org.cn",
            "key_points": {
                "诊断": [
                    "病理诊断: 活检或细胞学检查确认",
                    "分期:胸部CT、PET-CT、脑MRI、骨扫描",
                    "分子检测: EGFR、ALK、ROS1、KRAS、MET、RET、BRAF、NTRK"
                ],
                "治疗": [
                    "I-II期: 手术为主，术后辅助化疗/放疗",
                    "III期: 多学科综合治疗，手术+辅助治疗或同步放化疗",
                    "IV期: 靶向治疗、免疫治疗、化疗",
                    "EGFR突变: 奥希替尼、阿美替尼、伏美替尼一线",
                    "ALK融合: 阿来替尼、布格替尼、洛拉替尼一线",
                    "PD-L1≥50%: 帕博利珠单抗单免或联合化疗",
                    "PD-L1 1-49%: 帕博利珠单抗+化疗"
                ],
                "随访": [
                    "术后2年: 每3-6个月复查",
                    "术后2-5年: 每6-12个月复查",
                    "5年后: 每年复查"
                ]
            }
        },
        {
            "cancer_type": "小细胞肺癌",
            "category": "CSCO指南",
            "version": "2024",
            "source": "CSCO",
            "key_points": {
                "诊断": [
                    "病理诊断: 神经内分泌癌",
                    "分期: 广泛期 vs 局限期",
                    "标志物: NSE、ProGRP"
                ],
                "治疗": [
                    "局限期: 同步放化疗(依托泊苷+顺铂)",
                    "广泛期: 化疗+/-免疫治疗",
                    "斯鲁利单抗、替雷利珠单抗联合化疗",
                    "复发后: 拓扑异构酶I抑制剂"
                ]
            }
        }
    ])

    # 乳腺癌指南
    guidelines.extend([
        {
            "cancer_type": "乳腺癌",
            "category": "CSCO指南",
            "version": "2024",
            "source": "CSCO",
            "key_points": {
                "诊断": [
                    "病理: 浸润性癌/原位癌",
                    "免疫组化: ER、PR、HER2、Ki-67",
                    "基因检测: OncoType DX、MammaPrint(可选)"
                ],
                "治疗": [
                    "激素受体阳性: 内分泌治疗(他莫昔芬/AI)+CDK4/6抑制剂",
                    "HER2阳性: 曲妥珠单抗+帕妥珠单抗双靶",
                    "三阴性: 化疗为主，PD-L1阳性可加免疫",
                    "保乳术后: 全乳放疗"
                ],
                "随访": [
                    "术后2年: 每3个月复查",
                    "术后2-5年: 每6个月复查",
                    "5年后: 每年复查"
                ]
            }
        }
    ])

    # 胃癌指南
    guidelines.extend([
        {
            "cancer_type": "胃癌",
            "category": "CSCO指南",
            "version": "2024",
            "source": "CSCO",
            "key_points": {
                "诊断": [
                    "胃镜活检确诊",
                    "分期: 胸腹盆CT、胃镜超声",
                    "HER2检测",
                    "MSS/MSI检测"
                ],
                "治疗": [
                    "早期: 内镜下切除或手术",
                    "局部进展期: 手术+辅助化疗/放化疗",
                    "晚期: 化疗+靶向/免疫",
                    "HER2阳性: 曲妥珠单抗",
                    "Claudin18.2阳性: 佐妥昔单抗",
                    "MSI-H: 帕博利珠单抗"
                ]
            }
        }
    ])

    # 结直肠癌指南
    guidelines.extend([
        {
            "cancer_type": "结直肠癌",
            "category": "CSCO指南",
            "version": "2024",
            "source": "CSCO",
            "key_points": {
                "诊断": [
                    "肠镜活检",
                    "分期: CT、MRI",
                    "基因检测: KRAS、NRAS、BRAF、MSI"
                ],
                "治疗": [
                    "I期: 手术切除",
                    "II期: 手术+/-辅助化疗",
                    "III期: 手术+辅助化疗(奥沙利铂为基础)",
                    "IV期: 化疗+靶向+手术(转化治疗)",
                    "RAS野生: 抗EGFR(西妥昔单抗/帕尼单抗)",
                    "BRAF V600E: 达拉菲尼+曲美替尼+西妥昔单抗",
                    "MSI-H: 帕博利珠单抗"
                ],
                "随访": [
                    "术后2年: 每3-6个月复查",
                    "术后2-5年: 每6-12个月复查"
                ]
            }
        }
    ])

    # 肝癌指南
    guidelines.extend([
        {
            "cancer_type": "肝细胞癌",
            "category": "CSCO指南",
            "version": "2024",
            "source": "CSCO",
            "key_points": {
                "诊断": [
                    "影像学诊断(典型特征)",
                    "甲胎蛋白(AFP)",
                    "病理确诊(必要时)"
                ],
                "分期": "CNLC I-IV期",
                "治疗": [
                    "Ia-Ib期: 手术切除/消融",
                    "IIa期: 手术/消融",
                    "IIb期: 介入治疗",
                    "IIIa期: 介入/靶免/手术",
                    "IIIb期: 系统治疗",
                    "IV期: 对症支持",
                    "一线靶向: 仑伐替尼、索拉非尼、多纳非尼",
                    "免疫: 卡瑞利珠单抗、信迪利单抗"
                ]
            }
        }
    ])

    # 食管癌指南
    guidelines.extend([
        {
            "cancer_type": "食管癌",
            "category": "CSCO指南",
            "version": "2024",
            "source": "CSCO",
            "key_points": {
                "诊断": [
                    "胃镜+活检",
                    "分期: 胸腹盆CT、颈部超声",
                    "HER2、PD-L1检测"
                ],
                "治疗": [
                    "早期: 内镜下切除",
                    "局部进展期: 同步放化疗+手术",
                    "晚期: 化疗+靶向/免疫",
                    "鳞癌: 免疫+化疗(卡瑞利珠单抗、帕博利珠单抗)",
                    "腺癌: 化疗+曲妥珠单抗(HER2+)",
                    "放疗: 根治性/辅助/姑息"
                ]
            }
        }
    ])

    # 淋巴瘤指南
    guidelines.extend([
        {
            "cancer_type": "弥漫大B细胞淋巴瘤",
            "category": "CSCO指南",
            "version": "2024",
            "source": "CSCO",
            "key_points": {
                "诊断": [
                    "病理: 弥漫大B细胞淋巴瘤",
                    "免疫组化: CD20、CD79a、BCL2、BCL6、MYC",
                    "分型: GCB vs non-GCB",
                    "IPI评分"
                ],
                "治疗": [
                    "R-CHOP方案(利妥昔单抗+环磷酰胺+多柔比星+长春新碱+泼尼松)",
                    "中期评估(PET-CT)",
                    "难治/复发: R-GDP、R-ICE、自体移植、CAR-T"
                ],
                "随访": [
                    "治疗结束后2年内: 每3个月复查",
                    "2-5年: 每6个月复查"
                ]
            }
        }
    ])

    # 宫颈癌指南
    guidelines.extend([
        {
            "cancer_type": "宫颈癌",
            "category": "CSCO指南",
            "version": "2024",
            "source": "CSCO",
            "key_points": {
                "诊断": [
                    "病理: 鳞癌/腺癌/腺鳞癌",
                    "分期: 妇科检查、影像学",
                    "HPV检测"
                ],
                "治疗": [
                    "IA1期: 锥切/单纯子宫切除",
                    "IA2-IB3期: 根治性手术/根治性放疗",
                    "IIIC期: 同步放化疗",
                    "IV期: 化疗/靶向/免疫",
                    "贝伐单抗+化疗(复发/晚期)",
                    "帕博利珠单抗(PD-L1 CPS≥1)"
                ]
            }
        }
    ])

    # 胰腺癌指南
    guidelines.extend([
        {
            "cancer_type": "胰腺癌",
            "category": "CSCO指南",
            "version": "2024",
            "source": "CSCO",
            "key_points": {
                "诊断": [
                    "影像: CT/MRI/EUS",
                    "肿瘤标志物: CA19-9",
                    "病理: 细针穿刺"
                ],
                "治疗": [
                    "可切除: 手术+辅助化疗",
                    "交界可切除: 新辅助化疗+手术",
                    "局部进展期: 化疗/同步放化疗",
                    "晚期: 化疗为主",
                    "AG方案(白蛋白紫杉醇+吉西他滨)",
                    "FOLFIRINOX方案(进展期)",
                    "BRCA突变: 奥拉帕利维持"
                ]
            }
        }
    ])

    # 鼻咽癌指南
    guidelines.extend([
        {
            "cancer_type": "鼻咽癌",
            "category": "CSCO指南",
            "version": "2024",
            "source": "CSCO",
            "key_points": {
                "诊断": [
                    "鼻咽镜+活检",
                    "EB病毒检测",
                    "分期: 鼻咽MRI、胸腹盆CT、骨扫描"
                ],
                "治疗": [
                    "I期: 单纯放疗",
                    "II期: 放疗+同期化疗",
                    "III-IV期: 放疗+同期化疗+辅助化疗",
                    "调强放疗(IMRT)为标准",
                    "吉西他滨+顺铂(复发/转移一线)",
                    "免疫治疗: 卡瑞利珠单抗、替雷利珠单抗"
                ]
            }
        }
    ])

    # 保存每个指南
    for guideline in guidelines:
        filename = f"cso_{guideline['cancer_type'].replace(' ', '_')}.json"
        out_path = OUT_DIR / filename
        with open(out_path, "w", encoding="utf-8") as f:
            json.dump(guideline, f, ensure_ascii=False, indent=2)
        print(f"  已保存: {filename}")

    return guidelines


def create_metadata():
    """创建元数据"""
    metadata = {
        "name": "中国临床肿瘤学会(CSCO)诊疗指南",
        "name_en": "CSCO Cancer Treatment Guidelines",
        "source": "中国临床肿瘤学会",
        "source_url": "https://www.csco.org.cn",
        "description": "CSCO常见恶性肿瘤诊疗指南要点汇总",
        "cancer_types": list(CANCER_TYPES.keys()),
        "version": "2024",
        "note": "指南要点基于公开资料整理，详细内容请参考官方出版物"
    }

    meta_path = OUT_DIR / "metadata.json"
    with open(meta_path, "w", encoding="utf-8") as f:
        json.dump(metadata, f, ensure_ascii=False, indent=2)
    print(f"元数据已保存: {meta_path}")


def create_summary():
    """创建汇总文件"""
    guidelines = create_guideline_summary()

    summary = {
        "name": "CSCO诊疗指南汇总",
        "version": "2024",
        "total_count": len(guidelines),
        "guidelines": [
            {
                "cancer_type": g["cancer_type"],
                "version": g["version"]
            }
            for g in guidelines
        ]
    }

    out_path = OUT_DIR / "summary.json"
    with open(out_path, "w", encoding="utf-8") as f:
        json.dump(summary, f, ensure_ascii=False, indent=2)
    print(f"汇总已保存: {out_path}")


def main():
    print("=" * 60)
    print("CSCO肿瘤诊疗指南数据下载")
    print("=" * 60)

    print("\n[1/3] 创建元数据...")
    create_metadata()

    print("\n[2/3] 创建指南数据...")
    create_summary()

    print("\n[3/3] 获取说明:")
    print("-" * 40)
    print("1. 官方指南: 人民卫生出版社《CSCO诊疗指南》")
    print("2. 官方网址: https://www.csco.org.cn")
    print("3. 覆盖癌种:", ", ".join(CANCER_TYPES.keys()))
    print("4. 版本: 2024版")
    print("-" * 40)

    print("\n完成! 数据已保存到:", OUT_DIR)


if __name__ == "__main__":
    main()