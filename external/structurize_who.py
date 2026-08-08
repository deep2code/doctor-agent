#!/usr/bin/env python3
"""Structurize WHO Chinese fact sheets into KnowledgeEntry JSON via LLM.

Pipeline:
  1. Read external/who_factsheets_zh/{slug}.md (official Chinese WHO pages).
  2. Prompt DeepSeek-V3 (SiliconFlow, OpenAI-compatible) to produce a
     single KnowledgeEntry JSON strictly derived from the text.
  3. Validate + write external/who_factsheets_structured/{slug}.json.
  4. Merge all into internal/knowledge/data/who_factsheets.json.

Idempotent: skips slugs whose structured file already exists. Regenerate from
scratch by deleting external/who_factsheets_structured/.
"""
import json, os, re, sys, time, urllib.request
from pathlib import Path

ROOT = Path(__file__).parent.parent
SRC = Path(__file__).parent / "who_factsheets_zh"
OUT_DIR = Path(__file__).parent / "who_factsheets_structured"
MERGED = ROOT / "internal" / "knowledge" / "data" / "who_factsheets.json"

API_URL = "https://api.siliconflow.cn/v1/chat/completions"
MODEL = "deepseek-ai/DeepSeek-V3"
BASE_URL = "https://www.who.int/zh/news-room/fact-sheets/detail/{slug}"

# Fallback providers used when the primary runs out of quota. glm-4-flash is
# free; Qwen/DashScope and SiliconFlow are paid. All are OpenAI-compatible
# chat-completions endpoints.
FALLBACKS = [
    {"url": "https://open.bigmodel.cn/api/paas/v4/chat/completions",
     "model": "glm-4-flash", "key_env": "ZHIPU_API_KEY"},
    {"url": "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions",
     "model": "qwen-plus", "key_env": "QWEN_API_KEY"},
    {"url": "https://api.siliconflow.cn/v1/chat/completions",
     "model": "deepseek-ai/DeepSeek-V3", "key_env": "SILICONFLOW_API_KEY"},
]

# Selected topics: southern-China high-burden / complementary to the existing
# 10 conditions. slug -> (中文标题, 分类, 地区)
SELECTED = {
    "anaemia": ("贫血", "haematology", "华南,全国"),
    "dengue-and-severe-dengue": ("登革热", "infectious_disease", "华南,全国"),
    "hepatitis-b": ("乙型肝炎", "infectious_disease", "华南,全国"),
    "rabies": ("狂犬病", "infectious_disease", "全国"),
    "animal-bites": ("动物咬伤", "injury_prevention", "全国"),
    "snakebite-envenoming": ("蛇咬伤", "injury_prevention", "华南,全国"),
    "japanese-encephalitis": ("日本脑炎", "infectious_disease", "全国"),
    "malaria": ("疟疾", "infectious_disease", "华南边境,全国"),
    "chikungunya": ("基孔肯雅热", "infectious_disease", "华南,全国"),
    "zika-virus": ("寨卡病毒病", "infectious_disease", "华南,全国"),
    "mpox": ("猴痘", "infectious_disease", "全国"),
    "hepatitis-a": ("甲型肝炎", "infectious_disease", "全国"),
    "hepatitis-e": ("戊型肝炎", "infectious_disease", "华南,全国"),
    "typhoid": ("伤寒", "infectious_disease", "全国"),
    "cholera": ("霍乱", "infectious_disease", "华南,全国"),
    "salmonella-(non-typhoidal)": ("非伤寒沙门氏菌感染", "infectious_disease", "全国"),
    "campylobacter": ("弯曲杆菌感染", "infectious_disease", "全国"),
    "listeriosis": ("李斯特菌病", "infectious_disease", "全国"),
    "foodborne-trematode-infections": ("食源性吸虫感染", "infectious_disease", "华南,全国"),
    "schistosomiasis": ("血吸虫病", "infectious_disease", "长江流域,全国"),
    "hantavirus": ("汉坦病毒感染", "infectious_disease", "全国"),
    "tetanus": ("破伤风", "infectious_disease", "全国"),
    "tuberculosis": ("结核病", "infectious_disease", "全国"),
    "leprosy": ("麻风病", "infectious_disease", "华南,全国"),
    "scabies": ("疥疮", "infectious_disease", "华南,全国"),
    "ringworm-(tinea)": ("体癣", "infectious_disease", "华南,全国"),
    "candidiasis-(yeast-infection)": ("念珠菌病", "infectious_disease", "全国"),
    "sporotrichosis": ("孢子丝菌病", "infectious_disease", "华南,全国"),
    "chromoblastomycosis": ("着色芽生菌病", "infectious_disease", "华南,全国"),
    "sickle-cell-disease": ("镰状细胞病", "haematology", "全国"),
    "rheumatic-heart-disease": ("风湿性心脏病", "chronic_disease", "全国"),
    "hypertension": ("高血压", "chronic_disease", "华南,全国"),
    "diabetes": ("糖尿病", "chronic_disease", "华南,全国"),
    "mycotoxins": ("真菌毒素", "environmental_health", "华南,全国"),
    "natural-toxins-in-food": ("食物中的天然毒素", "environmental_health", "全国"),
    "ultraviolet-radiation": ("紫外线辐射", "environmental_health", "华南,全国"),
    "climate-change-heat-and-health": ("高温与健康", "environmental_health", "华南,全国"),
    "pre-eclampsia": ("子痫前期", "maternal_child_health", "全国"),
    "preterm-birth": ("早产", "maternal_child_health", "全国"),
    "drowning": ("溺水", "injury_prevention", "华南,全国"),
}

