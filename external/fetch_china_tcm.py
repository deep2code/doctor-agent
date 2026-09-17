#!/usr/bin/env python3
"""
中国中医药知识库下载脚本
数据来源: 公开的中医药数据、药典、方剂等
目标: 下载中医药知识、药典、方剂等数据
"""

import os
import re
import json
import time
import requests
from pathlib import Path
from urllib.parse import urljoin

# 配置
OUT_DIR = Path(__file__).parent / "china_tcm"
OUT_DIR.mkdir(parents=True, exist_ok=True)

# 中医药知识分类
TCM_CATEGORIES = {
    "中药": [
        "清热药", "温里药", "理气药", "活血化瘀药", "补益药",
        "安神药", "平肝息风药", "收涩药", "泻下药", "驱虫药"
    ],
    "方剂": [
        "解表剂", "泻下剂", "和解剂", "清热剂", "温里剂",
        "补益剂", "固涩剂", "安神剂", "理气剂", "理血剂"
    ],
    "经络": [
        "十二正经", "奇经八脉", "十五络脉", "十二经别"
    ],
    "穴位": [
        "头颈部穴位", "胸腹部穴位", "背腰部穴位", "四肢穴位"
    ]
}

# 公开的中医药数据源
# 1. 国家药典委员会 - 药典
# 2. 中医药数据平台
# 3. 公开的中医药数据库

# 中药数据结构
HERB_STRUCTURE = {
    "name": "中药名称",
    "name_zhuyin": "拼音",
    "name_latin": "拉丁名",
    "category": "分类",
    "properties": "性味归经",
    "effects": "功效",
    "indications": "主治",
    "dosage": "用法用量",
    "contraindications": "禁忌",
    "source": "来源"
}


def fetch_url(url, timeout=30):
    """获取URL内容"""
    headers = {
        "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
    }
    try:
        resp = requests.get(url, headers=headers, timeout=timeout)
        resp.raise_for_status()
        return resp.text
    except Exception as e:
        print(f"  获取失败: {url} - {e}")
        return None


