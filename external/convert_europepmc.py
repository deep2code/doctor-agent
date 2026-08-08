#!/usr/bin/env python3
"""Convert external/europepmc/*.json (4800 abstracts, 16 topics) into the
project's embedded literature file internal/knowledge/data/literature.json.

Idempotent: regenerates the file from scratch each run. Filtering keeps only
articles with a DOI or PMID and a substantial abstract (>= 200 chars) so the
keyword retriever returns citable, useful entries.
"""
import json, re, sys
from datetime import date
from pathlib import Path

SRC = Path(__file__).parent / "europepmc"
OUT = Path(__file__).parent.parent / "internal" / "knowledge" / "data" / "literature.json"

MIN_ABSTRACT = 200

TAG_RE = re.compile(r"<[^>]+>")

# topic id -> (中文名, 检索关键词 [中文+英文同义词])
TOPICS = {
    "thalassemia_china":    ("地中海贫血", ["地中海贫血", "地贫", "thalassemia", "thalassaemia", "beta thalassemia", "alpha thalassemia"]),
    "g6pd_deficiency":      ("G6PD缺乏症", ["G6PD缺乏症", "葡萄糖-6-磷酸脱氢酶", "g6pd deficiency", "glucose-6-phosphate dehydrogenase", "glucose 6 phosphate dehydrogenase"]),
    "nasopharyngeal_carcinoma": ("鼻咽癌", ["鼻咽癌", "nasopharyngeal carcinoma", "npc", "ebv", "eb病毒"]),
    "hepatitis_b_china":    ("乙型肝炎", ["乙肝", "乙型肝炎", "hepatitis b", "hbv", "乙型病毒性肝炎"]),
    "lactose_intolerance":  ("乳糖不耐受", ["乳糖不耐受", "lactose intolerance", "lactase deficiency", "乳糖", "牛奶", "喝牛奶", "拉肚子", "奶制品"]),
    "aldh2_ethanol":        ("ALDH2与酒精代谢", ["aldh2", "乙醛脱氢酶2", "酒精代谢", "alcohol", "acetaldehyde", "脸红", "喝酒"]),
    "dengue_south_china":   ("登革热", ["登革热", "dengue", "登革病毒"]),
    "dengue_vaccine":       ("登革热疫苗", ["登革热疫苗", "dengue vaccine", "登革热"]),
    "favism":               ("蚕豆病", ["蚕豆病", "favism", "蚕豆"]),
    "dog_bite_rabies":      ("狂犬病与犬伤", ["狂犬病", "rabies", "狗咬伤", "dog bite", "犬伤"]),
    "hbv_vaccine":          ("乙肝疫苗", ["乙肝疫苗", "hepatitis b vaccine", "hbv vaccine", "乙肝"]),
    "heat_stroke":          ("中暑", ["中暑", "heat stroke", "heatstroke", "热射病", "头晕", "发烧"]),
    "childhood_fever":      ("儿童发热", ["儿童发热", "fever", "发热", "childhood fever", "发烧"]),
    "oral_rehydration":     ("口服补液", ["口服补液", "oral rehydration", "ors", "腹泻", "脱水", "拉肚子"]),
    "preconception_care":   ("孕前保健", ["孕前保健", "preconception", "备孕", "孕前"]),
    "tinea_humid":          ("真菌感染与体癣", ["体癣", "真菌感染", "tinea", "fungal", "潮湿"]),
}


def main():
    topics_out, articles_out, dropped = [], [], 0
    for topic_id, (zh, keywords) in sorted(TOPICS.items()):
        src = SRC / f"{topic_id}.json"
        if not src.exists():
            print(f"⚠️ 缺少 {src}", file=sys.stderr)
            continue
        data = json.load(open(src))
        kept = []
        for i, a in enumerate(data):
            abstract = TAG_RE.sub(" ", (a.get("abstract") or "")).strip()
            abstract = re.sub(r"\s+", " ", abstract)
            if not (a.get("doi") or a.get("pmid")):
                dropped += 1
                continue
            if len(abstract) < MIN_ABSTRACT:
                dropped += 1
                continue
            year = a.get("year")
            kept.append({
                "id": f"{topic_id}-{i:04d}",
                "topic": topic_id,
                "title": a.get("title", "").strip(),
                "abstract": abstract,
                "journal": a.get("journal", "").strip(),
                "year": int(year) if str(year).isdigit() else 0,
                "doi": a.get("doi", ""),
                "pmid": a.get("pmid", ""),
            })
        topics_out.append({"id": topic_id, "name_zh": zh, "keywords": keywords, "count": len(kept)})
        articles_out.extend(kept)
        print(f"{topic_id}: {len(data)} -> {len(kept)}", file=sys.stderr)

    doc = {
        "source": "europepmc",
        "updated": date.today().isoformat(),
        "topics": topics_out,
        "articles": articles_out,
    }
    OUT.write_text(json.dumps(doc, ensure_ascii=False, indent=1), encoding="utf-8")
    size_mb = OUT.stat().st_size / 1024 / 1024
    print(f"\n✅ 写出 {OUT}  ({len(articles_out)} 篇文献, 丢弃 {dropped} 条, {size_mb:.1f} MB)", file=sys.stderr)


if __name__ == "__main__":
    main()
