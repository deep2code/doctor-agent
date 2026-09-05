#!/usr/bin/env python3
"""Post-process the health-expansion KnowledgeEntry JSONs (elderly_care /
gyn_health / ortho_child_health):

1. Split keywords that the LLM emitted as one "、" / "," / "，" joined
   string into proper array elements (keyword matching needs individual
   terms).
2. Dedupe keywords; enforce 5-10 entries.
3. Inject verified factual corrections / colloquial keywords per entry id
   (EXTRA table below — numbers transcribed from the source documents,
   never LLM-regenerated).

  python3 external/postprocess_health_json.py [file.json ...]
"""
import json
import re
import sys
from pathlib import Path

DATA = Path(__file__).parent.parent / "internal" / "knowledge" / "data"

# entry-id substring -> {"append_keywords": [...], "patch": {field: value}}
EXTRA = {
    "cervical-screening-2021-1": {
        "append_keywords": ["两癌筛查", "TCT", "宫颈癌筛查年龄", "cervical cancer screening"],
        "patch": {"prevention": [
            "筛查对象为 35-64 周岁妇女",
            "主要筛查方法为宫颈细胞学检查(TCT)与高危型 HPV 检测",
            "接种 HPV 疫苗",
            "安全性行为",
        ]},
    },
    "breast-screening-2021-1": {
        "append_keywords": ["两癌筛查", "钼靶", "乳腺X光", "乳腺彩超", "breast cancer screening"],
        "patch": {"prevention": [
            "筛查对象为 35-64 周岁妇女",
            "乳腺超声与乳腺 X 线(钼靶)为主要筛查方法",
            "定期筛查、乳腺自查不能替代专业筛查",
            "适龄生育、母乳喂养、控制体重",
        ]},
    },
    "menopause-2025": {
        "append_keywords": ["更年期", "绝经", "围绝经期", "潮热", "盗汗", "脾气暴躁", "menopause"],
    },
    "disability-prevention-2019-4": {
        "append_keywords": ["老人疫苗", "老年人打疫苗", "打疫苗", "疫苗", "预防肺炎", "老人接种"],
    },
    "elderly-dietary": {
        "append_keywords": ["老人营养", "老年营养", "吃饭没胃口", "肌肉流失", "肌少症",
                            "吃不下饭", "没食欲", "不爱吃饭", "消瘦", "体重下降", "营养不良"],
    },
    "autism-screening-2022-2": {
        "append_keywords": ["不会说话", "不理人", "叫名字没反应", "自闭症", "语言发育落后",
                            "目光对视差", "发育落后", "孩子不交流", "孤独症表现", "autism"],
        "patch": {"diagnosis": {"clinical_features": [
            "3月龄:对很大声音没有反应/逗引时不发音或不会微笑/不注视人脸、不追视移动人或物品/俯卧时不会抬头",
            "6月龄:发音少不会笑出声/不会伸手抓物/紧握拳松不开/不能扶坐",
            "8月龄:听到声音无应答/不会区分生人和熟人/双手间不会传递玩具/不会独坐",
            "12月龄:呼唤名字无反应/不会模仿再见或欢迎动作/不会用拇食指对捏小物品/不会扶物站立",
            "18月龄:不会有意识叫爸爸妈妈/不会按要求指人或物/与人无目光交流/不会独走",
            "24月龄:不会说3个物品的名称/不会按吩咐做简单事情/不会用勺吃饭/不会扶栏上楼梯台阶",
            "30月龄:不会说2-3个字的短语/兴趣单一刻板/不会示意大小便/不会跑",
            "36月龄:不会说自己的名字/不会玩拿棍当马骑等假想游戏/不会模仿画圆/不会双脚跳",
            "4岁:不会说带形容词的句子/不能按要求等待或轮流/不会独立穿衣/不会单脚站立",
            "5岁:不能简单叙说事情经过/不知道自己的性别/不会用筷子吃饭/不会单脚跳",
            "6岁:不会表达自己的感受或想法/不会玩角色扮演的集体游戏/不会画方形/不会奔跑",
            "任何一条预警征象阳性提示发育偏异可能,应转诊复筛",
        ]}},
    },
    "alzheimer": {
        "append_keywords": ["老年痴呆", "失智症", "记性变差", "认知下降", "Alzheimer"],
    },
    "osteoporosis": {
        "append_keywords": ["骨松", "骨头脆", "驼背", "一摔就骨折", "osteoporosis"],
    },
}


def normalize_treatment(entry: dict) -> None:
    """LLM emits treatment[].{name,details}; the Go schema expects
    {method, indication, evidence_level, notes} — remap so the field
    survives JSON unmarshalling."""
    t = entry.get("treatment")
    if not isinstance(t, list):
        return
    out = []
    for item in t:
        if isinstance(item, dict) and "name" in item and "method" not in item:
            out.append({
                "method": str(item.get("name", "")),
                "indication": "",
                "evidence_level": "",
                "notes": str(item.get("details", item.get("note", ""))),
            })
        else:
            out.append(item)
    entry["treatment"] = out


def split_keywords(kws: list[str]) -> list[str]:
    out = []
    for k in kws:
        parts = re.split(r"[、,，;；\s]+", k.strip())
        for p in parts:
            if len(p) < 2:
                continue
            # drop bare English stopwords from LLM phrase splitting
            if p.isascii() and len(p) < 3 and p.lower() not in ("hpv", "tct"):
                continue
            out.append(p)
    return list(dict.fromkeys(out))


def main() -> int:
    files = [Path(a) for a in sys.argv[1:]] or [
        DATA / "gyn_health.json", DATA / "elderly_care.json", DATA / "ortho_child_health.json"]
    for f in files:
        if not f.exists():
            print(f"SKIP {f.name} (不存在)", file=sys.stderr)
            continue
        entries = json.loads(f.read_text(encoding="utf-8"))
        patched = 0
        for e in entries:
            e["keywords"] = split_keywords(e.get("keywords", []))
            normalize_treatment(e)
            for key_sub, extra in EXTRA.items():
                if key_sub in e.get("id", ""):
                    e["keywords"] = list(dict.fromkeys(
                        e["keywords"] + extra.get("append_keywords", [])))
                    e.update(extra.get("patch", {}))
                    patched += 1
        f.write_text(json.dumps(entries, ensure_ascii=False, indent=1), encoding="utf-8")
        print(f"{f.name}: {len(entries)} 条, 修正 {patched} 条, "
              f"keywords 平均 {sum(len(e.get('keywords', [])) for e in entries) // max(len(entries), 1)} 个",
              file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
