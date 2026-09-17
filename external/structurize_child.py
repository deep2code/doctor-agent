#!/usr/bin/env python3
"""Structurize child-health查漏 documents into KnowledgeEntry JSON via LLM.

Input:  external/child/{slug}.txt   (from fetch_child_health.py)
Output: external/child_structured/{slug}.json
        internal/knowledge/data/ortho_child_health.json (merged, bare array)

  python3 external/structurize_child.py
"""
import json
import os
import re
import sys
import time
import urllib.request
from pathlib import Path

SRC = Path(__file__).parent / "child"
OUT_DIR = Path(__file__).parent / "child_structured"
MERGED = Path(__file__).parent.parent / "internal" / "knowledge" / "data" / "ortho_child_health.json"

FALLBACKS = [
    {"url": "https://open.bigmodel.cn/api/paas/v4/chat/completions",
     "model": "glm-4-flash", "key_env": "ZHIPU_API_KEY"},
    {"url": "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions",
     "model": "qwen-plus", "key_env": "QWEN_API_KEY"},
    {"url": "https://api.siliconflow.cn/v1/chat/completions",
     "model": "deepseek-ai/DeepSeek-V3", "key_env": "SILICONFLOW_API_KEY"},
]

DOCS = {
    "autism-screening-2022": (
        "0~6岁儿童孤独症筛查干预服务规范(试行)(国卫办妇幼发〔2022〕12号)", 2022,
        "https://www.gov.cn/zhengce/zhengceku/2022-09/23/content_5711379.htm",
        "拆条:孤独症筛查服务流程(11 次初筛时间安排)/儿童心理行为发育问题预警征象(按月龄分组列出各月龄预警征象)/复筛与诊断转诊/干预康复原则/家长健康教育,5-8 条;预警征象按月龄完整保留原文表述"),
}

SYSTEM_PROMPT = """你是循证医学知识库工程师，负责把国家卫健委官方发布的儿童健康权威文件拆分成结构化的中文医学知识条目 JSON 数组。

规则：
1. 所有内容必须严格来自给定文件，禁止编造、补充或外推原文没有的信息。
2. 每条短语不超过 40 字；专业术语保留英文括号注释。
3. keywords 必须包含中文正式术语 + 家长口语说法 + 英文，5-10 个。
4. 输出必须是合法 JSON 数组，不要输出任何其他文字、注释或 markdown 代码块标记。"""

USER_TEMPLATE = """请把以下官方儿童健康文件拆分成知识条目。

文件标题: {title}
拆分提示: {hint}

JSON 数组元素结构（字段名固定，无内容的字段省略；citations 按我给的固定引用生成）：
[{{
  "id": "child-{slug}-<序号>",
  "condition_zh": "<主题/疾病中文名，如 孤独症筛查、儿童心理行为发育预警征象>",
  "condition_en": "<英文名>",
  "category": "child_health",
  "diagnosis": {{"clinical_features": ["<表现/预警征象要点>"]}},
  "risk_factors": ["<危险因素>"],
  "prevention": ["<预防/筛查要点>"],
  "treatment": [{{"name": "<干预方式>", "details": "<要点>"}}],
  "when_to_seek_care": ["<就医/转诊指征>"],
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
                e["id"] = f"child-{slug}-{n}"
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
