#!/usr/bin/env python3
"""Merge external/medical_terminology_2024.json into the live knowledge layers.

The source file is an unregistered 2025-09 resource list (20 guidelines +
4 terminology standards + 8 flat synonym keyword lists). It is split:

  1. synonym lists  -> curated concept groups merged (union, never
     overwriting) into internal/knowledge/alias_map.json, which is
     //go:embed-ed and applied by ExpandQuery at query time — zero DB,
     zero reseed, effective immediately.
  2. guideline / standard metadata rows -> appended to
     internal/knowledge/data/public_resources.json, which already ships a
     keyword retrieval layer (RetrievePublicResources) and is exposed via
     knowledge_search dataset "public_resources". Deduped against the
     existing 90 rows by normalized name or URL.

After running: python3 external/make_gz.py && go run . seed-knowledge
(only public_resources needs the reseed; alias_map is embedded).

Idempotent: reruns skip already-present rows/aliases.
"""

import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SOURCE = ROOT / "external" / "medical_terminology_2024.json"
ALIAS_MAP = ROOT / "internal" / "knowledge" / "alias_map.json"
PUBLIC_RESOURCES = ROOT / "internal" / "knowledge" / "data" / "public_resources.json"

# Curated concept groups derived from the source's 8 flat keyword lists.
# Each group is written bidirectionally (any member in a query expands to
# the rest), per the alias_map convention. Deliberate exclusions:
# 胸痛 is a symptom, not a synonym of 心绞痛/心肌梗死; 肺气肿 and 结肠炎
# are distinct entities from 慢阻肺/肠炎; 认知障碍 stays out of the
# 阿尔茨海默病 group (hypernym too broad).
SYNONYM_GROUPS = [
    ["高血压", "血压高", "血压升高"],
    ["冠心病", "冠状动脉粥样硬化", "冠状动脉粥样硬化性心脏病"],
    ["心肌梗死", "心梗"],
    ["心力衰竭", "心衰", "心脏衰竭"],
    ["肺炎", "肺部感染", "支气管肺炎"],
    ["哮喘", "支气管哮喘", "气喘"],
    ["慢阻肺", "COPD", "慢性阻塞性肺疾病"],
    ["胃炎", "胃病"],
    ["肝炎", "肝病"],
    ["乙肝", "乙型肝炎"],
    ["糖尿病", "血糖高", "高血糖"],
    ["甲亢", "甲状腺功能亢进", "大脖子病"],
    ["甲减", "甲状腺功能减退"],
    ["甲状腺肿", "大脖子病"],
    ["脑卒中", "中风", "脑梗", "脑血栓", "脑血管意外"],
    ["癫痫", "羊角风", "羊癫疯"],
    ["帕金森病", "帕金森", "震颤麻痹"],
    ["阿尔茨海默病", "老年痴呆"],
    ["发热", "发烧", "高烧"],
    ["腹泻", "拉肚子", "坏肚子"],
    ["腹痛", "肚子疼"],
    ["胃痛", "胃疼"],
    ["头痛", "头疼"],
    ["呼吸困难", "气短", "喘不上气", "憋气"],
    ["乏力", "疲倦", "疲劳", "没精神"],
    ["嗜睡", "犯困"],
    ["血常规", "验血", "抽血"],
    ["B超", "超声", "彩超", "超声波"],
    ["核磁", "磁共振", "MRI"],
    ["肠镜", "结肠镜"],
    ["感冒", "上呼吸道感染"],
]


def norm_name(s: str) -> str:
    s = re.sub(r"[（()）\s《》]", "", s)
    s = re.sub(r"(20\d\d|修订|年版|版)", "", s)
    return s


def merge_alias_map(entries: list) -> tuple:
    alias = json.loads(ALIAS_MAP.read_text(encoding="utf-8"))
    added_keys = added_vals = 0
    for group in SYNONYM_GROUPS:
        for member in group:
            others = [x for x in group if x != member]
            cur = alias.get(member)
            if cur is None:
                alias[member] = others
                added_keys += 1
                continue
            before = len(cur)
            alias[member] = cur + [x for x in others if x not in cur]
            added_vals += len(alias[member]) - before
    ALIAS_MAP.write_text(
        json.dumps(alias, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    return added_keys, added_vals


def to_public_resource(x: dict) -> dict:
    cat = x.get("category", "")
    title = x.get("title", "")
    if cat == "terminology_standard":
        category, rid = "术语标准", x["id"]
        name_en = title.split(" - ")[0].strip() if " - " in title else ""
    else:
        category = "共识" if "共识" in title else "指南"
        rid = x["id"]
        name_en = ""
    desc = x.get("summary", "")
    extras = []
    if x.get("pub_date"):
        extras.append("发布时间: " + x["pub_date"])
    if x.get("source"):
        extras.append("聚合来源: " + x["source"])
    if extras:
        desc += "（" + "；".join(extras) + "）"
    return {
        "id": "pub_" + rid.replace("-", "_"),
        "name_zh": x.get("title_zh") or title,
        "name_en": name_en,
        "category": category,
        "description_zh": desc,
        "description_en": "",
        "url": x.get("url", ""),
        "keywords": x.get("keywords", []),
    }


def merge_public_resources(entries: list) -> int:
    doc = json.loads(PUBLIC_RESOURCES.read_text(encoding="utf-8"))
    existing = doc["resources"]
    by_id = {r["id"] for r in existing}
    by_name = {norm_name(r["name_zh"]) for r in existing}
    # Several source rows share bare aggregator directory URLs (e.g.
    # guide.medlive.cn/guide/) — those identify no document, so only
    # source-unique URLs may participate in dedupe.
    urls = [x.get("url", "") for x in entries if x.get("category") != "synonym"]
    specific = {u for u in urls if u and urls.count(u) == 1}
    by_url = {r.get("url", "") for r in existing
              if r.get("url") and r["url"] in specific}
    added = 0
    for x in entries:
        if x.get("category") == "synonym":
            continue
        row = to_public_resource(x)
        if row["id"] in by_id or norm_name(row["name_zh"]) in by_name:
            continue
        if row["url"] and row["url"] in by_url:
            continue
        existing.append(row)
        by_id.add(row["id"])
        by_name.add(norm_name(row["name_zh"]))
        if row["url"] in specific:
            by_url.add(row["url"])
        added += 1
    PUBLIC_RESOURCES.write_text(
        json.dumps(doc, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    return added


def main() -> None:
    src = json.loads(SOURCE.read_text(encoding="utf-8"))
    entries = src["entries"]
    k, v = merge_alias_map(entries)
    n = merge_public_resources(entries)
    print(f"alias_map: +{k} keys, +{v} values")
    print(f"public_resources: +{n} rows")


if __name__ == "__main__":
    main()
