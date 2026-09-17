#!/usr/bin/env python3
"""Structurize WHO vaccine position papers into KnowledgeEntry JSON.

Reads external/position_papers/{name}.txt (official WER position papers;
some are full-issue WER texts — the prompt tells the model to extract only
the target vaccine's position paper section), prompts an LLM for a Chinese
KnowledgeEntry, and merges all into internal/knowledge/data/who_vaccines.json.

Providers: Zhipu glm-4-flash (free) -> Qwen -> SiliconFlow (OpenAI-compatible).
Idempotent: skips names whose structured file exists.
"""
import json, os, re, sys, time, urllib.request
from pathlib import Path

ROOT = Path(__file__).parent.parent
SRC = Path(__file__).parent / "position_papers"
OUT_DIR = Path(__file__).parent / "pp_structured"
MERGED = ROOT / "internal" / "knowledge" / "data" / "who_vaccines.json"

PROVIDERS = [
    {"name": "zhipu", "url": "https://open.bigmodel.cn/api/paas/v4/chat/completions",
     "model": "glm-4-flash", "key_env": "ZHIPU_API_KEY"},
    {"name": "qwen", "url": "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions",
     "model": "qwen-plus", "key_env": "QWEN_API_KEY"},
    {"name": "siliconflow", "url": "https://api.siliconflow.cn/v1/chat/completions",
     "model": "deepseek-ai/DeepSeek-V3", "key_env": "SILICONFLOW_API_KEY"},
]

# name -> (中文标题, 官方 URL)
VACCINES = {
    "rabies": ("狂犬病疫苗", "https://www.who.int/publications/i/item/who-wer9324"),
    "japanese-encephalitis": ("乙型脑炎疫苗", "https://www.who.int/publications/i/item/who-wer9009-69-88"),
    "hpv": ("人乳头瘤病毒疫苗", "https://www.who.int/publications/i/item/who-wer9750-645-672"),
    "hepatitis-b": ("乙型肝炎疫苗", "https://www.who.int/publications/i/item/who-wer9233"),
    "dengue": ("登革热疫苗", "https://www.who.int/publications/i/item/who-wer9335"),
    "influenza": ("流感疫苗", "https://www.who.int/publications/i/item/who-wer9719"),
    "typhoid": ("伤寒疫苗", "https://www.who.int/publications/i/item/who-wer9313"),
    "cholera": ("霍乱疫苗", "https://www.who.int/publications/i/item/who-wer9234-477-500"),
    "tetanus-diphtheria": ("破伤风和白喉疫苗", "https://www.who.int/publications/i/item/who-wer9206"),
    "rotavirus": ("轮状病毒疫苗", "https://www.who.int/publications/i/item/who-wer9633"),
    "measles": ("麻疹疫苗", "https://www.who.int/publications/i/item/who-wer9217-205-227"),
    "pneumococcal": ("肺炎球菌疫苗", "https://www.who.int/publications/i/item/who-wer9420"),
}

SYSTEM_PROMPT = """你是循证医学知识库工程师，负责把世界卫生组织(WHO)的疫苗立场文件(position paper)转成结构化的中文医学知识条目 JSON。

规则：
1. 所有字段必须严格来自给定文本，禁止编造、补充或外推任何原文没有的信息。
2. 文本中未出现的字段一律省略（不输出该字段）。
3. 如果文本是整期《疫情周报》(Weekly Epidemiological Record)，只提取目标疫苗的 position paper 部分，忽略其他文章。
4. 输出必须是合法 JSON 对象，不要输出任何其他文字、注释或 markdown 代码块标记。
5. 疫苗条目重点字段：接种对象/推荐人群(risk_factors 或 diagnosis 描述)、接种程序(剂次、间隔，放入 treatment.method)、安全性(complications)、禁忌症(prevention 或 risk_factors)、保护效力。用简洁中文短语列表，每条不超过 40 字。
6. keywords 填 6-10 个检索关键词（中文症状/疾病/疫苗口语词 + 英文同义词）。
7. citations 固定为一条 WHO 立场文件引用：
   {"type": "who_position_paper", "title": "<中文标题>", "journal": "Weekly Epidemiological Record", "year": <年份>, "doi": "", "pmid": "", "level": "international_guideline", "url": "<官方URL>"}
8. 疫苗立场文件有 GRADE 证据分级，治疗/预防等建议的证据等级写 "international_guideline"。"""

USER_TEMPLATE = """请把以下 WHO 疫苗立场文件转成 KnowledgeEntry JSON。

疫苗: {title_zh}
官方URL: {url}
文件: {fname}

KnowledgeEntry 的 JSON 结构（字段名固定）：
{{
  "id": "who-vaccine-{slug}",
  "condition_zh": "{title_zh}",
  "condition_en": "<英文名>",
  "category": "vaccine",
  "regions": ["全国"],
  "diagnosis": {{"lab_tests": [], "clinical_features": ["<接种对象/推荐人群>"], "gold_standard": "<若有>"}},
  "treatment": [{{"method": "<接种程序，如 2剂次间隔4周>", "indication": "<适用人群>", "evidence_level": "international_guideline"}}],
  "differential_diagnosis": [],
  "risk_factors": ["<高危人群/禁忌症>"],
  "complications": ["<不良反应/安全性>"],
  "prevention": ["<接种建议/群体免疫>"],
  "citations": [<WHO 立场文件引用>],
  "keywords": ["<中文口语词>", "..."]
}}

文本如下：
-----文本开始-----
{body}
-----文本结束-----"""


