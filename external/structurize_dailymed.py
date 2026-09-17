#!/usr/bin/env python3
"""Structurize downloaded FDA drug labels into Chinese drug-knowledge entries.

Pipeline (requires an LLM API key — set ZHIPU_API_KEY / QWEN_API_KEY /
SILICONFLOW_API_KEY / ANTHROPIC_API_KEY as available):
  1. Read external/dailymed/{slug}.json (raw FDA prescribing sections).
  2. Prompt the LLM to produce a concise Chinese drug-knowledge entry strictly
     derived from the label text (no invented facts, no dosages verbatim
     beyond what the label states).
  3. Validate + write external/dailymed_structured/{slug}.json.
  4. Merge all into internal/knowledge/data/fda_drug_labels.json.

Idempotent: skips slugs whose structured file already exists.
"""
import json
import os
import re
import sys
import time
import urllib.parse
import urllib.request
from pathlib import Path

ROOT = Path(__file__).parent.parent
SRC = Path(__file__).parent / "dailymed"
OUT_DIR = Path(__file__).parent / "dailymed_structured"
MERGED = ROOT / "internal" / "knowledge" / "data" / "fda_drug_labels.json"

# Zhipu glm-4.7-flash is FREE (thinking disabled via the API param so it
# behaves like a plain chat model); Qwen/SiliconFlow fallbacks are
# intentionally removed so a Zhipu rate-limit can never silently charge paid
# providers. If Zhipu fails, the drug is skipped (idempotent — rerun later).
FALLBACKS = [
    {"url": "https://open.bigmodel.cn/api/paas/v4/chat/completions",
     "model": "glm-4.7-flash", "key_env": "ZHIPU_API_KEY"},
]

SYSTEM_PROMPT = """你是循证医学知识库工程师，负责把 FDA 药品标签(英文)转成简洁的中文药品知识条目 JSON。

规则：
1. 所有内容必须严格来自给定标签文本，禁止编造、补充或外推任何原文没有的信息；标签没有的字段一律省略。
2. 每条短语不超过 40 字，用中文；专业术语保留英文括号注释（如 过敏性休克(anaphylaxis)）。
3. 剂量信息(dosage)只保留标签中明确给出的常规成人剂量要点，禁止自行换算或推荐。
4. 输出必须是合法 JSON 对象，不要输出任何其他文字、注释或 markdown 代码块标记。"""

USER_TEMPLATE = """请把以下 FDA 药品标签转成中文药品知识 JSON。

药品 INN: {inn}
来源 URL: {url}

JSON 结构（字段名固定，无内容的字段省略）：
{{
  "name_zh": "<药品中文通用名>",
  "name_en": "<INN 英文名>",
  "category": "<药物类别，如 抗生素/降糖药/降压药/镇痛药/抗凝药…>",
  "indications": ["<适应症要点>", "..."],
  "contraindications": ["<禁忌要点>", "..."],
  "warnings": ["<警告与注意事项要点>", "..."],
  "interactions": ["<药物相互作用要点>", "..."],
  "adverse_reactions": ["<常见不良反应要点>", "..."],
  "dosage": ["<常规成人剂量要点>", "..."],
  "keywords": ["<检索关键词：中文药名、疾病名、英文同义词，5-10 个>"]
}}

-----标签正文开始-----
{body}
-----标签正文结束-----"""


def call_llm(inn: str, url: str, body: str) -> str:
    user = USER_TEMPLATE.format(inn=inn, url=url, body=body)
    last_err = None
    for f in FALLBACKS:
        key = os.environ.get(f["key_env"], "")
        if not key:
            continue
        payload = json.dumps({
            "model": f["model"],
            "messages": [
                {"role": "system", "content": SYSTEM_PROMPT},
                {"role": "user", "content": user},
            ],
            "max_tokens": 4096,
            "temperature": 0.2,
            # glm-4.7-flash is a reasoning model; disable thinking so the
            # answer comes back in message.content (plain chat behavior).
            "thinking": {"type": "disabled"},
            "response_format": {"type": "json_object"},
        }).encode()
        req = urllib.request.Request(f["url"], data=payload, headers={
            "Authorization": f"Bearer {key}", "Content-Type": "application/json"})
        for i in range(3):
            try:
                with urllib.request.urlopen(req, timeout=180) as r:
                    d = json.loads(r.read())
                return d["choices"][0]["message"]["content"]
            except Exception as e:
                last_err = e
                if getattr(e, "code", None) == 402 or i == 2:
                    print(f"     [{f['model']}]: {str(e)[:60]}", file=sys.stderr)
                    break
                time.sleep(5)
    raise RuntimeError(f"所有 provider 失败: {last_err}")


