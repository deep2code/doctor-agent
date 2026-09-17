#!/usr/bin/env python3
"""Fetch FDA drug labels for the WHO EML 24th-list INN names.

Pipeline per medicine (no LLM involved — the raw prescribing sections are
downloaded; a later structurize step translates/summarizes them into the KB):

  1. RxNorm (NLM): resolve the ingredient name to RXCUI(s).
  2. OpenFDA drug/label: search `openfda.rxcui:"<rxcui>"` -> latest label.
     Fallback: `openfda.generic_name:"<name>"`.
  3. Extract the prescribing sections + openfda metadata ->
     external/dailymed/<slug>.json

Idempotent: skips existing files. Respects OpenFDA rate limits (240 req/min).
Run from repo root:
  python3 external/fetch_dailymed.py                 # all WHO EML names
  python3 external/fetch_dailymed.py --limit 20      # first 20 (probe)
  python3 external/fetch_dailymed.py --names amoxicillin metformin
"""
import argparse
import json
import re
import sys
import time
import urllib.parse
import urllib.request
from pathlib import Path

EML_JSON = Path(__file__).parent.parent / "internal" / "knowledge" / "data" / "who_eml.json"
OUT = Path(__file__).parent / "dailymed"
INDEX = OUT / "index.json"
UA = {"User-Agent": "Mozilla/5.0 (research; contact: self)"}
SLEEP_OPENFDA = 0.35   # ~170 req/min, safely under 240
SLEEP_RXNORM = 0.12    # RxNorm allows 20 req/s

# Prescribing sections worth keeping (OpenFDA drug/label field names).
SECTIONS = [
    "boxed_warning", "indications_and_usage", "dosage_and_administration",
    "contraindications", "warnings_and_cautions", "drug_interactions",
    "adverse_reactions", "use_in_specific_populations", "description",
    "overdosage", "highlights_prescribing_information",
    "spl_product_data_elements",
]

INN_STRIP = re.compile(r"\((?:acetaminophen|adrenaline)\)|–|\u2013")


def slugify(inn: str) -> str:
    s = INN_STRIP.sub("", inn).strip()
    s = re.sub(r"\+", "plus", s)
    s = re.sub(r"[^a-z0-9]+", "_", s.lower()).strip("_")
    return s[:80] or "unnamed"


def get(url: str, timeout: int = 30) -> dict:
    req = urllib.request.Request(url, headers=UA)
    for i in range(3):
        try:
            with urllib.request.urlopen(req, timeout=timeout) as r:
                return json.loads(r.read().decode("utf-8", "replace"))
        except Exception as e:
            if i == 2:
                raise
            time.sleep(1.5 * (i + 1))


def rxnorm_rxcui(name: str):
    """Return the best RXCUI for an ingredient name, or None."""
    url = "https://rxnav.nlm.nih.gov/REST/rxcui.json?" + urllib.parse.urlencode(
        {"name": name, "search": "2"}
    )
    try:
        d = get(url)
        ids = d.get("idGroup", {}).get("rxnormId", [])
        return ids[0] if ids else None
    except Exception:
        return None


def score_generic(generic: str, want: str) -> float:
    """How well an FDA generic name matches the INN we searched for."""
    g, w = generic.lower(), want.lower()
    if g == w:
        return 100
    if g.startswith(w) or w.startswith(g):
        return 50
    if w in g:
        return 30  # contains — may be a combination product
    return 0


def openfda_label(generic_candidates: list[str]):
    """Return the best drug label result for the first generic-name candidate
    that matches (OpenFDA label records do not populate openfda.rxcui, and
    generic_name search is substring-based, so we fetch several results and
    score them to prefer the plain single-ingredient label)."""
    for g in generic_candidates:
        url = "https://api.fda.gov/drug/label.json?" + urllib.parse.urlencode(
            {"search": f'openfda.generic_name:"{g}"', "limit": 5}
        )
        try:
            d = get(url)
            res = d.get("results", [])
            if not res:
                continue
            best = max(
                res,
                key=lambda r: score_generic(
                    r.get("openfda", {}).get("generic_name", [""])[0], g
                ),
            )
            return best
        except Exception:
            continue
    return None


def name_variants(inn: str) -> list[str]:
    """Candidate names to try against RxNorm, most specific first."""
    v = []
    core = INN_STRIP.sub("", inn).strip()
    v.append(inn)
    if core != inn:
        v.append(core)
    if " + " in core:
        v.append(core.replace(" + ", " and "))
        # multi-ingredient: try the first ingredient alone as a fallback
        v.append(core.split(" + ")[0])
    return list(dict.fromkeys(v))


