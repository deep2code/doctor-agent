#!/usr/bin/env python3
"""Convert newborn-care sources into internal/knowledge/data/newborn_care.json.

Sources:
  1. WHO "Recommendations for care of the preterm or low-birth-weight infant"
     (2022, ISBN 9789240058262) — 26 recommendation rows from Executive
     Summary Table 1 (pages x-xii), PDF text layer. English recommendation
     text kept verbatim; zh summary via LLM (Zhipu glm-4-flash, free), with
     a hardcoded topic_zh map for stability.
  2. China newborn screening programmes (hand-written entries from public
     official documents): heel-prick metabolic screening, hearing screening,
     CHD pulse-oximetry screening — each with official/gov URL citation.

Idempotent: LLM step caches per-rec into external/newborn/who_cache/.
Run from repo root: python3 external/convert_newborn.py
"""
import json
import re
import sys
import time
import urllib.request
from pathlib import Path

from pypdf import PdfReader

ROOT = Path(__file__).parent.parent
SRC = ROOT / "external" / "newborn"
CACHE = SRC / "who_cache"
OUT = ROOT / "internal" / "knowledge" / "data" / "newborn_care.json"

PDF = SRC / "who_preterm_lbw_2022.pdf"
WHO_URL = "https://www.who.int/publications/i/item/9789240058262"

# exec-summary table pages (0-based: 10,11,12 = printed x,xi,xii)
TABLE_PAGES = (10, 11, 12)

TOPIC_ZH = {
    "A.1a": "袋鼠式护理（常规）", "A.1b": "袋鼠式护理（生后立即）",
    "A.2": "母亲自己的乳汁", "A.3": "捐赠人乳",
    "A.4": "人乳多组分强化", "A.5": "早产儿配方奶",
    "A.6": "早期启动肠内喂养", "A.7": "按计划喂养与回应式喂养",
    "A.8": "喂养量递增速度", "A.9": "纯母乳喂养时长",
    "A.10a": "铁补充", "A.10b": "锌补充", "A.10c": "维生素D补充", "A.10d": "维生素A补充",
    "A.11": "益生菌", "A.12": "润肤剂（涂油按摩）",
    "B.1": "CPAP治疗呼吸窘迫综合征", "B.2": "生后立即CPAP",
    "B.3": "CPAP压力源（气泡CPAP）", "B.4": "咖啡因治疗呼吸暂停",
    "B.5": "咖啡因用于撤机", "B.6": "咖啡因预防呼吸暂停",
    "C.1": "家庭参与日常护理", "C.2": "家庭支持",
    "C.3": "家访", "C.4": "育儿假与保障",
}
DOMAIN_ZH = {
    "A": "预防与促进性护理 (Preventive and promotive care)",
    "B": "并发症护理 (Care for complications)",
    "C": "家庭参与与支持 (Family involvement and support)",
}

ZHIPU_URL = "https://open.bigmodel.cn/api/paas/v4/chat/completions"
ZHIPU_MODEL = "glm-4-flash"


def extract_who() -> list[dict]:
    r = PdfReader(PDF)
    text = "\n".join((r.pages[i].extract_text() or "") for i in TABLE_PAGES)
    # normalise wrapped words: "T able"->"Table", "Meth ylxanthines" stays as-is in topic only
    blocks = re.split(r"(?=^[ABC]\.\d+[a-d]?\s)", text, flags=re.M)
    recs = []
    for b in blocks[1:]:
        m = re.match(r"^([ABC]\.\d+[a-d]?)\s", b)
        if not m:
            continue
        rid = m.group(1)
        body = b[m.end():].strip()
        strength = ""
        sm = re.search(r"\((Strong recommendation[^)]*|Conditional recommendation[^)]*|Good practice statement)\)", body)
        if sm:
            strength = sm.group(1)
        status = "Updated" if re.search(r"\bUpdated\b", body[sm.end():] if sm else body) else \
                 ("New" if re.search(r"\bNew\b", body[sm.end():] if sm else body) else "")
        # Keep the raw block; the LLM pass below strips the topic-label column
        # and produces a clean recommendation_en + zh (PDF table text flow mixes
        # the topic column into the sentence, regexes proved unreliable).
        seg = re.sub(r"\s+", " ", body).strip()
        recs.append({
            "id": rid,
            "domain": DOMAIN_ZH[rid[0]],
            "topic_zh": TOPIC_ZH.get(rid, ""),
            "raw_en": seg,
            "strength_en": strength,
            "status": status,
        })
    return recs


