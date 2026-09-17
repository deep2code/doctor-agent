#!/usr/bin/env python3
"""Convert the 中国婴幼儿喂养指南(2022) 三册 + 国家卫健委《婴幼儿喂养健康教育
核心信息》 into internal/knowledge/data/feeding_guidelines.json —
KnowledgeEntry entries (category="feeding_guideline").

Authoritative text sources (all public, cross-verified):
- 中国营养学会膳食指南官网《中国婴幼儿喂养指南(2022)》核心信息:
  http://dg.cnsoc.org/article/04/gc5cUak3RhSGheqSaRljnA.html
- 东莞市疾病预防控制中心《7—24月龄婴幼儿喂养和营养指南》(依据《中国居民膳食
  指南(2022)》, 六条准则全文):
  http://dghb.dg.gov.cn/zsjg/dzsjbyfkzzx/ztzl/yyeyyzl/yyeyyzl/post_3962203.html
- 国家卫健委《婴幼儿喂养健康教育核心信息》(首都儿科研究所转载, 2026-08):
  https://news.qq.com/rain/a/20260805A08UYH00
No inference beyond the source texts.
"""
import json
import sys
from pathlib import Path

ROOT = Path(__file__).parent.parent
OUT = ROOT / "internal" / "knowledge" / "data" / "feeding_guidelines.json"

CNS = [{
    "type": "national_guideline",
    "title": "中国婴幼儿喂养指南（2022）核心信息（中国营养学会）",
    "journal": "",
    "year": 2022,
    "doi": "",
    "pmid": "",
    "level": "national_guideline",
    "url": "http://dg.cnsoc.org/article/04/gc5cUak3RhSGheqSaRljnA.html",
}]
NHC_CORE = [{
    "type": "national_guideline",
    "title": "婴幼儿喂养健康教育核心信息（国家卫健委，2026 母乳喂养周）",
    "journal": "",
    "year": 2026,
    "doi": "",
    "pmid": "",
    "level": "national_guideline",
    "url": "https://news.qq.com/rain/a/20260805A08UYH00",
}]
DG = [{
    "type": "national_guideline",
    "title": "7—24月龄婴幼儿喂养和营养指南（依据《中国居民膳食指南(2022)》）",
    "journal": "",
    "year": 2022,
    "doi": "",
    "pmid": "",
    "level": "national_guideline",
    "url": "http://dghb.dg.gov.cn/zsjg/dzsjbyfkzzx/ztzl/yyeyyzl/yyeyyzl/post_3962203.html",
}]

