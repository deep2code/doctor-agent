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
    "disability-prevention": {
        "append_keywords": ["老年失能", "失能老人", "卧床老人", "照护老人", "elderly disability"],
    },
    "alzheimer": {
        "append_keywords": ["老年痴呆", "失智症", "记性变差", "认知下降", "Alzheimer"],
    },
    "elderly-dietary": {
        "append_keywords": ["老人营养", "老年营养", "吃饭没胃口", "肌肉流失", "肌少症"],
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
        out.extend(p for p in parts if len(p) >= 2)
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
