#!/usr/bin/env python3
"""Batch-fetch PubMed abstracts from Europe PMC for China-focused medical topics.
Each topic becomes external/europepmc/{slug}.json with fields:
id, pmid, doi, title, journal, year, authors, abstract, topic.
"""
import json, sys, time, urllib.parse, urllib.request
from pathlib import Path

BASE = "https://www.ebi.ac.uk/europepmc/webservices/rest/search"
OUT = Path(__file__).parent / "europepmc"
PER_TOPIC = int(sys.argv[1]) if len(sys.argv) > 1 else 300

TOPICS = {
    "thalassemia_china": 'thalassemia AND (China OR Chinese) AND (screening OR carrier OR prevalence)',
    "g6pd_deficiency": '("G6PD deficiency" OR "glucose-6-phosphate dehydrogenase deficiency")',
    "dengue_south_china": '(dengue) AND (Guangdong OR Guangzhou OR "south China")',
    "hepatitis_b_china": '("hepatitis B") AND (China) AND (treatment OR screening OR mother-to-child)',
    "nasopharyngeal_carcinoma": '("nasopharyngeal carcinoma") AND (EBV OR "Epstein-Barr") AND (screening OR Guangdong OR Cantonese)',
    "lactose_intolerance": '("lactose intolerance") AND (Chinese OR Asia OR lactase)',
    "aldh2_ethanol": '("ALDH2" OR "aldehyde dehydrogenase 2") AND (alcohol OR ethanol OR cancer)',
    "tinea_humid": '("tinea pedis" OR "dermatophytosis" OR "fungal skin infection") AND (humid OR tropical OR hot)',
    "favism": '(favism OR "fava bean" OR "broad bean") AND (hemolysis OR "G6PD")',
    "childhood_fever": '("febrile child" OR "childhood fever" OR "fever in children") AND (management OR guideline)',
    "dog_bite_rabies": '(rabies OR "dog bite" OR "animal bite") AND (prophylaxis OR "post-exposure" OR wound)',
    "heat_stroke": '("heat stroke" OR heatstroke OR "heat-related illness") AND (management OR prevention)',
    "oral_rehydration": '("oral rehydration" OR "oral rehydration solution" OR "diarrhea" OR diarrhoea) AND (children OR dehydration)',
    "preconception_care": '("preconception" OR "pre-pregnancy" OR "premarital") AND (screening OR counseling OR genetic)',
    "dengue_vaccine": '(dengue) AND (vaccine OR vaccination)',
    "hbv_vaccine": '("hepatitis B vaccine") AND (infant OR newborn OR China)',
}

def fetch(query: str, page: int) -> dict:
    params = urllib.parse.urlencode({
        "query": query, "format": "json", "pageSize": 100, "page": page,
        "resultType": "core",
    })
    url = f"{BASE}?{params}"
    for attempt in range(3):
        try:
            with urllib.request.urlopen(url, timeout=30) as r:
                return json.load(r)
        except Exception as e:
            print(f"  retry {attempt+1}: {e}", file=sys.stderr)
            time.sleep(2)
    return {}

def main():
    OUT.mkdir(parents=True, exist_ok=True)
    for slug, query in TOPICS.items():
        records, page, hits = [], 1, 0
        while page * 100 <= PER_TOPIC + 100:
            data = fetch(query, page)
            results = data.get("resultList", {}).get("result", [])
            if not results:
                break
            hits = data.get("hitCount", 0)
            for r in results:
                records.append({
                    "id": r.get("id"), "pmid": r.get("pmid", ""),
                    "doi": r.get("doi", ""), "title": r.get("title", ""),
                    "journal": r.get("journalInfo", {}).get("journal", {}).get("title", ""),
                    "year": (r.get("pubYear") or ""),
                    "authors": [a.get("fullName", "") for a in r.get("authorList", {}).get("author", [])][:10],
                    "abstract": (r.get("abstractText") or ""),
                    "topic": slug,
                })
            print(f"  {slug}: page {page}, got {len(results)} (hitCount={hits})", file=sys.stderr)
            page += 1
            if len(records) >= PER_TOPIC:
                break
            time.sleep(0.3)
        out = OUT / f"{slug}.json"
        with open(out, "w") as f:
            json.dump(records, f, ensure_ascii=False, indent=1)
        print(f"✔ {slug}: {len(records)} records -> {out}", file=sys.stderr)
    print("DONE")

if __name__ == "__main__":
    main()