def create_sample_herbs():
    """创建示例中药数据"""
    herbs = [
        {
            "name": "人参",
            "name_pinyin": "renshen",
            "name_latin": "Panax ginseng C.A.Mey.",
            "category": "补虚药",
            "properties": "甘、微苦，微温。归脾、肺、心、肾经。",
            "effects": "大补元气，复脉固脱，补脾益肺，生津养血，安神益智。",
            "indications": "体虚欲脱，肢冷脉微，脾虚食少，肺虚喘咳，津伤口渴，内热消渴，气血亏虚，久病虚羸，惊悸失眠，阳痿宫冷。",
            "dosage": "3-9g，另煎兑服；野山参研末吞服，每次2g，日服2次。",
            "contraindications": "不宜与藜芦、五灵脂同用。",
            "source": "《中国药典》2020年版"
        },
        {
            "name": "黄芪",
            "name_pinyin": "huangqi",
            "name_latin": "Astragalus membranaceus (Fisch.) Bge.",
            "category": "补虚药",
            "properties": "甘，微温。归肺、脾经。",
            "effects": "补气升阳，固表止汗，利水消肿，生津养血，行滞通痹，托毒排脓，敛疮生肌。",
            "indications": "气虚乏力，食少便溏，中气下陷，久泻脱肛，便血崩漏，表虚自汗，气虚水肿，内热消渴，血虚萎黄，气血两虚，痹痛麻木，半身不遂，痈疽难溃，久溃不敛。",
            "dosage": "9-30g。",
            "contraindications": "表实邪盛、气滞湿阻、食积停滞、痈疽初起或溃后热毒尚盛等实证，以及阴虚阳亢者，均须慎用。",
            "source": "《中国药典》2020年版"
        },
        {
            "name": "当归",
            "name_pinyin": "danggui",
            "name_latin": "Angelica sinensis (Oliv.) Diels",
            "category": "补血药",
            "properties": "甘、辛，温。归肝、心、脾经。",
            "effects": "补血活血，调经止痛，润肠通便。",
            "indications": "血虚萎黄，眩晕心悸，月经不调，经闭痛经，虚寒腹痛，瘀血作痛，跌打损伤，痹痛麻木，肠燥便秘。",
            "dosage": "6-12g。",
            "contraindications": "湿盛中满、大便泄泻者忌服。",
            "source": "《中国药典》2020年版"
        },
        {
            "name": "金银花",
            "name_pinyin": "jinyinhua",
            "name_latin": "Lonicera japonica Thunb.",
            "category": "清热药",
            "properties": "甘，寒。归肺、心、胃经。",
            "effects": "清热解毒，疏散风热。",
            "indications": "痈肿疔疮，喉痹，丹毒，热毒血痢，风热感冒，温病发热。",
            "dosage": "6-15g。",
            "contraindications": "脾胃虚寒及疮疡属阴证者慎用。",
            "source": "《中国药典》2020年版"
        },
        {
            "name": "板蓝根",
            "name_pinyin": "banlangen",
            "name_latin": "Isatis indigotica Fort.",
            "category": "清热药",
            "properties": "苦，寒。归心、胃经。",
            "effects": "清热解毒，凉血利咽。",
            "indications": "温疫时毒，发热咽痛，温毒发斑，痄腮，烂喉丹痧，大头瘟疫，丹毒，痈肿。",
            "dosage": "9-15g。",
            "contraindications": "脾胃虚寒者慎用。",
            "source": "《中国药典》2020年版"
        },
        {
            "name": "连翘",
            "name_pinyin": "lianqiao",
            "name_latin": "Forsythia suspensa (Thunb.) Vahl",
            "category": "清热药",
            "properties": "苦，微寒。归肺、心、小肠经。",
            "effects": "清热解毒，消肿散结，疏散风热。",
            "indications": "痈疽，瘰疬，乳痈，丹毒，风热感冒，温病初起，温热入营，高热烦渴，神昏发斑，热淋涩痛。",
            "dosage": "6-15g。",
            "contraindications": "脾胃虚寒及疮疡属阴证者慎用。",
            "source": "《中国药典》2020年版"
        },
        {
            "name": "柴胡",
            "name_pinyin": "chaihu",
            "name_latin": "Bupleurum chinense DC.",
            "category": "解表药",
            "properties": "苦、辛，微寒。归肝、胆、肺经。",
            "effects": "疏肝解郁，升举阳气，退热解表。",
            "indications": "感冒发热，寒热往来，肝郁气滞，胸胁胀痛，脏器下垂，脱肛，子宫脱垂，月经不调。",
            "dosage": "3-10g。",
            "contraindications": "肝阳上亢、阴虚阳亢者慎用。",
            "source": "《中国药典》2020年版"
        },
        {
            "name": "川芎",
            "name_pinyin": "chuanxiong",
            "name_latin": "Ligusticum chuanxiong Hort.",
            "category": "活血化瘀药",
            "properties": "辛，温。归肝、胆、心包经。",
            "effects": "活血行气，祛风止痛。",
            "indications": "血瘀气滞痛证，头痛，风湿痹痛，月经不调，经闭痛经，癥瘕腹痛，胸胁刺痛，跌打损伤。",
            "dosage": "3-10g。",
            "contraindications": "阴虚火旺、舌红口干者慎用。",
            "source": "《中国药典》2020年版"
        },
        {
            "name": "丹参",
            "name_pinyin": "danshen",
            "name_latin": "Salvia miltiorrhiza Bge.",
            "category": "活血化瘀药",
            "properties": "苦，微寒。归心、肝经。",
            "effects": "活血祛瘀，通经止痛，清心除烦，凉血消痈。",
            "indications": "瘀血阻滞之痛证，经闭痛经，癥瘕积聚，胸痹心痛，心腹疼痛，热痹疼痛，疮疡肿痛，心烦不眠。",
            "dosage": "10-15g。",
            "contraindications": "不宜与藜芦同用。",
            "source": "《中国药典》2020年版"
        },
        {
            "name": "枸杞子",
            "name_pinyin": "gouqizi",
            "name_latin": "Lycium barbarum L.",
            "category": "补虚药",
            "properties": "甘，平。归肝、肾、肺经。",
            "effects": "滋补肝肾，益精明目，润肺止咳。",
            "indications": "肝肾不足，腰膝酸软，头晕目眩，视力减退，遗精，消渴，阴虚咳嗽。",
            "dosage": "6-12g。",
            "contraindications": "外感发热、脾虚便溏者慎用。",
            "source": "《中国药典》2020年版"
        }
    ]

    # 保存每味中药
    for herb in herbs:
        filename = f"herb_{herb['name_pinyin']}.json"
        out_path = OUT_DIR / filename
        with open(out_path, "w", encoding="utf-8") as f:
            json.dump(herb, f, ensure_ascii=False, indent=2)
        print(f"  已保存: {filename}")

    return herbs


