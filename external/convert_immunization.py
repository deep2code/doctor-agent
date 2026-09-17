#!/usr/bin/env python3
"""Convert the official 国家免疫规划疫苗儿童免疫程序及说明（2021年版）
(NHC, published 2021-02) into internal/knowledge/data/china_vaccines.json —
KnowledgeEntry entries (category="vaccine_cn"), same shape as who_vaccines.json.

Source text: external/immunization/china-epi-program-2021.txt extracted from the
official PDF (广东省疾控中心 mirror):
https://cdcp.gd.gov.cn/attachment/0/416/416672/3442966.pdf

Entries follow the document structure: one overview entry (program table +
general principles), one per vaccine family (HepB/BCG/polio/DTaP+DT/MMR/JE/
meningococcal/HepA), and one for special-health-state children. All content is
hand-curated from the official text; no inference.
"""
import json
import sys
from pathlib import Path

ROOT = Path(__file__).parent.parent
OUT = ROOT / "internal" / "knowledge" / "data" / "china_vaccines.json"

CITE = [{
    "type": "national_guideline",
    "title": "国家免疫规划疫苗儿童免疫程序及说明（2021年版）",
    "journal": "",  # institutional publication — leave empty (verify rule)
    "year": 2021,
    "doi": "",
    "pmid": "",
    "level": "national_guideline",
    "url": "https://cdcp.gd.gov.cn/attachment/0/416/416672/3442966.pdf",
}]

