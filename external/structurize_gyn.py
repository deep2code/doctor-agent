#!/usr/bin/env python3
"""Structurize gynecology (妇科) documents into KnowledgeEntry JSON via LLM.

Input:  external/gyn/{slug}.txt   (from fetch_gyn.py)
Output: external/gyn_structured/{slug}.json  (per-doc entries, idempotent)
        internal/knowledge/data/gyn_health.json (merged, bare array)

  python3 external/structurize_gyn.py
"""
import json
import os
import re
import sys
import time
import urllib.request
from pathlib import Path

SRC = Path(__file__).parent / "gyn"
OUT_DIR = Path(__file__).parent / "gyn_structured"
MERGED = Path(__file__).parent.parent / "internal" / "knowledge" / "data" / "gyn_health.json"

FALLBACKS = [
    {"url": "https://open.bigmodel.cn/api/paas/v4/chat/completions",
     "model": "glm-4-flash", "key_env": "ZHIPU_API_KEY"},
    {"url": "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions",
     "model": "qwen-plus", "key_env": "QWEN_API_KEY"},
    {"url": "https://api.siliconflow.cn/v1/chat/completions",
     "model": "deepseek-ai/DeepSeek-V3", "key_env": "SILICONFLOW_API_KEY"},
]

# slug -> (citation title, year, url, split hint)
DOCS = {
    "cervical-screening-2021": (
        "宫颈癌筛查工作方案(国卫办妇幼函〔2021〕635号)", 2021,
        "https://www.nwccw.gov.cn/2022/02/17/99337480.html",
        "拆条:宫颈癌筛查策略(对象/年龄/间隔/方法如 HPV 检测与 TCT)/筛查异常管理与随访/宫颈癌防治健康教育/ HPV 疫苗接种要点,4-6 条"),
    "breast-screening-2021": (
        "乳腺癌筛查工作方案(国卫办妇幼函〔2021〕635号)", 2021,
        "https://www.nwccw.gov.cn/2022/02/17/99337480.html",
        "拆条:乳腺癌筛查策略(对象/年龄/间隔/方法如乳腺 X 线与超声)/筛查异常管理与随访/乳腺癌防治健康教育/高危人群,4-6 条"),
    "menopause-2025": (
        "更年期女性健康教育核心信息(国家妇幼健康中心 2025)", 2025,
        "https://www.sxcdc.cn/zxzx/rdjd/art/2025/art_cdfa05da8e2247f3baf9f2cfadb4836b.html",
        "15 条核心信息按主题聚合拆条:更年期保健总体/膳食营养运动/慢病与心理/就医指征与月经异常/盆底健康与尿失禁/绝经激素治疗/中医药,5-7 条;原文只有条目标题,不得外推具体内容"),
}

SYSTEM_PROMPT = """你是循证医学知识库工程师，负责把国家卫健委等官方发布的妇科健康权威文件拆分成结构化的中文医学知识条目 JSON 数组。

规则：
1. 所有内容必须严格来自给定文件，禁止编造、补充或外推原文没有的信息。
2. 每条短语不超过 40 字；专业术语保留英文括号注释。
3. keywords 必须包含中文正式病名/术语 + 老百姓口语说法 + 英文，5-10 个。
4. 输出必须是合法 JSON 数组，不要输出任何其他文字、注释或 markdown 代码块标记。"""

USER_TEMPLATE = """请把以下官方妇科健康文件拆分成知识条目。

文件标题: {title}
拆分提示: {hint}

JSON 数组元素结构（字段名固定，无内容的字段省略；citations 按我给的固定引用生成）：
[{{
  "id": "gyn-{slug}-<序号>",
  "condition_zh": "<主题/疾病中文名，如 宫颈癌筛查、乳腺癌筛查、更年期保健>",
  "condition_en": "<英文名>",
  "category": "gyn_health",
  "diagnosis": {{"clinical_features": ["<临床表现要点>"]}},
  "risk_factors": ["<危险因素>"],
  "prevention": ["<预防/筛查/保健要点>"],
  "treatment": [{{"name": "<治疗/干预方式>", "details": "<要点>"}}],
  "when_to_seek_care": ["<就医指征>"],
  "citations": [{{"type": "national_guideline", "title": "{cite_title}", "journal": "", "year": {year}, "doi": "", "pmid": "", "level": "official_guidance", "url": "{url}"}}],
  "keywords": ["<关键词>"]
}}]

-----文件正文开始-----
{body}
-----文件正文结束-----"""


def call_llm(user: str) -> str:
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
            "max_tokens": 8192,
            "temperature": 0.2,
            "response_format": {"type": "json_object"},
        }).encode()
        for i in range(3):
            try:
                req = urllib.request.Request(f["url"], data=payload, headers={
                    "Authorization": f"Bearer {key}", "Content-Type": "application/json"})
                with urllib.request.urlopen(req, timeout=180) as r:
                    d = json.loads(r.read())
                return d["choices"][0]["message"]["content"]
            except Exception as e:
                last_err = e
                if getattr(e, "code", None) in (402, 401) or i == 2:
                    print(f"     [{f['model']}]: {str(e)[:70]}", file=sys.stderr)
                    break
                time.sleep(5)
    raise RuntimeError(f"所有 provider 失败: {last_err}")


def parse_json_array(text: str) -> list:
    text = text.strip()
    if text.startswith("```"):
        text = re.sub(r"^```(json)?", "", text).strip()
        text = re.sub(r"```$", "", text).strip()
    data = json.loads(text)
    if isinstance(data, dict):
        for k in ("entries", "data", "items", "results"):
            if isinstance(data.get(k), list):
                return data[k]
        raise ValueError(f"对象无数组字段: {list(data)[:5]}")
    return data


def main() -> int:
    OUT_DIR.mkdir(parents=True, exist_ok=True)
    merged = []
    ok, fail = 0, []
    for f in sorted(SRC.glob("*.txt")):
        slug = f.stem
        if slug not in DOCS:
            continue
        out = OUT_DIR / f"{slug}.json"
        if out.exists():
            merged += json.load(open(out, encoding="utf-8"))
            ok += 1
            continue
        cite_title, year, url, hint = DOCS[slug]
        doc_title, body = f.read_text(encoding="utf-8").split("\n\n", 1)
        user = USER_TEMPLATE.format(
            title=doc_title, hint=hint, slug=slug,
            cite_title=cite_title, year=year, url=url, body=body.strip())
        try:
            entries = parse_json_array(call_llm(user))
            if not entries:
                raise ValueError("空数组")
            for n, e in enumerate(entries, 1):
                e["id"] = f"gyn-{slug}-{n}"
                if not e.get("condition_zh") or not e.get("keywords") or not e.get("citations"):
                    raise ValueError(f"缺少必需字段: {json.dumps(e, ensure_ascii=False)[:150]}")
            out.write_text(json.dumps(entries, ensure_ascii=False, indent=1), encoding="utf-8")
            merged += entries
            ok += 1
            print(f"✅ {slug}: {len(entries)} 条", file=sys.stderr)
        except Exception as e:
            fail.append((slug, str(e)[:120]))
            print(f"❌ {slug}: {str(e)[:120]}", file=sys.stderr)
        time.sleep(1)

    MERGED.write_text(json.dumps(merged, ensure_ascii=False, indent=1), encoding="utf-8")
    print(f"DONE: ok={ok} fail={len(fail)} entries={len(merged)} -> {MERGED}", file=sys.stderr)
    return 0 if not fail else 1


if __name__ == "__main__":
    sys.exit(main())
