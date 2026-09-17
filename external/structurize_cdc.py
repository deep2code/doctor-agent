#!/usr/bin/env python3
"""Structurize China CDC (中国疾控中心) health-alert articles into
KnowledgeEntry JSON via LLM (requires an LLM API key — see FALLBACKS).

Each monthly/seasonal alert article covers several diseases; the LLM splits
it into one KnowledgeEntry per disease (risk factors, prevention, red flags).

Pipeline:
  1. Read external/cdc/{column}/{fname}.txt (title + body).
  2. Prompt the LLM for a JSON array of disease-focused entries.
  3. Validate + write external/cdc_structured/{column}_{fname}.json.
  4. Merge all into external/cdc/cdc_alerts.json (pipeline artifact; the
     embedded file is internal/knowledge/data/cdc_entries.json).

Idempotent: skips existing structured files.
"""
import json
import os
import re
import sys
import time
import urllib.request
from pathlib import Path

SRC = Path(__file__).parent / "cdc"
OUT_DIR = Path(__file__).parent / "cdc_structured"
MERGED = Path(__file__).parent / "cdc" / "cdc_alerts.json"

FALLBACKS = [
    {"url": "https://open.bigmodel.cn/api/paas/v4/chat/completions",
     "model": "glm-4-flash", "key_env": "ZHIPU_API_KEY"},
    {"url": "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions",
     "model": "qwen-plus", "key_env": "QWEN_API_KEY"},
    {"url": "https://api.siliconflow.cn/v1/chat/completions",
     "model": "deepseek-ai/DeepSeek-V3", "key_env": "SILICONFLOW_API_KEY"},
]

SYSTEM_PROMPT = """你是循证医学知识库工程师，负责把中国疾控中心(China CDC)的官方健康风险提示文章拆分成结构化的中文医学知识条目 JSON 数组。

规则：
1. 所有内容必须严格来自给定文章，禁止编造、补充或外推原文没有的信息。
2. 文章通常按月/节假日给出"需关注的疾病"清单及防控要点；把每个疾病拆成一条独立条目。
3. 每条短语不超过 40 字；专业术语保留英文括号注释。
4. 输出必须是合法 JSON 数组，不要输出任何其他文字、注释或 markdown 代码块标记。"""

USER_TEMPLATE = """请把以下中国疾控中心健康提示文章拆分成疾病条目。

文章标题: {title}
来源URL: {url}

JSON 数组元素结构（字段名固定，无内容的字段省略）：
{{
  "id": "cdc-{yyyymm}-{n}",
  "condition_zh": "<疾病中文名>",
  "condition_en": "<英文名>",
  "category": "<infectious_disease/chronic_disease/environmental_health/injury_prevention/nutrition/other>",
  "season": "<月份或节假日，如 2026年8月>",
  "risk_factors": ["<风险因素要点>", "..."],
  "prevention": ["<防控要点>", "..."],
  "symptoms": ["<主要症状要点>", "..."],
  "when_to_seek_care": ["<就医指征要点>", "..."],
  "keywords": ["<检索关键词：中文病名/口语词/英文，3-8 个>"]
}}

-----文章正文开始-----
{body}
-----文章正文结束-----"""


def call_llm(title: str, url: str, yyyymm: str, body: str) -> str:
    user = USER_TEMPLATE.format(title=title, url=url, yyyymm=yyyymm, n=1, body=body)
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


def parse_json(text: str) -> list:
    text = text.strip()
    if text.startswith("```"):
        text = re.sub(r"^```(json)?", "", text).strip()
        text = re.sub(r"```$", "", text).strip()
    return json.loads(text)


def main() -> int:
    OUT_DIR.mkdir(parents=True, exist_ok=True)
    merged = []
    ok, fail = 0, []
    files = sorted(SRC.glob("**/*.txt"))
    for i, f in enumerate(files, 1):
        rel = "_".join(f.relative_to(SRC).with_suffix("").parts)
        out = OUT_DIR / f"{rel}.json"
        if out.exists():
            merged += json.load(open(out, encoding="utf-8"))
            ok += 1
            continue
        title, body = f.read_text(encoding="utf-8").split("\n\n", 1)
        body = body.strip()
        if len(body) < 300:
            fail.append((rel, "正文过短"))
            continue
        fname = f.name[:-4]  # t20260805_1838674
        m_date = re.search(r"t(\d{6})(\d{2})_(\d+)", f.name)  # t20260428_1835392
        yyyymm = m_date.group(1) if m_date else "2026"
        url = f"https://www.chinacdc.cn/jkts/{yyyymm}/{fname}.html" if m_date else f"https://www.chinacdc.cn/{f.name}"
        try:
            raw = call_llm(title, url, yyyymm, body)
            entries = parse_json(raw)
            if not isinstance(entries, list) or not entries:
                raise ValueError(f"非数组或为空: {raw[:200]}")
            # renumber ids sequentially
            for n, e in enumerate(entries, 1):
                e["id"] = f"cdc-{yyyymm}-{n}"
                if not e.get("condition_zh") or not e.get("keywords"):
                    raise ValueError(f"缺少字段: {json.dumps(e, ensure_ascii=False)[:200]}")
            out.write_text(json.dumps(entries, ensure_ascii=False, indent=1), encoding="utf-8")
            merged += entries
            ok += 1
            print(f"[{i}/{len(files)}] ✅ {rel} ({len(entries)} 条疾病)", file=sys.stderr)
        except Exception as e:
            fail.append((rel, str(e)[:100]))
            print(f"[{i}/{len(files)}] ❌ {rel}: {str(e)[:100]}", file=sys.stderr)
        time.sleep(1)

    out_set = {
        "source": "中国疾控中心官方健康提示 (China CDC)",
        "updated": time.strftime("%Y-%m-%d"),
        "entries": merged,
    }
    MERGED.write_text(json.dumps(out_set, ensure_ascii=False, indent=1), encoding="utf-8")
    print(f"DONE: ok={ok} fail={len(fail)} entries={len(merged)} -> {MERGED}", file=sys.stderr)
    return 0 if not fail else 1


if __name__ == "__main__":
    sys.exit(main())