def create_sample_formulas():
    """创建示例方剂数据"""
    formulas = [
        {
            "name": "四君子汤",
            "name_pinyin": "sijunzi tang",
            "category": "补益剂",
            "composition": "人参9g 白术9g 茯苓9g 甘草6g",
            "source": "《太平惠民和剂局方》",
            "indications": "脾胃气虚证。面色萎白，语声低微，气短乏力，食少便溏，舌淡苔白，脉虚数。",
            "effects": "益气健脾。",
            "dosage": "水煎服，日一剂，分二次温服。",
            "modifications": [
                "加陈皮为异功散",
                "加半夏、陈皮为六君子汤",
                "再加木香、砂仁为香砂六君子汤"
            ]
        },
        {
            "name": "四物汤",
            "name_pinyin": "siwu tang",
            "category": "补血剂",
            "composition": "当归9g 川芎6g 白芍9g 熟地黄12g",
            "source": "《太平惠民和剂局方》",
            "indications": "营血虚滞证。头晕心悸，面色无华，妇人月经不调，量少或经闭不行，舌淡，脉细弦或细涩。",
            "effects": "补血调血。",
            "dosage": "水煎服，日一剂，分二次温服。",
            "modifications": [
                "加桃仁、红花为桃红四物汤",
                "合四君子为八珍汤",
                "再加黄芪、肉桂为十全大补汤"
            ]
        },
        {
            "name": "银翘散",
            "name_pinyin": "yinqiao san",
            "category": "解表剂",
            "composition": "金银花30g 连翘30g 桔梗18g 薄荷18g 竹叶12g 生甘草15g 荆芥穗12g 淡豆豉15g 牛蒡子18g",
            "source": "《温病条辨》",
            "indications": "温病初起。发热无汗，或有汗不畅，微恶风寒，头痛口渴，咳嗽咽痛，舌尖红，苔薄白或薄黄，脉浮数。",
            "effects": "辛凉透表，清热解毒。",
            "dosage": "共为散，每服18g，鲜芦根汤煎，香气大出，即取服，勿过煎。",
            "modifications": [
                "热重加石膏、知母",
                "渴甚加天花粉",
                "咽喉肿痛加马勃、玄参"
            ]
        },
        {
            "name": "藿香正气散",
            "name_pinyin": "huoxiang zhengqi san",
            "category": "和解剂",
            "composition": "大腹皮9g 白芷9g 紫苏9g 茯苓9g 半夏曲9g 白术9g 厚朴12g 桔梗12g 藿香18g 甘草12g",
            "source": "《太平惠民和剂局方》",
            "indications": "外感风寒，内伤湿滞证。恶寒发热，头痛，胸膈满闷，脘腹疼痛，恶心呕吐，肠鸣泄泻，舌苔白腻，脉浮或濡缓。",
            "effects": "解表化湿，理气和中。",
            "dosage": "水煎服，日一剂，分二次温服。",
            "modifications": [
                "改用藿香正气丸、藿香正气水、藿香正气胶囊等中成药"
            ]
        },
        {
            "name": "血府逐瘀汤",
            "name_pinyin": "xuefu zhuyu tang",
            "category": "理血剂",
            "composition": "桃仁12g 红花9g 当归9g 生地黄9g 川芎6g 赤芍6g 牛膝9g 桔梗5g 柴胡3g 枳壳6g 甘草3g",
            "source": "《医林改错》",
            "indications": "胸中血瘀证。胸痛，头痛日久，痛如针刺而有定处，或呃逆干呕，或心悸失眠，或急躁易怒，入暮潮热，唇暗或两目暗黑，舌质暗红或有瘀斑，脉涩或弦紧。",
            "effects": "活血化瘀，行气止痛。",
            "dosage": "水煎服，日一剂，分二次温服。",
            "modifications": [
                "合桂枝茯苓丸加减",
                "加三七、丹参"
            ]
        },
        {
            "name": "补中益气汤",
            "name_pinyin": "buzhong yiqi tang",
            "category": "补益剂",
            "composition": "黄芪18g 甘草9g 人参9g 当归6g 橘皮6g 升麻6g 柴胡6g 白术9g",
            "source": "《脾胃论》",
            "indications": "1. 脾胃气虚证。食少便溏，体倦肢软，少气懒言，面色萎黄，脉虚弱。2. 气虚下陷证。脱肛，子宫脱垂，久泻久痢，崩漏等。3. 气虚发热证。身热自汗，渴喜热饮，气短乏力，脉虚大无力。",
            "effects": "补中益气，升阳举陷。",
            "dosage": "水煎服，日一剂，分二次温服。",
            "modifications": [
                "加枳壳、陈皮为调中益气汤",
                "加重升麻、柴胡用量增强升提之力"
            ]
        },
        {
            "name": "六味地黄丸",
            "name_pinyin": "liuwei dihuang wan",
            "category": "补益剂",
            "composition": "熟地黄24g 山萸肉12g 山药12g 泽泻9g 牡丹皮9g 茯苓9g",
            "source": "《小儿药证直诀》",
            "indications": "肾阴虚证。腰膝酸软，头晕目眩，耳鸣耳聋，盗汗，遗精，消渴，骨蒸潮热，手足心热，舌燥咽痛，牙齿动摇，足跟作痛，以及小儿囟门不合，舌红少苔，脉沉细数。",
            "effects": "滋阴补肾。",
            "dosage": "蜜丸，每丸约9g，每次1丸，日2次口服。",
            "modifications": [
                "加枸杞子、菊花为杞菊地黄丸",
                "加知母、黄柏为知柏地黄丸",
                "加五味子为都气丸",
                "加麦冬、五味子为麦味地黄丸"
            ]
        },
        {
            "name": "小柴胡汤",
            "name_pinyin": "xiaochaihu tang",
            "category": "和解剂",
            "composition": "柴胡24g 黄芩9g 人参9g 甘草9g 半夏9g 生姜9g 大枣4枚",
            "source": "《伤寒论》",
            "indications": "1. 伤寒少阳证。寒热往来，胸胁苦满，默默不欲饮食，心烦喜呕，口苦，咽干，目眩，舌苔薄白，脉弦。2. 妇人伤寒热入血室，以及疟疾、黄疸等杂病见少阳证者。",
            "effects": "和解少阳。",
            "dosage": "水煎服，日一剂，分二次温服。",
            "modifications": [
                "去人参、甘草加枳壳、桔梗为柴胡枳桔汤",
                "加龙骨、牡蛎为柴胡加龙骨牡蛎汤"
            ]
        }
    ]

    # 保存每个方剂
    for formula in formulas:
        filename = f"formula_{formula['name_pinyin'].replace(' ', '_')}.json"
        out_path = OUT_DIR / filename
        with open(out_path, "w", encoding="utf-8") as f:
            json.dump(formula, f, ensure_ascii=False, indent=2)
        print(f"  已保存: {filename}")

    return formulas