ENTRIES = [
    {
        "id": "cn-feeding-0to6",
        "condition_zh": "0-6月龄婴儿母乳喂养指南",
        "condition_en": "Breastfeeding guideline, infants 0-6 months (2022)",
        "category": "feeding_guideline",
        "regions": ["全国"],
        "diagnosis": {"lab_tests": [], "clinical_features": ["0-6月龄婴儿"], "gold_standard": ""},
        "treatment": [
            {"method": "准则1：母乳是婴儿最理想的食物，坚持6月龄内纯母乳喂养（不需添加水和其他食物）", "indication": "0-6月龄婴儿", "evidence_level": "national_guideline"},
            {"method": "准则2：生后1小时内开奶，重视尽早吸吮", "indication": "新生儿", "evidence_level": "national_guideline"},
            {"method": "准则3：回应式喂养，建立良好的生活规律（按需哺乳，每日8-10次以上，识别咂嘴/吐舌/寻觅等进食信号，不应等到哭闹再哺喂）", "indication": "0-6月龄婴儿", "evidence_level": "national_guideline"},
            {"method": "准则4：适当补充维生素D（医生指导下每日400-800国际单位），母乳喂养无需补钙", "indication": "0-6月龄婴儿", "evidence_level": "national_guideline"},
            {"method": "准则5：婴儿配方奶是不能纯母乳喂养时的无奈选择", "indication": "无法纯母乳喂养者", "evidence_level": "national_guideline"},
            {"method": "准则6：监测体格指标，保持健康生长", "indication": "0-6月龄婴儿", "evidence_level": "national_guideline"},
        ],
        "differential_diagnosis": [],
        "risk_factors": ["早产儿与低出生体重儿（更加提倡母乳喂养）", "患病母亲（应咨询医务人员，一般感冒、腹泻可坚持哺乳）"],
        "complications": [],
        "prevention": [
            "母乳含丰富营养素、免疫活性物质和水分，可满足0-6个月婴儿全部营养需求，任何配方奶、牛羊奶无法替代",
            "母乳喂养可降低婴儿感冒、腹泻、肺炎风险，减少成年后肥胖、糖尿病和心脑血管疾病，促进大脑发育，增进亲子关系；还可减少母亲产后出血、乳腺癌、卵巢癌风险",
            "正常足月婴儿出生后6个月内一般不用补充钙剂",
            "母乳挤出后室温可保存4-8小时，冰箱冷藏1-2天，冷冻3-6个月",
        ],
        "when_to_seek_care": ["婴儿发生腹泻不需禁食，可继续母乳喂养，在医生指导下及时补充体液避免脱水", "早产儿、低出生体重儿和患病婴儿应听从医务人员指导科学喂养"],
        "clinical_examples": [],
        "citations": CNS,
        "keywords": ["母乳喂养", "纯母乳", "喂奶", "母乳", "开奶", "按需哺乳", "维生素D", "要不要补钙", "配方奶", "奶粉", "0-6个月喂养", "婴儿喂养"],
    },
    {
        "id": "cn-feeding-7to24",
        "condition_zh": "7-24月龄婴幼儿喂养指南",
        "condition_en": "Complementary feeding guideline, infants 7-24 months (2022)",
        "category": "feeding_guideline",
        "regions": ["全国"],
        "diagnosis": {"lab_tests": [], "clinical_features": ["满6月龄至不满2周岁的婴幼儿"], "gold_standard": ""},
        "treatment": [
            {"method": "准则1：继续母乳喂养，满6月龄起必须添加辅食，从富含铁的泥糊状食物开始（肉泥、肝泥、强化铁的婴儿谷粉）", "indication": "满6月龄起", "evidence_level": "national_guideline"},
            {"method": "准则2：及时引入多样化食物，重视动物性食物的添加（每次只引入一种新食物，适应2-3天观察反应；不盲目回避易过敏食物，1岁内适时引入各种食物）", "indication": "7-24月龄", "evidence_level": "national_guideline"},
            {"method": "准则3：尽量少加糖盐，油脂适当，保持食物原味（辅食单独制作；1岁以内不加盐、糖和调味品；1岁以后少盐少糖，尝试淡口味家庭膳食）", "indication": "7-24月龄", "evidence_level": "national_guideline"},
            {"method": "准则4：提倡回应式喂养，鼓励但不强迫进食（识别饥饱信号；进餐时不看电视、不玩玩具，每次不超过20分钟；不以食物作为奖励或惩罚）", "indication": "7-24月龄", "evidence_level": "national_guideline"},
            {"method": "准则5：注重饮食卫生和进食安全（选择安全优质新鲜食材、生熟分开、饭前洗手、成人看护、不吃剩饭；避免整粒花生、坚果、果冻等防窒息）", "indication": "7-24月龄", "evidence_level": "national_guideline"},
            {"method": "准则6：定期监测体格指标，追求健康生长（每3个月测量一次身长、体重、头围；平稳生长是最佳生长模式）", "indication": "7-24月龄", "evidence_level": "national_guideline"},
        ],
        "differential_diagnosis": [],
        "risk_factors": ["辅食添加过早（满4月龄前）可增加超重肥胖及代谢性疾病风险", "辅食添加过晚（满6月龄后）增加贫血、铁和维生素A缺乏风险"],
        "complications": [],
        "prevention": [
            "满6月龄后继续母乳喂养到2岁或以上；6个月后单一母乳已不能完全满足生长发育需求",
            "辅食添加原则：由少到多、由稀到稠、由细到粗、由一种到多种，循序渐进；从泥糊状（6月龄）→带小颗粒稠粥烂面（9月龄）→块状（10-12月龄）→软烂饭（1岁）→接近家庭饮食（2岁）",
            "6-9月龄每日添加辅食1-2次（哺乳4-5次）；9-12月龄每日2-3次（哺乳2-3次）；1-2岁每日3餐+2次加餐，继续母乳喂养",
            "每日辅食种类不少于4种，至少包括一种动物性食物、一种蔬菜和一种谷薯类",
            "引入新食物1-2日内出现皮疹、腹泻、呕吐等轻微不适，暂停添加，好转后小量再试；症状严重及时就医",
            "辅食应含适量油脂（推荐亚麻籽油、核桃油、大豆油、菜籽油等富含必需脂肪酸的油）",
        ],
        "when_to_seek_care": ["添加辅食后反复出现皮疹、腹泻、呕吐等过敏/不耐受表现，或症状严重时及时就医", "贫困/食物供应不足地区婴幼儿可在医生指导下给予辅食营养补充剂（营养包）"],
        "clinical_examples": [],
        "citations": DG + CNS,
        "keywords": ["辅食", "添加辅食", "辅食添加", "宝宝辅食", "米粉", "什么时候加辅食", "辅食怎么加", "回应式喂养", "顺应喂养", "7-24月龄", "辅食油", "泥糊状食物"],
    },
    {
        "id": "cn-feeding-preschool",
        "condition_zh": "学龄前儿童膳食指南",
        "condition_en": "Dietary guideline for preschool children (2022)",
        "category": "feeding_guideline",
        "regions": ["全国"],
        "diagnosis": {"lab_tests": [], "clinical_features": ["2-6岁学龄前儿童"], "gold_standard": ""},
        "treatment": [
            {"method": "准则1：食物多样，规律就餐，自主进食，培养健康饮食行为（每天早中晚三次正餐+两次加餐，正餐间隔4-5小时；每天食物种类12种以上、每周25种以上）", "indication": "学龄前儿童", "evidence_level": "national_guideline"},
            {"method": "准则2：每天饮奶，足量饮水，正确选择零食（每日饮奶300-500ml或相当量奶制品；首选白开水；零食优选奶制品、水果、蔬菜和坚果，少吃高盐高糖高脂食品）", "indication": "学龄前儿童", "evidence_level": "national_guideline"},
            {"method": "准则3：合理烹调，少调料少油炸（蒸、煮、炖、煨为主；2-3岁每日食盐<2克，4-5岁<3克）", "indication": "学龄前儿童", "evidence_level": "national_guideline"},
            {"method": "准则4：参与食物选择与制作，增进对食物的认知和喜爱", "indication": "学龄前儿童", "evidence_level": "national_guideline"},
            {"method": "准则5：经常户外活动，定期体格测量，保障健康成长（每天身体活动总时间180分钟，其中户外活动至少120分钟）", "indication": "学龄前儿童", "evidence_level": "national_guideline"},
        ],
        "differential_diagnosis": [],
        "risk_factors": ["挑食偏食", "过量进食", "饮用含糖饮料", "进食时看电视/使用电子产品"],
        "complications": [],
        "prevention": [
            "进餐定时定位、细嚼慢咽但不拖延，30分钟内完成；让儿童自己使用筷子和勺进食",
            "避免整粒豆类、坚果以防呛入气管（建议磨成粉或打成糊）",
            "控制含糖饮料，预防龋齿和超重肥胖",
        ],
        "when_to_seek_care": ["身高体重明显偏离生长曲线时及时咨询儿童保健医生"],
        "clinical_examples": [],
        "citations": CNS,
        "keywords": ["学龄前儿童", "儿童膳食", "幼儿园饮食", "孩子吃饭", "挑食", "偏食", "每天喝奶", "儿童零食", "几岁吃盐", "儿童户外活动", "3岁孩子吃什么"],
    },
    {
        "id": "cn-feeding-core-info",
        "condition_zh": "婴幼儿喂养健康教育核心信息（国家卫健委）",
        "condition_en": "NHC core messages on infant & young child feeding",
        "category": "feeding_guideline",
        "regions": ["全国"],
        "diagnosis": {"lab_tests": [], "clinical_features": ["0-3岁婴幼儿"], "gold_standard": ""},
        "treatment": [
            {"method": "母乳是婴儿最理想的天然食物，0-6个月提倡纯母乳喂养（不需添加水和任何其他食物）", "indication": "0-6月龄", "evidence_level": "national_guideline"},
            {"method": "母亲按需哺乳每日8-10次以上；识别咂嘴、吐舌、寻觅等进食信号及时哺喂", "indication": "0-6月龄", "evidence_level": "national_guideline"},
            {"method": "出生开始在医生指导下每天补充维生素D 400-800国际单位；正常足月婴儿6个月内一般不用补钙", "indication": "0-6月龄", "evidence_level": "national_guideline"},
            {"method": "婴儿6个月起添加辅食，在添加辅食基础上可继续母乳喂养至2岁及以上", "indication": "满6月龄", "evidence_level": "national_guideline"},
            {"method": "辅食从单一食物开始、每次只添加一种新食物；先选含铁丰富的泥糊状食物，每次1小勺逐渐加量，2-3日适应后再加新食物", "indication": "6月龄起", "evidence_level": "national_guideline"},
            {"method": "6-9月龄每日辅食1-2次（哺乳4-5次）；9-12月龄每日2-3次（哺乳2-3次）；1-2岁每日3餐+2次加餐并继续母乳", "indication": "6月龄-2岁", "evidence_level": "national_guideline"},
            {"method": "辅食质地从泥糊状（6月龄）→小颗粒稠粥烂面（9月龄）→块状（10-12月龄）→软烂饭（1岁）→接近家庭饮食（2岁）", "indication": "6月龄-2岁", "evidence_level": "national_guideline"},
            {"method": "1岁以内辅食保持原味，不加盐、糖和调味品；1岁以后少盐少糖；2岁后仍少盐少糖，避免腌制品、熏肉、含糖饮料", "indication": "6月龄-3岁", "evidence_level": "national_guideline"},
            {"method": "营造快乐轻松的进食环境，鼓励但不强迫；进餐时不看电视电脑手机，每次进餐20分钟左右、最长不超过30分钟", "indication": "6月龄-3岁", "evidence_level": "national_guideline"},
            {"method": "1岁内婴儿在3、6、8、12个月，1-3岁幼儿在18、24、30、36个月到乡镇卫生院/社区卫生服务中心或妇幼保健院接受儿童健康检查，评价生长发育和营养状况", "indication": "0-3岁", "evidence_level": "national_guideline"},
        ],
        "differential_diagnosis": [],
        "risk_factors": ["辅食添加过晚（满6月龄后）", "辅食种类频次不足（可致贫血、低体重、生长迟缓、智力发育落后）", "高盐高糖高脂饮食", "整粒花生、坚果、果冻（窒息风险）"],
        "complications": [],
        "prevention": [
            "混合喂养及人工喂养的婴儿满6个月也要及时添加辅食",
            "辅食食物包括谷薯类、豆类和坚果类、动物性食物（鱼禽肉及内脏）、蛋、含维生素A丰富的蔬果、其他蔬果、奶类及奶制品7类；每日种类不少于4种，至少含一种动物性食物、一种蔬菜、一种谷薯类",
            "母亲患一般感冒、腹泻时可坚持母乳喂养（乳汁中特异抗体保护婴儿）；患病时咨询医务人员",
            "引入新食物1-2日内出现皮疹、腹泻、呕吐等轻微不适暂停添加，好转后小量再试",
        ],
        "when_to_seek_care": ["引入新食物后症状严重时及时就医", "婴儿腹泻时不需要禁食，可继续母乳喂养，在医生指导下补充体液避免脱水", "早产儿、低出生体重儿和患病婴儿听从医务人员指导"],
        "clinical_examples": [],
        "citations": NHC_CORE,
        "keywords": ["喂养", "婴幼儿喂养", "科学喂养", "喂养指南", "辅食添加时间", "辅食次数", "宝宝几个月加辅食", "辅食质地", "儿童健康检查", "营养评价", "母乳喂养周"],
    },
]


def main() -> int:
    ids = [e["id"] for e in ENTRIES]
    assert len(ids) == len(set(ids)), "duplicate ids"
    for e in ENTRIES:
        assert e["condition_zh"] and e["keywords"] and e["treatment"], e["id"]
        assert e["citations"] and e["citations"][0]["url"], e["id"]
    OUT.write_text(json.dumps(ENTRIES, ensure_ascii=False, indent=1), encoding="utf-8")
    print(f"wrote {OUT} ({len(ENTRIES)} entries)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
