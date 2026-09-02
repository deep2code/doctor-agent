#!/usr/bin/env python3
"""Convert CDC milestones into internal/knowledge/data/development_milestones.json.

Input:  external/growth/cdc_milestones.json (fetched via server-side reader,
        12 ages x 4 domains, English verbatim from CDC 2022 revision pages).
LLM:    per-age batch translation (Zhipu glm-4-flash, free) adds zh fields.
Output: internal/knowledge/data/development_milestones.json — same structure
        with *_zh arrays aligned index-by-index with the English ones.

Idempotent: per-age cache in external/growth/milestone_cache/.
Run from repo root: python3 external/convert_milestones.py
"""
import json
import os
import re
import sys
import time
import urllib.request
from pathlib import Path

ROOT = Path(__file__).parent.parent
SRC = ROOT / "external" / "growth" / "cdc_milestones.json"
CACHE = ROOT / "external" / "growth" / "milestone_cache"
OUT = ROOT / "internal" / "knowledge" / "data" / "development_milestones.json"

ZHIPU_URL = "https://open.bigmodel.cn/api/paas/v4/chat/completions"
ZHIPU_MODEL = "glm-4-flash"

DOMAINS = ["social_emotional", "language_communication", "cognitive", "movement_physical"]
DOMAIN_ZH = {
    "social_emotional": "社交/情绪",
    "language_communication": "语言/沟通",
    "cognitive": "认知（学习、思考、解决问题）",
    "movement_physical": "运动/体格发育",
}


def llm_translate(age: dict) -> dict:
    """Return {domain: [zh, ...]} aligned with the English lists."""
    key = os.environ.get("ZHIPU_API_KEY")
    if not key:
        return {}
    parts = [f"儿童年龄：{age['age_label_zh']}（{age['age_label_en']}）\n"]
    for dom in DOMAINS:
        parts.append(f"{DOMAIN_ZH[dom]}：")
        for i, en in enumerate(age.get(dom, [])):
            parts.append(f"{i}. {en}")
        parts.append("")
    prompt = (
        "下面是 CDC 儿童发育里程碑清单（2022 修订版，指 75% 或更多儿童在该年龄能做到的行为）"
        "某个年龄的四类条目。请把每条翻译成准确、口语自然、家长能懂的中文"
        "（医学/发育术语规范；he/she 统一译作\"孩子\"或\"他/她\"均可但保持自然；保留举例）。\n"
        "输出 JSON（不要 markdown 代码块、不要解释），键用域名，值为中文数组，"
        "顺序与条目编号一一对应：\n"
        '{"social_emotional": ["...", ...], "language_communication": [...], '
        '"cognitive": [...], "movement_physical": [...]}\n\n' + "\n".join(parts)
    )
    payload = json.dumps({
        "model": ZHIPU_MODEL,
        "messages": [{"role": "user", "content": prompt}],
        "temperature": 0.1,
    }).encode()
    req = urllib.request.Request(ZHIPU_URL, data=payload, headers={
        "Content-Type": "application/json", "Authorization": f"Bearer {key}"})
    for attempt in range(3):
        try:
            with urllib.request.urlopen(req, timeout=90) as resp:
                data = json.loads(resp.read())
                content = data["choices"][0]["message"]["content"].strip()
                content = re.sub(r"^```(?:json)?|```$", "", content, flags=re.M).strip()
                parsed = json.loads(content)
                out = {}
                for dom in DOMAINS:
                    zh_list = parsed.get(dom)
                    if isinstance(zh_list, list) and len(zh_list) == len(age.get(dom, [])):
                        out[dom] = zh_list
                if len(out) == len(DOMAINS):
                    return out
                print(f"  {age['age_key']}: LLM domain mismatch, retry", file=sys.stderr)
        except Exception as e:
            if attempt == 2:
                print(f"  LLM fail {age['age_key']}: {e}", file=sys.stderr)
                return {}
            time.sleep(3)
    return {}


def main():
    CACHE.mkdir(parents=True, exist_ok=True)
    doc = json.loads(SRC.read_text())
    total, zh_ok = 0, 0
    for age in doc["ages"]:
        cf = CACHE / f"{age['age_key']}.json"
        if cf.exists():
            tr = json.loads(cf.read_text())
        else:
            tr = llm_translate(age)
            if tr:
                cf.write_text(json.dumps(tr, ensure_ascii=False))
            time.sleep(1)
        for dom in DOMAINS:
            ens = age.get(dom, [])
            total += len(ens)
            if dom in tr and len(tr[dom]) == len(ens):
                age[f"{dom}_zh"] = tr[dom]
                zh_ok += len(ens)
            else:
                age[f"{dom}_zh"] = []
    print(f"milestones: {total} items, zh translated {zh_ok}")
    OUT.write_text(json.dumps(doc, ensure_ascii=False))
    print(f"size: {OUT.stat().st_size/1024:.1f} KB -> {OUT}")


if __name__ == "__main__":
    main()
