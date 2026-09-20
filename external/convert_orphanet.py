#!/usr/bin/env python3
"""Merge Orphanet rare-disease data into data/orphanet_diseases.json.

Inputs (external/orphanet/, from orphadata.com CC-BY-4.0):
  en_product1.xml   2026 disorders: EN name, synonyms, ICD-10 + ICD-11 refs
  zh_product1.json  2021-12 translated product1: zh name + synonyms + ICD-10

Join key is OrphaCode. zh-only rows are kept (English falls back to the
2021 record's latin-script label if present). Output schema:

  {source, updated, diseases: [{orpha_code, name_zh, name_en, synonyms[],
    icd10[], icd11[], type}]}

Registered in Go as exact_lookup type=orphanet (structured lookup table).
"""

import json
import sys
import xml.etree.ElementTree as ET
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DIR = ROOT / "external" / "orphanet"
OUT = ROOT / "internal" / "knowledge" / "data" / "orphanet_diseases.json"


def labels(node, lang):
    out = []
    for n in node.iter("Name"):
        if n.get("lang") == lang and (n.text or "").strip():
            out.append(n.text.strip())
    return out


def parse_en():
    path = DIR / "en_product1.xml"
    if not path.exists():
        return {}
    try:
        root = ET.parse(path).getroot()
    except ET.ParseError:
        print("en_product1.xml incomplete — skipping English merge", file=sys.stderr)
        return {}
    disorders = {}
    for d in root.iter("Disorder"):
        code = (d.findtext("OrphaCode") or "").strip()
        if not code:
            continue
        name_en = ""
        nm = d.find("Name")
        if nm is not None and (nm.text or "").strip():
            name_en = nm.text.strip()
        syns = [
            (s.text or "").strip()
            for s in d.iter("Synonym")
            if (s.text or "").strip()
        ]
        icd10, icd11 = [], []
        for er in d.iter("ExternalReference"):
            src = (er.findtext("Source") or "").strip()
            ref = (er.findtext("Reference") or "").strip()
            if not ref:
                continue
            if src == "ICD-10" and ref not in icd10:
                icd10.append(ref)
            elif src == "ICD-11" and ref not in icd11:
                icd11.append(ref)
        dtype = ""
        dt = d.find("DisorderType")
        if dt is not None:
            tn = dt.find("Name")
            if tn is not None and (tn.text or "").strip():
                dtype = tn.text.strip()
        disorders[code] = {
            "name_en": name_en,
            "synonyms_en": syns,
            "icd10": icd10,
            "icd11": icd11,
            "type": dtype,
        }
    return disorders


def parse_zh():
    path = DIR / "zh_product1.json"
    data = json.loads(path.read_text(encoding="utf-8"))
    disorders = {}
    for d in data["JDBOR"][0]["DisorderList"][0]["Disorder"]:
        code = str(d.get("OrphaCode", "")).strip()
        if not code:
            continue
        names = [n["label"] for n in d.get("Name", []) if n.get("lang") == "zh"]
        syns = []
        for sl in d.get("SynonymList", []) or []:
            inner = sl.get("Synonym", []) if isinstance(sl, dict) else []
            for s in inner:
                if s.get("lang") == "zh" and s.get("label"):
                    syns.append(s["label"])
        icd10 = []
        for erl in d.get("ExternalReferenceList", []) or []:
            inner = erl.get("ExternalReference", []) if isinstance(erl, dict) else []
            for er in inner:
                if er.get("Source") == "ICD-10" and er.get("Reference"):
                    r = er["Reference"].strip()
                    if r and r not in icd10:
                        icd10.append(r)
        dtype = ""
        dt = d.get("DisorderType")
        if isinstance(dt, list) and dt:
            for n in dt[0].get("Name", []):
                if n.get("lang") == "zh":
                    dtype = n["label"]
        disorders[code] = {
            "name_zh": names[0] if names else "",
            "synonyms_zh": syns,
            "icd10": icd10,
        }
    return disorders


def main():
    en = parse_en()
    zh = parse_zh()
    codes = sorted(set(en) | set(zh), key=lambda c: int(c) if c.isdigit() else 0)
    diseases = []
    for code in codes:
        e = en.get(code, {})
        z = zh.get(code, {})
        syns = list(dict.fromkeys(
            [s for s in z.get("synonyms_zh", []) if s]
            + [s for s in e.get("synonyms_en", []) if s]
        ))
        row = {
            "orpha_code": code,
            "name_zh": z.get("name_zh", ""),
            "name_en": e.get("name_en", ""),
            "synonyms": syns[:12],
            "icd10": list(dict.fromkeys(z.get("icd10", []) + e.get("icd10", [])))[:6],
            "icd11": e.get("icd11", [])[:6],
            "type": {"Disease": "疾病", "Pathological anomaly": "病理异常",
                     "Sign or symptom": "体征或症状", "Etiology": "病因",
                     "Disorder": "疾病", "Syndrome": "综合征"}.get(
                         e.get("type", ""), e.get("type", "") or ""),
        }
        if not (row["name_zh"] or row["name_en"]):
            continue
        if not row["type"] and code in zh:
            row["type"] = "疾病"
        diseases.append(row)

    out = {
        "source": "Orphanet 罕见病目录 (orphadata.com product1, CC-BY-4.0)",
        "updated": "2026-06" if en else "2021-12",
        "diseases": diseases,
    }
    OUT.write_text(
        json.dumps(out, ensure_ascii=False, separators=(",", ":")), encoding="utf-8"
    )
    zh_n = sum(1 for x in diseases if x["name_zh"])
    i11 = sum(1 for x in diseases if x["icd11"])
    print(f"wrote {OUT}: {len(diseases)} diseases, {zh_n} zh names, {i11} with ICD-11")


if __name__ == "__main__":
    main()