ENTRIES = [
    {
        "id": "cn-vaccine-overview",
        "condition_zh": "国家免疫规划疫苗儿童免疫程序（总览）",
        "condition_en": "China national immunization schedule (overview)",
        "category": "vaccine_cn",
        "regions": ["全国"],
        "diagnosis": {"lab_tests": [], "clinical_features": ["0-6 岁儿童按程序接种"], "gold_standard": ""},
        "treatment": [
            {"method": "乙肝疫苗（HepB）：出生时、1月龄、6月龄各 1 剂（第1剂出生后24小时内）", "indication": "新生儿", "evidence_level": "national_guideline"},
            {"method": "卡介苗（BCG）：出生时 1 剂", "indication": "新生儿", "evidence_level": "national_guideline"},
            {"method": "脊灰疫苗：2月龄、3月龄 IPV 各 1 剂，4月龄、4周岁 bOPV 各 1 剂", "indication": "儿童", "evidence_level": "national_guideline"},
            {"method": "百白破疫苗（DTaP）：3月龄、4月龄、5月龄、18月龄各 1 剂；白破疫苗（DT）：6周岁 1 剂", "indication": "儿童", "evidence_level": "national_guideline"},
            {"method": "麻腮风疫苗（MMR）：8月龄、18月龄各 1 剂", "indication": "儿童", "evidence_level": "national_guideline"},
            {"method": "乙脑减毒活疫苗（JE-L）：8月龄、2周岁各 1 剂（或乙脑灭活疫苗 JE-I 四剂次）", "indication": "儿童", "evidence_level": "national_guideline"},
            {"method": "A群流脑多糖疫苗（MPSV-A）：6月龄、9月龄各 1 剂；A群C群流脑多糖疫苗（MPSV-AC）：3周岁、6周岁各 1 剂", "indication": "儿童", "evidence_level": "national_guideline"},
            {"method": "甲肝减毒活疫苗（HepA-L）：18月龄 1 剂（或甲肝灭活疫苗 HepA-I：18、24月龄各 1 剂）", "indication": "儿童", "evidence_level": "national_guideline"},
        ],
        "differential_diagnosis": [],
        "risk_factors": ["未按时接种的儿童", "HBsAg阳性母亲所生新生儿", "早产儿与低出生体重儿", "HIV感染母亲所生儿童"],
        "complications": [],
        "prevention": [
            "程序表所列接种时间是可接种该剂次的最小年龄；达到接种年龄应尽早接种",
            "推荐完成时间：乙肝第1剂出生后24小时内；卡介苗小于3月龄；乙肝第3剂/脊灰第3剂/百白破第3剂/麻腮风第1剂/乙脑第1剂小于12月龄；A群流脑第2剂小于18月龄；麻腮风第2剂/甲肝第1剂/百白破第4剂小于24月龄；白破/流脑AC第2剂小于7周岁",
            "两种及以上注射类疫苗应在不同部位接种，严禁混合吸入同一支注射器；现阶段国家免疫规划疫苗均可按程序或补种原则同时接种",
            "两种及以上注射类减毒活疫苗若未同时接种，应间隔不小于28天；灭活疫苗与口服减毒活疫苗对间隔不做限制",
            "流行季节可全年常规接种，也可按需开展补充免疫和应急接种",
        ],
        "when_to_seek_care": ["错过推荐接种年龄的儿童应尽早补种，只需补种未完成剂次，无需重新开始全程接种"],
        "clinical_examples": [],
        "citations": CITE,
        "keywords": ["预防针", "疫苗", "接种", "免疫程序", "儿童疫苗", "打疫苗", "疫苗时间表", "什么时候打疫苗", "免疫规划", "国家免疫规划"],
    },
    {
        "id": "cn-vaccine-hepb",
        "condition_zh": "乙肝疫苗",
        "condition_en": "Hepatitis B vaccine (HepB)",
        "category": "vaccine_cn",
        "regions": ["全国"],
        "diagnosis": {"lab_tests": [], "clinical_features": ["新生儿、婴幼儿"], "gold_standard": ""},
        "treatment": [
            {"method": "按“0-1-6个月”程序接种3剂：第1剂出生后24小时内（医院分娩由出生医院接种），第2剂1月龄，第3剂6月龄；肌内注射", "indication": "新生儿及儿童", "evidence_level": "national_guideline"},
            {"method": "重组（酵母）HepB 每剂 10μg；重组（CHO细胞）HepB：HBsAg阴性产妇所生新生儿 10μg，HBsAg阳性产妇所生新生儿 20μg", "indication": "按疫苗类型", "evidence_level": "national_guideline"},
            {"method": "HBsAg阳性产妇所生新生儿可按医嘱肌内注射 100 国际单位乙肝免疫球蛋白（HBIG），同时在不同部位接种第1剂 HepB；HepB、HBIG 和卡介苗可在不同部位同时接种", "indication": "HBsAg阳性产妇所生新生儿", "evidence_level": "national_guideline"},
            {"method": "HBsAg阳性或不详产妇所生新生儿出生后12小时内尽早接种第1剂；体重小于2000g者出生后尽早接种第1剂，并在满1月龄、2月龄、7月龄时再完成3剂次", "indication": "HBsAg阳性或不详产妇所生低体重儿", "evidence_level": "national_guideline"},
        ],
        "differential_diagnosis": [],
        "risk_factors": ["HBsAg阳性母亲所生新生儿", "出生体重小于2000g的新生儿", "危重症新生儿（极低出生体重/严重出生缺陷/重度窒息/呼吸窘迫综合征）——应在生命体征平稳后尽早接种"],
        "complications": [],
        "prevention": [
            "母亲为HBsAg阳性的儿童接种最后一剂 HepB 后 1-2 个月进行 HBsAg 和抗-HBs 检测；若 HBsAg 阴性、抗-HBs 阴性或小于 10mIU/ml，可再按程序免费接种 3 剂次",
            "未在医院分娩的新生儿由辖区接种单位全程接种",
        ],
        "when_to_seek_care": ["出生24小时内未及时接种第1剂者应尽早补种；未完成全程免疫程序者需尽早补齐未接种剂次"],
        "clinical_examples": [],
        "citations": CITE,
        "keywords": ["乙肝疫苗", "乙肝预防针", "乙肝", "HepB", "乙型肝炎疫苗", "新生儿乙肝", "母婴阻断", "HBsAg"],
    },
    {
        "id": "cn-vaccine-bcg",
        "condition_zh": "卡介苗",
        "condition_en": "BCG vaccine",
        "category": "vaccine_cn",
        "regions": ["全国"],
        "diagnosis": {"lab_tests": [], "clinical_features": ["新生儿"], "gold_standard": ""},
        "treatment": [
            {"method": "出生时接种 1 剂，皮内注射 0.1ml；严禁皮下或肌内注射", "indication": "新生儿", "evidence_level": "national_guideline"},
            {"method": "早产儿胎龄大于31孕周且医学评估稳定后可以接种；胎龄小于或等于31孕周的早产儿，医学评估稳定后可在出院前接种", "indication": "早产儿", "evidence_level": "national_guideline"},
        ],
        "differential_diagnosis": [],
        "risk_factors": ["早产儿", "未接种卡介苗的儿童"],
        "complications": [],
        "prevention": [
            "卡介苗主要预防结核性脑膜炎、粟粒性肺结核等严重结核病",
            "与免疫球蛋白接种间隔不做特别限制",
        ],
        "when_to_seek_care": [
            "未接种的小于3月龄儿童可直接补种",
            "3月龄-3岁儿童对结核菌素纯蛋白衍生物（TB-PPD）或卡介菌蛋白衍生物（BCG-PPD）试验阴性者应予补种",
            "大于或等于4岁儿童不予补种",
            "已接种的儿童即使卡痕未形成也不再补种",
        ],
        "clinical_examples": [],
        "citations": CITE,
        "keywords": ["卡介苗", "BCG", "预防结核", "卡痕", "出生疫苗", "第一针"],
    },
    {
        "id": "cn-vaccine-polio",
        "condition_zh": "脊髓灰质炎疫苗（脊灰疫苗）",
        "condition_en": "Polio vaccine (IPV/bOPV)",
        "category": "vaccine_cn",
        "regions": ["全国"],
        "diagnosis": {"lab_tests": [], "clinical_features": ["儿童"], "gold_standard": ""},
        "treatment": [
            {"method": "共接种4剂：2月龄、3月龄各1剂脊灰灭活疫苗（IPV，肌内注射0.5ml）；4月龄、4周岁各1剂二价脊灰减毒活疫苗（bOPV，口服，糖丸1粒或液体2滴约0.1ml）", "indication": "儿童", "evidence_level": "national_guideline"},
            {"method": "原发性免疫缺陷、胸腺疾病、HIV感染、正在接受化疗的恶性肿瘤、近期接受造血干细胞移植、正在使用免疫抑制或免疫调节药物、目前或近期接受免疫细胞靶向放疗者，建议按说明书全程使用 IPV", "indication": "免疫缺陷/免疫抑制人群", "evidence_level": "national_guideline"},
        ],
        "differential_diagnosis": [],
        "risk_factors": ["原发性免疫缺陷", "胸腺疾病", "HIV感染", "正在化疗的恶性肿瘤", "造血干细胞移植后", "使用免疫抑制/免疫调节药物者"],
        "complications": [],
        "prevention": [
            "已按说明书接种过 IPV 或含 IPV 成分联合疫苗者，可视为完成相应剂次；如已按免疫程序完成4剂次含IPV成分疫苗接种，4岁无需再接种 bOPV",
        ],
        "when_to_seek_care": [
            "小于4岁儿童未达到3剂（含补充免疫等）应补种完成3剂；大于或等于4岁儿童未达到4剂应补种完成4剂",
            "补种遵循先 IPV 后 bOPV 原则，两剂次间隔不小于28天",
        ],
        "clinical_examples": [],
        "citations": CITE,
        "keywords": ["脊灰疫苗", "脊髓灰质炎", "小儿麻痹", "IPV", "bOPV", "糖丸", "脊灰"],
    },
    {
        "id": "cn-vaccine-dtap",
        "condition_zh": "百白破疫苗/白破疫苗",
        "condition_en": "DTaP / DT vaccine",
        "category": "vaccine_cn",
        "regions": ["全国"],
        "diagnosis": {"lab_tests": [], "clinical_features": ["儿童"], "gold_standard": ""},
        "treatment": [
            {"method": "百白破疫苗（DTaP）：3月龄、4月龄、5月龄、18月龄各接种1剂，肌内注射0.5ml；白破疫苗（DT）：6周岁接种1剂，肌内注射0.5ml", "indication": "儿童", "evidence_level": "national_guideline"},
            {"method": "第1剂与第2剂、第2剂与第3剂间隔不小于28天；第3剂与第4剂间隔不小于6个月", "indication": "常规程序", "evidence_level": "national_guideline"},
        ],
        "differential_diagnosis": [],
        "risk_factors": ["未按程序完成百白破/白破接种的儿童"],
        "complications": [],
        "prevention": ["可预防百日咳、白喉、破伤风"],
        "when_to_seek_care": [
            "小于3月龄起始接种时第1剂为DTaP；若第3剂与第4剂之间间隔不足6个月时补种第4剂",
            "DTaP和DT累计大于或等于3剂的，已接种至少1剂DT则无需补种；仅3剂DTaP者接种1剂DT，与第3剂间隔不小于6个月；接种4剂DTaP但满7周岁未接种DT者补种1剂DT，与第4剂间隔不小于12个月",
        ],
        "clinical_examples": [],
        "citations": CITE,
        "keywords": ["百白破", "白破", "DTaP", "DT", "百日咳疫苗", "白喉", "破伤风疫苗", "百白破疫苗"],
    },
    {
        "id": "cn-vaccine-mmr",
        "condition_zh": "麻腮风疫苗",
        "condition_en": "MMR vaccine",
        "category": "vaccine_cn",
        "regions": ["全国"],
        "diagnosis": {"lab_tests": [], "clinical_features": ["儿童"], "gold_standard": ""},
        "treatment": [
            {"method": "共接种2剂次：8月龄、18月龄各接种1剂，皮下注射0.5ml", "indication": "儿童", "evidence_level": "national_guideline"},
            {"method": "注射免疫球蛋白者应间隔不小于3个月接种 MMR；接种 MMR 后2周内避免使用免疫球蛋白", "indication": "近期使用免疫球蛋白者", "evidence_level": "national_guideline"},
        ],
        "differential_diagnosis": [],
        "risk_factors": ["未完成2剂MMR接种的儿童", "近期使用过免疫球蛋白者"],
        "complications": [],
        "prevention": [
            "可预防麻疹、风疹、流行性腮腺炎",
            "如需接种包括MMR在内多种疫苗但无法同时完成时，应优先接种 MMR",
            "麻疹疫情应急接种时可根据流行病学特征对6-7月龄儿童接种1剂含麻疹成分疫苗（不计入常规免疫剂次）",
        ],
        "when_to_seek_care": [
            "2019年10月1日及以后出生儿童未按程序完成2剂MMR接种的，使用MMR补齐",
            "2007年扩免后至2019年9月30日出生的儿童，应至少接种2剂含麻疹成分、1剂含风疹成分和1剂含腮腺炎成分疫苗，不足者用MMR补齐",
            "2007年扩免前出生的小于18周岁人群，如未完成2剂含麻疹成分疫苗接种，使用MMR补齐",
            "需补种两剂MMR时，接种间隔应不小于28天",
        ],
        "clinical_examples": [],
        "citations": CITE,
        "keywords": ["麻腮风", "MMR", "麻疹疫苗", "风疹疫苗", "腮腺炎疫苗", "麻风腮", "麻腮风疫苗"],
    },
    {
        "id": "cn-vaccine-je",
        "condition_zh": "乙脑疫苗",
        "condition_en": "Japanese encephalitis vaccine (JE-L/JE-I)",
        "category": "vaccine_cn",
        "regions": ["全国"],
        "diagnosis": {"lab_tests": [], "clinical_features": ["儿童"], "gold_standard": ""},
        "treatment": [
            {"method": "乙脑减毒活疫苗（JE-L）：共2剂次，8月龄、2周岁各接种1剂，皮下注射0.5ml", "indication": "儿童（默认选择）", "evidence_level": "national_guideline"},
            {"method": "乙脑灭活疫苗（JE-I）：共4剂次，8月龄接种2剂（间隔7-10天），2周岁和6周岁各接种1剂，肌内注射0.5ml", "indication": "儿童（选择灭活疫苗时）", "evidence_level": "national_guideline"},
        ],
        "differential_diagnosis": [],
        "risk_factors": ["青海、新疆和西藏地区无乙脑疫苗免疫史的居民迁居其他省份或乙脑流行季节前往其他省份旅行者——建议接种1剂JE-L"],
        "complications": [],
        "prevention": ["注射免疫球蛋白者应间隔不小于3个月接种JE-L；JE-I为不小于1个月"],
        "when_to_seek_care": [
            "乙脑疫苗纳入免疫规划后出生且未接种的适龄儿童：用JE-L补种应补齐2剂，间隔不小于12个月；用JE-I补种应补齐4剂，第1、2剂间隔7-10天，第2、3剂间隔1-12个月，第3、4剂间隔不小于3年",
        ],
        "clinical_examples": [],
        "citations": CITE,
        "keywords": ["乙脑疫苗", "流行性乙型脑炎", "JE-L", "JE-I", "乙脑", "日本脑炎"],
    },
    {
        "id": "cn-vaccine-meningococcal",
        "condition_zh": "流脑疫苗（脑膜炎球菌多糖疫苗）",
        "condition_en": "Meningococcal vaccine (MPSV-A / MPSV-AC)",
        "category": "vaccine_cn",
        "regions": ["全国"],
        "diagnosis": {"lab_tests": [], "clinical_features": ["儿童"], "gold_standard": ""},
        "treatment": [
            {"method": "A群流脑多糖疫苗（MPSV-A）：2剂次，6月龄、9月龄各1剂，皮下注射0.5ml，两剂间隔不小于3个月", "indication": "儿童", "evidence_level": "national_guideline"},
            {"method": "A群C群流脑多糖疫苗（MPSV-AC）：2剂次，3周岁、6周岁各1剂，皮下注射0.5ml，两剂间隔不小于3年（3年内避免重复接种）", "indication": "儿童", "evidence_level": "national_guideline"},
        ],
        "differential_diagnosis": [],
        "risk_factors": ["未接种或未完成规定剂次的儿童", "流脑疫情波及地区儿童"],
        "complications": [],
        "prevention": [
            "第1剂MPSV-AC与第2剂MPSV-A间隔不小于12个月",
            "小于24月龄儿童如已按流脑结合疫苗说明书接种规定剂次，可视为完成MPSV-A接种；3周岁和6周岁已接种含A群和C群流脑成分疫苗者可视为完成相应MPSV-AC剂次",
            "流脑疫情应急接种时根据引起疫情的菌群选择相应种类流脑疫苗",
        ],
        "when_to_seek_care": [
            "小于24月龄儿童补齐MPSV-A剂次；大于或等于24月龄不再补种或接种MPSV-A，仍需完成两剂次MPSV-AC",
            "大于或等于24月龄未接种过MPSV-A者可在3周岁前尽早接种MPSV-AC；已接种过1剂MPSV-A者间隔不小于3个月尽早接种MPSV-AC",
        ],
        "clinical_examples": [],
        "citations": CITE,
        "keywords": ["流脑疫苗", "脑膜炎球菌", "MPSV-A", "MPSV-AC", "流行性脑脊髓膜炎", "流脑", "A群流脑", "A群C群流脑"],
    },
    {
        "id": "cn-vaccine-hepa",
        "condition_zh": "甲肝疫苗",
        "condition_en": "Hepatitis A vaccine (HepA-L / HepA-I)",
        "category": "vaccine_cn",
        "regions": ["全国"],
        "diagnosis": {"lab_tests": [], "clinical_features": ["儿童"], "gold_standard": ""},
        "treatment": [
            {"method": "甲肝减毒活疫苗（HepA-L）：18月龄接种1剂，皮下注射0.5ml或1.0ml（按说明书）", "indication": "儿童（默认选择）", "evidence_level": "national_guideline"},
            {"method": "甲肝灭活疫苗（HepA-I）：共2剂次，18月龄和24月龄各接种1剂，肌内注射0.5ml", "indication": "儿童（选择灭活疫苗时）", "evidence_level": "national_guideline"},
        ],
        "differential_diagnosis": [],
        "risk_factors": ["未接种甲肝疫苗的适龄儿童"],
        "complications": [],
        "prevention": [
            "接种2剂次及以上含甲肝灭活成分疫苗可视为完成甲肝疫苗免疫程序",
            "注射免疫球蛋白后应间隔不小于3个月接种 HepA-L",
        ],
        "when_to_seek_care": [
            "使用HepA-L补种1剂；使用HepA-I补种应补齐2剂，间隔不小于6个月",
            "已接种1剂HepA-I但无条件接种第2剂时，可接种1剂HepA-L完成补种，间隔不小于6个月",
        ],
        "clinical_examples": [],
        "citations": CITE,
        "keywords": ["甲肝疫苗", "甲型肝炎", "HepA-L", "HepA-I", "甲肝", "甲型肝炎疫苗"],
    },
    {
        "id": "cn-vaccine-special-health",
        "condition_zh": "特殊健康状态儿童接种建议",
        "condition_en": "Vaccination for children with special health conditions",
        "category": "vaccine_cn",
        "regions": ["全国"],
        "diagnosis": {"lab_tests": [], "clinical_features": ["早产儿/低出生体重儿、过敏体质、HIV暴露、免疫缺陷等"], "gold_standard": ""},
        "treatment": [
            {"method": "早产儿（胎龄小于37周）和/或低出生体重儿（出生体重小于2500g）如医学评估稳定且处于持续恢复状态，按出生后实际月龄接种疫苗（卡介苗另有规定）", "indication": "早产儿与低出生体重儿", "evidence_level": "national_guideline"},
            {"method": "HIV感染母亲所生儿童：确认HIV感染且有艾滋病相关症状或免疫抑制者不予接种卡介苗、含麻疹成分疫苗；无相关症状者可接种含麻疹成分疫苗；除非已明确未感染HIV，否则不予接种乙脑减毒活疫苗、甲肝减毒活疫苗、脊灰减毒活疫苗，可接种相应灭活疫苗", "indication": "HIV感染母亲所生儿童", "evidence_level": "national_guideline"},
            {"method": "除HIV感染者外的免疫缺陷或正在接受全身免疫抑制治疗者，可以接种灭活疫苗，原则上不予接种减毒活疫苗（补体缺陷患者除外）", "indication": "免疫功能异常儿童", "evidence_level": "national_guideline"},
        ],
        "differential_diagnosis": [],
        "risk_factors": ["早产儿", "低出生体重儿", "HIV感染母亲所生儿童", "免疫缺陷或免疫抑制治疗者", "对已知疫苗成分严重过敏或既往接种发生严重过敏反应者"],
        "complications": [],
        "prevention": [
            "所谓“过敏性体质”不是疫苗接种的禁忌证",
            "下述常见疾病不作为疫苗接种禁忌：生理性和母乳性黄疸，单纯性热性惊厥史，癫痫控制处于稳定期，病情稳定的脑疾病、肝脏疾病，常见先天性疾病（先天性甲状腺功能减退、苯丙酮尿症、唐氏综合征、先天性心脏病）和先天性感染（梅毒、巨细胞病毒和风疹病毒）",
            "对已知疫苗成分严重过敏或既往因接种发生喉头水肿、过敏性休克及其他全身性严重过敏反应的，禁忌继续接种同种疫苗",
            "其他特殊健康状况儿童如无明确证据表明接种疫苗存在安全风险，原则上可按免疫程序接种",
        ],
        "when_to_seek_care": ["HIV感染母亲所生儿童由医疗机构出具是否为HIV感染、是否出现症状、是否有免疫抑制的诊断，据此决定接种方案；小于18月龄婴儿接种前不必进行HIV抗体筛查"],
        "clinical_examples": [],
        "citations": CITE,
        "keywords": ["特殊健康状态", "早产儿接种", "低出生体重儿", "过敏体质能打疫苗吗", "黄疸能打疫苗吗", "热性惊厥接种", "先天性心脏病疫苗", "HIV暴露儿童", "免疫缺陷疫苗"],
    },
]


def main() -> int:
    ids = [e["id"] for e in ENTRIES]
    assert len(ids) == len(set(ids)), "duplicate ids"
    for e in ENTRIES:
        assert e["condition_zh"] and e["keywords"], e["id"]
        assert e["treatment"], e["id"]
    OUT.write_text(json.dumps(ENTRIES, ensure_ascii=False, indent=1), encoding="utf-8")
    print(f"wrote {OUT} ({len(ENTRIES)} entries)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