def llm_zh(rec: dict) -> dict:
    key = __import__("os").environ.get("ZHIPU_API_KEY")
    if not key:
        return {"recommendation_en": rec["raw_en"], "recommendation_zh": ""}
    prompt = (
        "下面是世界卫生组织(WHO)早产/低出生体重儿护理指南执行摘要表格中一行的原始文本流。"
        "表格第一列是主题短标签（如 \"Any KMC\"、\"CP AP for respiratory distress syndrome\"），"
        "第二列才是推荐正文，末尾括号内是推荐强度与证据质量，之后可能有 Updated/New 状态词。\n"
        "请输出 JSON（不要 markdown 代码块）：\n"
        '{"recommendation_en": "清洗后的英文推荐正文（去掉主题标签列与 Updated/New 状态词；'
        '保留完整句子与末尾的强度括号说明；修复 PDF 断词，如 CP AP→CPAP、T able→Table）",\n'
        ' "recommendation_zh": "该推荐正文的准确简洁中文翻译（医学术语规范；强度括号一并译出）"}\n\n'
        f"原始文本：{rec['raw_en']}"
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
            with urllib.request.urlopen(req, timeout=60) as resp:
                data = json.loads(resp.read())
                content = data["choices"][0]["message"]["content"].strip()
                content = re.sub(r"^```(?:json)?|```$", "", content, flags=re.M).strip()
                parsed = json.loads(content)
                return {
                    "recommendation_en": parsed.get("recommendation_en", rec["raw_en"]),
                    "recommendation_zh": parsed.get("recommendation_zh", ""),
                }
        except Exception as e:
            if attempt == 2:
                print(f"  LLM fail {rec['id']}: {e}", file=sys.stderr)
                return {"recommendation_en": rec["raw_en"], "recommendation_zh": ""}
            time.sleep(3)
    return {"recommendation_en": rec["raw_en"], "recommendation_zh": ""}


CHINA_SCREENING = [
    {
        "id": "cn-nbs-metabolic",
        "title_zh": "新生儿遗传代谢病筛查（足跟血）",
        "source": "《新生儿疾病筛查管理办法》（原卫生部令第64号）及各地新技术规范",
        "url": "https://www.fxtp.gov.cn/content/2023/797640.html",
        "points": [
            "全国筛查病种：先天性甲状腺功能减低症（CH）、苯丙酮尿症（PKU）等新生儿遗传代谢病；各省可按本地情况增加病种（如先天性肾上腺皮质增生症 CAH、葡萄糖-6-磷酸脱氢酶缺乏症 G6PD 等）",
            "采血时间：出生 72 小时后、吃饱奶（至少 6-8 次）后由助产机构采集足跟血制成干血片；早产儿、提前出院者需追踪采血，最迟不宜超过出生后 20 天",
            "程序：血片采集 → 送检 → 实验室检测 → 阳性病例确诊和治疗",
            "意义：患儿出生时大多外观正常，出生 3-6 个月才逐渐出现症状；早筛查早治疗可避免智力低下、发育落后等不可逆损害",
            "家长须知：筛查遵循知情同意原则；初筛阳性不等于确诊，需按通知尽快复查确诊",
        ],
    },
    {
        "id": "cn-nbs-hearing",
        "title_zh": "新生儿听力筛查",
        "source": "《新生儿疾病筛查管理办法》（原卫生部令第64号）",
        "url": "https://www.fxtp.gov.cn/content/2023/797640.html",
        "points": [
            "初筛：出生后 48 小时至出院前，用耳声发射（OAE）或自动听性脑干反应（AABR）检测",
            "复筛：初筛未通过者需在出生 42 天内复筛",
            "程序：初筛 → 复筛 → 阳性病例确诊和治疗",
            "NICU 住院高危儿建议直接采用 AABR",
            "意义：永久性听力损失患儿在出生 6 个月内开始科学干预和康复训练，有助于语言认知正常发育",
        ],
    },
    {
        "id": "cn-nbs-chd",
        "title_zh": "新生儿先天性心脏病筛查（心脏听诊+经皮脉搏血氧饱和度）",
        "source": "新生儿先天性心脏病筛查项目（国家卫生健康委妇幼司，2018 年起全国推广）",
        "url": "https://wjw.fujian.gov.cn/jggk/csxx/fyjkfwc/gzdt/202408/t20240806_6497653.htm",
        "points": [
            "筛查时间：出生后 6-72 小时（出院前）",
            "筛查方法：心脏杂音听诊 + 经皮脉搏血氧饱和度测定（无创）",
            "筛查阳性者：1 周内转诊至诊断机构做超声心动图明确诊断",
            "确诊患儿转诊至治疗机构治疗并随访",
            "筛查需监护人知情同意",
        ],
    },
]


def main():
    CACHE.mkdir(parents=True, exist_ok=True)
    recs = extract_who()
    print(f"WHO recommendations extracted: {len(recs)}")
    for rec in recs:
        cf = CACHE / f"{rec['id']}.json"
        if cf.exists():
            cached = json.loads(cf.read_text())
            rec["recommendation_en"] = cached.get("recommendation_en", rec["raw_en"])
            rec["recommendation_zh"] = cached.get("recommendation_zh", "")
            continue
        out = llm_zh(rec)
        rec["recommendation_en"] = out["recommendation_en"]
        rec["recommendation_zh"] = out["recommendation_zh"]
        if out["recommendation_zh"]:
            cf.write_text(json.dumps(out, ensure_ascii=False))
        time.sleep(1)
    n_zh = sum(1 for r in recs if r.get("recommendation_zh"))
    print(f"LLM zh translations: {n_zh}/{len(recs)}")

    doc = {
        "version": "1.0.0",
        "updated": "2026-09-02",
        "who_preterm_lbw": {
            "source": "WHO recommendations for care of the preterm or low-birth-weight infant (2022)",
            "url": WHO_URL,
            "pdf": "external/newborn/who_preterm_lbw_2022.pdf",
            "recommendations": recs,
        },
        "china_screening": CHINA_SCREENING,
    }
    for rec in doc["who_preterm_lbw"]["recommendations"]:
        rec.pop("raw_en", None)
    OUT.write_text(json.dumps(doc, ensure_ascii=False))
    print(f"size: {OUT.stat().st_size/1024:.1f} KB -> {OUT}")


if __name__ == "__main__":
    main()