def create_sample_acupoints():
    """创建示例穴位数据"""
    acupoints = [
        {
            "name": "足三里",
            "name_pinyin": "zusanli",
            "code": "ST36",
            "category": "足阳明胃经",
            "location": "在小腿外侧，犊鼻下3寸，犊鼻与解溪连线上。",
            "indications": "胃痛，呕吐，噎膈，腹胀，泄泻，痢疾，便秘，肠痈，下肢痿痹，心悸，气短，癫狂，失眠，虚劳羸瘦。",
            "effects": "调理脾胃，补中益气，通经活络，疏风化湿。",
            "moxibustion": "艾灸5-10分钟。",
            "category_secondary": "强壮穴"
        },
        {
            "name": "合谷",
            "name_pinyin": "hegu",
            "code": "LI4",
            "category": "手阳明大肠经",
            "location": "在手背，第2掌骨桡侧的中点处。",
            "indications": "头痛，眩晕，牙痛，口眼歪斜，咽喉肿痛，腹痛，便秘，热病，无汗或多汗，闭经，滞产。",
            "effects": "疏风解表，通络镇痛。",
            "moxibustion": "艾灸3-5分钟。",
            "category_secondary": "四总穴"
        },
        {
            "name": "内关",
            "name_pinyin": "neiguan",
            "code": "PC6",
            "category": "手厥阴心包经",
            "location": "在前臂掌侧，腕横纹上2寸，掌长肌腱与桡侧腕屈肌腱之间。",
            "indications": "心痛，胸闷，心悸，烦躁，失眠，眩晕，呕吐，呃逆，肘臂挛痛。",
            "effects": "宁心安神，理气止痛。",
            "moxibustion": "艾灸5-10分钟。",
            "category_secondary": "八脉交会穴"
        },
        {
            "name": "三阴交",
            "name_pinyin": "sanyinjiao",
            "code": "SP6",
            "category": "足太阴脾经",
            "location": "在内踝尖上3寸，胫骨内侧缘后方。",
            "indications": "肠鸣腹胀，泄泻，月经不调，带下，阴挺，不孕，滞产，遗精，阳痿，遗尿，疝气，失眠，下肢痿痹。",
            "effects": "健脾和胃，调补肝肾，行气活血，疏经通络。",
            "moxibustion": "艾灸5-10分钟。",
            "category_secondary": "妇科要穴"
        },
        {
            "name": "百会",
            "name_pinyin": "baihui",
            "code": "GV20",
            "category": "督脉",
            "location": "在头部，前发际正中直上5寸。",
            "indications": "头痛，眩晕，中风失语，失眠，健忘，癫狂，痫证，脱肛，阴挺，胃下垂，肾下垂。",
            "effects": "升阳固脱，醒脑开窍。",
            "moxibustion": "艾灸5-10分钟。",
            "category_secondary": "百脉之会"
        },
        {
            "name": "风池",
            "name_pinyin": "fengchi",
            "code": "GB20",
            "category": "足少阳胆经",
            "location": "在项部，枕骨之下，与风府相平，胸锁乳突肌与斜方肌上端之间的凹陷处。",
            "indications": "头痛，眩晕，颈项强痛，感冒，发热，鼻塞，鼽衄，目赤肿痛，耳鸣，失眠，中风。",
            "effects": "祛风解表，清头明目。",
            "moxibustion": "艾灸3-5分钟。",
            "category_secondary": "风邪要穴"
        }
    ]

    # 保存每个穴位
    for point in acupoints:
        filename = f"acupoint_{point['name_pinyin']}.json"
        out_path = OUT_DIR / filename
        with open(out_path, "w", encoding="utf-8") as f:
            json.dump(point, f, ensure_ascii=False, indent=2)
        print(f"  已保存: {filename}")

    return acupoints


