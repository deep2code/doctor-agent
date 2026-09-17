#!/usr/bin/env python3
"""Convert the structured China CDC alert entries (external/cdc_structured/*.json
— produced by structurize_cdc.py) into KnowledgeEntry JSON
(internal/knowledge/data/cdc_entries.json) so they join the embedded store.
the medical-knowledge retrieval layer.

Mapping: cdc entry -> KnowledgeEntry
  season/condition -> id/citations title
  symptoms -> diagnosis.clinical_features
  when_to_seek_care -> when_to_seek_care
  citation: type=cdc_alert, journal empty (institutional URL, no DOI/PMID).
"""
import json
import re
from pathlib import Path

ROOT = Path(__file__).parent.parent
SRC_DIR = Path(__file__).parent / "cdc_structured"  # jkts_t20260428_1835392.json
OUT = ROOT / "internal" / "knowledge" / "data" / "cdc_entries.json"

CAT = {
    "infectious_disease": "infectious_disease",
    "chronic_disease": "chronic_disease",
    "environmental_health": "environmental_health",
    "injury_prevention": "injury_prevention",
    "nutrition": "nutrition",
    "other": "other",
}

# Generic symptom words that belong to the dedicated disease entries
# (dengue-001 etc.), not to the monthly CDC alert entries. Keeping them here
# would let monthly alerts crowd out specific disease entries in symptom
# queries (see TestRetrieverSymptomStyleChineseRecall).
GENERIC_SYMPTOMS = {
    "发热", "皮疹", "头痛", "关节痛", "呕吐", "腹泻", "恶心", "乏力",
    "肌肉酸痛", "出血", "白细胞减少", "咳嗽", "咽痛", "呼吸困难",
    "食欲", "腹痛", "头晕", "寒战", "淋巴结肿大",
}


def uniq(xs):
    return list(dict.fromkeys(xs))


def merge_alerts(groups):
    """Merge monthly alert entries of the same disease into one entry."""
    entries = []
    for key, alerts in groups.items():
        first = alerts[0]
        seasons = uniq(a.get("season", "") for a in alerts if a.get("season"))
        cites = []
        for a in alerts:
            c = a["_cite"]
            if c not in cites:
                cites.append(c)
        entry = {
            "id": first["_id"],
            "condition_zh": first["condition_zh"],
            "condition_en": first.get("condition_en", ""),
            "category": CAT.get(first.get("category", "other"), "other"),
            "regions": ["全国"],
            "season": "；".join(s for s in seasons if s),
            "diagnosis": {"clinical_features": uniq(
                s for a in alerts for s in a.get("symptoms", []))},
            "risk_factors": uniq(r for a in alerts for r in a.get("risk_factors", [])),
            "prevention": uniq(p for a in alerts for p in a.get("prevention", [])),
            "when_to_seek_care": uniq(
                w for a in alerts for w in a.get("when_to_seek_care", [])),
            "citations": cites,
            "keywords": uniq(k for a in alerts for k in a.get("keywords", [])
                             if k not in GENERIC_SYMPTOMS),
        }
        entries.append(entry)
    return entries


def main() -> int:
    raw = []  # list of (alert, cite)
    for f in sorted(SRC_DIR.glob("*.json")):
        alerts = json.loads(f.read_text(encoding="utf-8"))
        m = re.search(r"t(\d{8})_(\d+)", f.name)
        if m:
            yyyymm, dd, aid = m.group(1)[:6], m.group(1)[6:], m.group(2)
            url = f"https://www.chinacdc.cn/jkts/{yyyymm}/t{yyyymm}{dd}_{aid}.html"
        else:
            url = "https://www.chinacdc.cn/jkts/"
        for a in alerts:
            season = a.get("season", "")
            mm = re.search(r"(\d{4})年?(\d{1,2})月", season or "")
            year = int(mm.group(1)) if mm else 2026
            title = season or "中国疾控中心健康风险提示"
            cite = {
                "type": "cdc_alert",
                "title": title,
                "journal": "",
                "year": year,
                "doi": "",
                "pmid": "",
                "level": "official_guidance",
                "url": url,
            }
            a["_cite"] = cite
            a["_id"] = a["id"]
            raw.append(a)

    # merge by disease name (monthly alerts repeat the same diseases)
    groups = {}
    for a in raw:
        key = (a["condition_zh"], a.get("condition_en", ""))
        groups.setdefault(key, []).append(a)
    entries = merge_alerts(groups)

    OUT.write_text(json.dumps(entries, ensure_ascii=False, indent=1), encoding="utf-8")
    print(f"{len(entries)} 条 CDC 知识条目（合并自 {len(raw)} 条月度提示）-> {OUT}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
