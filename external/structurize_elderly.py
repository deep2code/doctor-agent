#!/usr/bin/env python3
"""Structurize elderly-care (老年护理) documents into KnowledgeEntry JSON
via LLM (Zhipu glm-4-flash free tier, with Qwen/SiliconFlow fallbacks).

Input:  external/elderly/{slug}.txt   (from fetch_elderly.py)
Output: external/elderly_structured/{slug}.json  (per-doc entries, idempotent)
        internal/knowledge/data/elderly_care.json (merged, bare array —
        same shape as cdc_entries.json, seeded into the medical dataset)

  python3 external/structurize_elderly.py
"""
import json
import os
import re
import sys
import time
import urllib.request
from pathlib import Path

SRC = Path(__file__).parent / "elderly"
OUT_DIR = Path(__file__).parent / "elderly_structured"
MERGED = Path(__file__).parent.parent / "internal" / "knowledge" / "data" / "elderly_care.json"

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
    "disability-prevention-2019": (
        "老年失能预防核心信息(国卫办老龄函〔2019〕689号)", 2019,
        "https://www.gov.cn/zhengce/zhengceku/2019-11/18/content_5453051.htm",
        "16 条核心信息按主题聚合拆条:健康素养/营养/骨骼肌肉/疫苗接种/跌倒预防/心理/社会功能/慢病管理/用药/康复等,相近条目合并"),
    "alzheimer-2019": (
        "阿尔茨海默病预防与干预核心信息(国卫办老龄函〔2019〕738号)", 2019,
        "https://www.crsi.com.cn/Html/News/Articles/642.html",
        "10 条核心信息拆条:预防(生活方式/危险因素)/早期迹象识别/及时就医/治疗/家庭照护/友善社会,相近条目合并"),
    "elderly-dietary-2022": (
        "中国老年人膳食指南(2022)核心推荐", 2022,
        "http://dg.cnsoc.org/article/04/op9MZtpBQHehHCo0SSqsmw.html",
        "拆 2 条:一般老年人(65-79岁)膳食核心推荐、高龄老年人(80岁及以上)膳食核心推荐"),
    "osteoporosis-2011": (
        "防治骨质疏松知识要点(卫办疾控函〔2011〕542号)", 2011,
        "http://www.bjchy.gov.cn/affair/domain/yl/8a24fe83302019b101311d7b75e80e4e.html",
        "11 点提示按主题聚合拆条:疾病认识/营养(钙/维生素D)/运动/高危人群/生活方式/骨折预防与诊治等"),
}

SYSTEM_PROMPT = """你是循证医学知识库工程师，负责把国家卫健委等官方发布的老年健康权威文件拆分成结构化的中文医学知识条目 JSON 数组。

规则：
1. 所有内容必须严格来自给定文件，禁止编造、补充或外推原文没有的信息。
2. 每条短语不超过 40 字；专业术语保留英文括号注释。
3. keywords 必须包含中文正式病名/术语 + 老百姓口语说法 + 英文，5-10 个。
4. 输出必须是合法 JSON 数组，不要输出任何其他文字、注释或 markdown 代码块标记。"""

USER_TEMPLATE = """请把以下官方老年健康文件拆分成知识条目。

文件标题: {title}
拆分提示: {hint}

JSON 数组元素结构（字段名固定，无内容的字段省略；citations 按我给的固定引用生成）：
[{{
  "id": "elderly-{slug}-<序号>",
  "condition_zh": "<主题/疾病中文名，如 老年失能预防、阿尔茨海默病、骨质疏松症>",
  "condition_en": "<英文名>",
  "category": "elderly_care",
  "diagnosis": {{"clinical_features": ["<临床表现/早期迹象要点>"]}},
  "risk_factors": ["<危险因素>"],
  "prevention": ["<预防/保健要点>"],
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
    if isinstance(data, dict):  # json_object mode may wrap the array
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
                e["id"] = f"elderly-{slug}-{n}"
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