def create_metadata():
    """创建元数据文件"""
    metadata = {
        "name": "中国中医药知识库",
        "name_en": "Traditional Chinese Medicine Knowledge Base",
        "source": "《中国药典》2020年版、《太平惠民和剂局方》、《温病条辨》等",
        "description": "包含中药、方剂、经络穴位等中医药知识",
        "categories": TCM_CATEGORIES,
        "data_types": {
            "中药": {
                "fields": list(HERB_STRUCTURE.keys()),
                "count": 10
            },
            "方剂": {
                "fields": ["name", "name_pinyin", "category", "composition", "source", "indications", "effects", "dosage", "modifications"],
                "count": 8
            },
            "穴位": {
                "fields": ["name", "name_pinyin", "code", "category", "location", "indications", "effects", "moxibustion", "category_secondary"],
                "count": 6
            }
        },
        "note": "示例数据，基于公开中医药典籍"
    }

    meta_path = OUT_DIR / "metadata.json"
    with open(meta_path, "w", encoding="utf-8") as f:
        json.dump(metadata, f, ensure_ascii=False, indent=2)
    print(f"元数据已保存: {meta_path}")


def create_summary():
    """创建汇总文件"""
    herbs = create_sample_herbs()
    formulas = create_sample_formulas()
    acupoints = create_sample_acupoints()

    summary = {
        "name": "中国中医药知识库汇总",
        "version": "2024-09",
        "total": {
            "herbs": len(herbs),
            "formulas": len(formulas),
            "acupoints": len(acupoints)
        },
        "categories": {
            "中药": "常用中药",
            "方剂": "经典方剂",
            "穴位": "针灸穴位"
        }
    }

    out_path = OUT_DIR / "summary.json"
    with open(out_path, "w", encoding="utf-8") as f:
        json.dump(summary, f, ensure_ascii=False, indent=2)
    print(f"汇总已保存: {out_path}")


def main():
    print("=" * 60)
    print("中国中医药知识库下载")
    print("=" * 60)

    print("\n[1/4] 创建元数据...")
    create_metadata()

    print("\n[2/4] 创建中药数据...")
    create_sample_herbs()

    print("\n[3/4] 创建方剂数据...")
    create_sample_formulas()

    print("\n[4/4] 创建穴位数据...")
    create_sample_acupoints()

    print("\n[5/5] 数据获取说明:")
    print("-" * 40)
    print("1. 官方数据源:")
    print("   - 国家药典委员会: www.chp.org.cn")
    print("   - 国家中医药管理局: satcm.gov.cn")
    print("2. 公开数据:")
    print("   - 《中国药典》2020年版")
    print("   - 《太平惠民和剂局方》")
    print("   - 《伤寒论》《金匮要略》")
    print("   - 《温病条辨》")
    print("3. 数据扩展:")
    print("   - 可从第三方中医药数据库扩展")
    print("   - 可从PubMed获取中医药研究文献")
    print("-" * 40)

    print("\n完成! 数据已保存到:", OUT_DIR)


if __name__ == "__main__":
    main()