SYSTEM_PROMPT = """你是循证医学知识库工程师，负责把 WHO 官方中文事实页(fact sheet)转成结构化的中文医学知识条目 JSON。

规则：
1. 所有字段必须严格来自给定文本，禁止编造、补充或外推任何原文没有的信息。
2. 文本中未出现的字段一律省略（不输出该字段）。
3. 输出必须是合法 JSON 对象，不要输出任何其他文字、注释或 markdown 代码块标记。
4. prevalence 的 rate 用小数（如 20.5% 写成 0.205），只有文本明确给出数字时才填。
5. diagnosis.lab_tests / clinical_features、treatment、risk_factors、complications、prevention、differential_diagnosis 用简洁中文短语列表，每条不超过 30 字。
6. keywords 填 5-10 个检索关键词（中文症状/疾病口语词 + 英文同义词），用于中文检索召回。
7. citations 固定为一条 WHO fact sheet 引用：
   {"type": "who_factsheet", "title": "<中文标题>", "journal": "", "year": 2026, "doi": "", "pmid": "", "level": "review", "url": "<官方中文URL>"}
8. icd10 只在文本明确给出时填写。"""

USER_TEMPLATE = """请把以下 WHO 官方中文 fact sheet 转成 KnowledgeEntry JSON。

标题: {title_zh}
分类: {category}
地区: {regions}
官方URL: {url}
英文slug: {slug}

KnowledgeEntry 的 JSON 结构（字段名固定）：
{{
  "id": "who-{slug_safe}",
  "condition_zh": "{title_zh}",
  "condition_en": "<英文病名>",
  "category": "{category}",
  "icd10": "<若有>",
  "regions": ["{regions}"],
  "prevalence": {{"<地区>": {{"rate": 0.xx, "population": "<人群>"}}}},
  "diagnosis": {{"lab_tests": [...], "clinical_features": [...], "gold_standard": "<若有>"}},
  "treatment": [{{"method": "<方法>", "indication": "<指征>", "evidence_level": "review"}}],
  "differential_diagnosis": [...],
  "risk_factors": [...],
  "complications": [...],
  "prevention": [...],
  "citations": [<WHO fact sheet 引用>],
  "keywords": [...]
}}

正文如下：
-----正文开始-----
{body}
-----正文结束-----"""


def call_llm(slug, title_zh, category, regions, body):
    user = USER_TEMPLATE.format(
        title_zh=title_zh, category=category, regions=regions,
        url=BASE_URL.format(slug=slug),
        slug=slug, slug_safe=re.sub(r"[^a-z0-9]", "-", slug), body=body)
    providers = [("zhipu", f["url"], f["model"], os.environ.get(f["key_env"], ""))
                 for f in FALLBACKS]
    last_err = None
    for name, url, model, key in providers:
        if not key:
            continue
        payload = json.dumps({
            "model": model,
            "messages": [
                {"role": "system", "content": SYSTEM_PROMPT},
                {"role": "user", "content": user},
            ],
            "max_tokens": 4096,
            "temperature": 0.2,
            "response_format": {"type": "json_object"},
        }).encode()
        req = urllib.request.Request(url, data=payload, headers={
            "Authorization": f"Bearer {key}", "Content-Type": "application/json"})
        for i in range(3):
            try:
                with urllib.request.urlopen(req, timeout=180) as r:
                    d = json.loads(r.read())
                return d["choices"][0]["message"]["content"]
            except Exception as e:
                last_err = e
                if getattr(e, "code", None) == 402 or i == 2:
                    print(f"     [{name}] {model}: {str(e)[:60]}", file=sys.stderr)
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
    merged = []
    ok, fail = 0, []
    for i, (slug, (title_zh, category, regions)) in enumerate(SELECTED.items(), 1):
        out = OUT_DIR / f"{slug}.json"
        if out.exists():
            merged.append(json.load(open(out)))
            ok += 1
            print(f"[{i}/{len(SELECTED)}] SKIP {slug} (已存在)", file=sys.stderr)
            continue
        src = SRC / f"{slug}.md"
        if not src.exists():
            fail.append((slug, "中文版未抓取"))
            print(f"[{i}/{len(SELECTED)}] FAIL {slug}: 中文版未抓取", file=sys.stderr)
            continue
        body = src.read_text(encoding="utf-8")
        try:
            raw = call_llm(slug, title_zh, category, regions, body)
            entry = parse_json(raw)
            # Basic validation
            for field in ("id", "condition_zh", "category", "keywords"):
                if not entry.get(field):
                    raise ValueError(f"缺少字段 {field}: {raw[:200]}")
            if not entry.get("citations"):
                raise ValueError("缺少 citations")
            out.write_text(json.dumps(entry, ensure_ascii=False, indent=1), encoding="utf-8")
            merged.append(entry)
            ok += 1
            print(f"[{i}/{len(SELECTED)}] ✅ {slug} ({len(body)} 字符正文)", file=sys.stderr)
        except Exception as e:
            fail.append((slug, str(e)[:100]))
            print(f"[{i}/{len(SELECTED)}] ❌ {slug}: {str(e)[:100]}", file=sys.stderr)
        time.sleep(1)

    if merged:
        # Sort by condition_zh for stable output, write merged file
        merged.sort(key=lambda e: e["condition_zh"])
        MERGED.write_text(json.dumps(merged, ensure_ascii=False, indent=1), encoding="utf-8")
        print(f"\n✅ 合并写出 {MERGED}  ({len(merged)} 条)", file=sys.stderr)
    print(f"完成: {ok}/{len(SELECTED)} 成功; 失败 {len(fail)}: {fail}", file=sys.stderr)


if __name__ == "__main__":
    main()