# WHO INN -> FDA (USAN) generic-name aliases where they differ.
INN_TO_FDA = {
    "acetylsalicylic acid": "aspirin",
    "paracetamol (acetaminophen)": "acetaminophen",
    "paracetamol": "acetaminophen",
    "epinephrine (adrenaline)": "epinephrine",
    "adrenaline": "epinephrine",
    "sulfamethoxazole + trimethoprim": "sulfamethoxazole and trimethoprim",
    "amoxicillin + clavulanic acid": "amoxicillin and clavulanate",
    "artesunate – sulfadoxine + pyrimethamine": "artesunate",
    "amodiaquine – sulfadoxine + pyrimethamine": "amodiaquine",
    "levodopa + carbidopa": "carbidopa and levodopa",
    "lidocaine + epinephrine (adrenaline)": "lidocaine and epinephrine",
    "benzylpenicillin": "penicillin g",
    "phenobarbital": "phenobarbital",
    "glyceryl trinitrate": "nitroglycerin",
    "ergocalciferol": "ergocalciferol",
    "phytomenadione": "phytonadione",
    "sodium lactate": "sodium lactate",
    "valproic acid (sodium valproate)": "valproic acid",
    "zinc sulfate": "zinc sulfate",
}


def generic_variants(inn: str) -> list[str]:
    """FDA generic-name candidates for an INN, most likely first."""
    v = []
    if inn in INN_TO_FDA:
        v.append(INN_TO_FDA[inn])
    core = INN_STRIP.sub("", inn).strip()
    if core != inn:
        v.append(core)
    if core in INN_TO_FDA:
        v.append(INN_TO_FDA[core])
    v.append(inn)
    return list(dict.fromkeys(x for x in v if x))


def extract_sections(label: dict) -> dict:
    out = {}
    for f in SECTIONS:
        val = label.get(f)
        if isinstance(val, list) and val:
            out[f] = val[0]
        elif isinstance(val, str) and val:
            out[f] = val
    return out


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--limit", type=int, default=0, help="only first N EML names")
    parser.add_argument("--names", nargs="*", help="specific INN names to fetch")
    parser.add_argument("--sleep", type=float, default=0, help="extra sleep per item")
    args = parser.parse_args()

    eml = json.loads(EML_JSON.read_text(encoding="utf-8"))
    names = [e["name"] for e in eml["entries"]]
    if args.names:
        names = args.names
    elif args.limit:
        names = names[: args.limit]
    print(f"计划抓取 {len(names)} 个药物标签", file=sys.stderr)

    OUT.mkdir(parents=True, exist_ok=True)
    index = {"updated": time.strftime("%Y-%m-%d"), "total": 0, "ok": 0,
             "no_label": [], "failed": [], "labels": []}

    for i, inn in enumerate(names, 1):
        slug = slugify(inn)
        out = OUT / f"{slug}.json"
        if out.exists():
            index["total"] += 1
            index["ok"] += 1
            index["labels"].append(slug)
            continue
        time.sleep(args.sleep)

        rxcui = None
        for cand in name_variants(inn):
            rxcui = rxnorm_rxcui(cand)
            if rxcui:
                break
            time.sleep(SLEEP_RXNORM)

        label = openfda_label(generic_variants(inn))
        time.sleep(SLEEP_OPENFDA)

        if not label:
            index["no_label"].append(inn)
            print(f"[{i}/{len(names)}] NO_LABEL {inn} (rxcui={rxcui})", file=sys.stderr)
            continue

        of = label.get("openfda", {})
        setID = (of.get("set_id") or [""])[0] or label.get("id", "")
        if setID:
            url = f"https://dailymed.nlm.nih.gov/dailymed/drugInfo.cfm?setid={setID}"
        else:
            generic = of.get("generic_name", [""])[0]
            url = (f"https://dailymed.nlm.nih.gov/dailymed/search.cfm?labeltype=all"
                   f"&query={urllib.parse.quote(generic)}")
        rec = {
            "inn": inn,
            "rxcui": rxcui or "",
            "generic_name": generic,
            "brand_names": of.get("brand_name", []),
            "set_id": setID,
            "sections": extract_sections(label),
            "url": url,
        }
        out.write_text(json.dumps(rec, ensure_ascii=False, indent=1), encoding="utf-8")
        sec_count = len(rec["sections"])
        index["total"] += 1
        index["ok"] += 1
        index["labels"].append(slug)
        print(f"[{i}/{len(names)}] OK {inn} -> {slug}.json ({sec_count} sections)",
              file=sys.stderr)

    INDEX.write_text(json.dumps(index, ensure_ascii=False, indent=1), encoding="utf-8")
    print(f"DONE: total={index['total']} ok={index['ok']} no_label={len(index['no_label'])} "
          f"failed={len(index['failed'])}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