def normalize_entry(entry: dict) -> dict:
    """Post-process an LLM-produced entry: split comma-joined keywords."""
    kws = entry.get("keywords")
    if isinstance(kws, str):
        kws = [kws]
    if kws:
        split = []
        for k in kws:
            for part in re.split(r"[，,;；]", k):
                part = part.strip()
                if part:
                    split.append(part)
        entry["keywords"] = list(dict.fromkeys(split))
    return entry


def parse_json(text: str) -> dict:
    text = text.strip()
    if text.startswith("```"):
        text = re.sub(r"^```(json)?", "", text).strip()
        text = re.sub(r"```$", "", text).strip()
    return json.loads(text)


def build_body(rec: dict) -> str:
    """Assemble the label sections into one bounded text block."""
    parts = []
    for k, v in rec["sections"].items():
        if not v:
            continue
        parts.append(f"[{k}]\n{v[:3000]}")
    body = "\n\n".join(parts)
    return body[:16000]  # keep prompt bounded


def main() -> int:
    OUT_DIR.mkdir(parents=True, exist_ok=True)
    merged = []
    files = sorted(SRC.glob("*.json"))
    files = [f for f in files if f.name != "index.json"]
    ok, fail = 0, []
    for i, f in enumerate(files, 1):
        out = OUT_DIR / f.name
        if out.exists():
            merged.append(json.load(open(out, encoding="utf-8")))
            ok += 1
            continue
        rec = json.loads(f.read_text(encoding="utf-8"))
        body = build_body(rec)
        if len(body) < 200:
            fail.append((f.stem, "正文过短"))
            print(f"[{i}/{len(files)}] ❌ {f.stem}: 正文过短", file=sys.stderr)
            continue
        try:
            raw = call_llm(rec["inn"], rec["url"], body)
            entry = parse_json(raw)
            entry = normalize_entry(entry)
            entry["name_en"] = rec["inn"]
            entry["rxcui"] = rec["rxcui"]
            entry["source_url"] = rec["url"]
            # a setid-less DailyMed URL is dead; fall back to a search URL
            if entry["source_url"].endswith("setid="):
                entry["source_url"] = (
                    "https://dailymed.nlm.nih.gov/dailymed/search.cfm?labeltype=all"
                    f"&query={urllib.parse.quote(rec['generic_name'] or rec['inn'])}"
                )
            if not entry.get("name_zh") or not entry.get("keywords"):
                raise ValueError(f"缺少字段: {raw[:200]}")
            out.write_text(json.dumps(entry, ensure_ascii=False, indent=1), encoding="utf-8")
            merged.append(entry)
            ok += 1
            print(f"[{i}/{len(files)}] ✅ {f.stem}", file=sys.stderr)
        except Exception as e:
            fail.append((f.stem, str(e)[:100]))
            print(f"[{i}/{len(files)}] ❌ {f.stem}: {str(e)[:100]}", file=sys.stderr)
        time.sleep(1)

    out_set = {
        "source": "FDA drug labels (DailyMed/OpenFDA), curated into Chinese",
        "updated": time.strftime("%Y-%m-%d"),
        "drugs": merged,
    }
    MERGED.write_text(json.dumps(out_set, ensure_ascii=False, indent=1), encoding="utf-8")
    print(f"DONE: ok={ok} fail={len(fail)} -> {MERGED}", file=sys.stderr)
    if fail:
        print("失败:", fail[:20], file=sys.stderr)
    return 0 if not fail else 1


if __name__ == "__main__":
    sys.exit(main())