def extract_section(body, fname, zh, slug):
    """For full-issue WER texts, locate the target vaccine's position paper
    and cut from there; single-article texts pass through unchanged."""
    if len(body) <= 20000:
        return body
    # Locate the article: try the "position paper" phrase, else the vaccine
    # name itself (full-issue text extraction sometimes loses the heading).
    idx = body.lower().find("position paper")
    if idx < 0 or idx > 30000:
        for key in (slug.replace("-", " "), zh.split("疫苗")[0]):
            idx = body.lower().find(key)
            if 1000 < idx < len(body) - 5000:
                break
    if 1000 < idx < len(body) - 5000:
        start = max(0, idx - 1000)
        return body[start:start + 32000]
    return body[:30000]


def call_llm(slug, title_zh, url, body, fname):
    body = extract_section(body, fname, title_zh, slug)
    user = USER_TEMPLATE.format(title_zh=title_zh, url=url, slug=slug, body=body, fname=fname)
    last_err = None
    for p in PROVIDERS:
        key = os.environ.get(p["key_env"], "")
        if not key:
            continue
        payload = json.dumps({
            "model": p["model"],
            "messages": [
                {"role": "system", "content": SYSTEM_PROMPT},
                {"role": "user", "content": user},
            ],
            "max_tokens": 4096,
            "temperature": 0.2,
            "response_format": {"type": "json_object"},
        }).encode()
        req = urllib.request.Request(p["url"], data=payload, headers={
            "Authorization": f"Bearer {key}", "Content-Type": "application/json"})
        for i in range(3):
            try:
                with urllib.request.urlopen(req, timeout=240) as r:
                    d = json.loads(r.read())
                return d["choices"][0]["message"]["content"]
            except Exception as e:
                last_err = e
                if getattr(e, "code", None) in (402, 403, 429) or i == 2:
                    print(f"     [{p['name']}] {p['model']}: {str(e)[:60]}", file=sys.stderr)
                    break
                time.sleep(5)
    raise RuntimeError(f"所有 provider 失败: {last_err}")


def parse_json(text):
    text = text.strip()
    if text.startswith("```"):
        text = re.sub(r"^```(json)?", "", text).strip()
        text = re.sub(r"```$", "", text).strip()
    return json.loads(text)


def main():
    OUT_DIR.mkdir(parents=True, exist_ok=True)
    merged, ok, fail = [], 0, []
    for i, (slug, (title_zh, url)) in enumerate(VACCINES.items(), 1):
        out = OUT_DIR / f"{slug}.json"
        if out.exists():
            merged.append(json.load(open(out)))
            ok += 1
            print(f"[{i}/{len(VACCINES)}] SKIP {slug}", file=sys.stderr)
            continue
        # prefer 中文整期文本, 否则英文
        src = None
        for cand in (SRC / f"{slug}-chi.txt", SRC / f"{slug}.txt"):
            if cand.exists() and cand.stat().st_size > 1000:
                src = cand
                break
        if not src:
            fail.append((slug, "无文本"))
            print(f"[{i}/{len(VACCINES)}] ❌ {slug}: 无文本", file=sys.stderr)
            continue
        body = src.read_text(encoding="utf-8", errors="replace")
        try:
            raw = call_llm(slug, title_zh, url, body, src.name)
            entry = parse_json(raw)
            for field in ("id", "condition_zh", "category", "keywords", "citations"):
                if not entry.get(field):
                    raise ValueError(f"缺少字段 {field}: {raw[:150]}")
            out.write_text(json.dumps(entry, ensure_ascii=False, indent=1), encoding="utf-8")
            merged.append(entry)
            ok += 1
            print(f"[{i}/{len(VACCINES)}] ✅ {slug} ({src.name}, {len(body)}字符)", file=sys.stderr)
        except Exception as e:
            fail.append((slug, str(e)[:100]))
            print(f"[{i}/{len(VACCINES)}] ❌ {slug}: {str(e)[:100]}", file=sys.stderr)
        time.sleep(1)

    if merged:
        merged.sort(key=lambda e: e["condition_zh"])
        MERGED.write_text(json.dumps(merged, ensure_ascii=False, indent=1), encoding="utf-8")
        print(f"\n✅ 合并写出 {MERGED} ({len(merged)} 条)", file=sys.stderr)
    print(f"完成: {ok}/{len(VACCINES)}; 失败 {len(fail)}: {fail}", file=sys.stderr)


if __name__ == "__main__":
    main()
