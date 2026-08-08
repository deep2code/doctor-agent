#!/usr/bin/env python3
"""Download ClinVar pathogenic/likely-pathogenic variants for southern-China
core genes (HBB/HBA1/HBA2/G6PD — thalassemia & G6PD deficiency) via NCBI
E-Utilities, then merge into internal/knowledge/data/clinvar.json.

Output schema:
  {"source": "clinvar", "updated": "...", "variants": [
    {"clinvar_id": "4818705", "gene": "HBB", "variation": "NM_000518.5(HBB):c.55del (p.Lys18_Val19insTer)",
     "cdna": "c.55del", "clinical_significance": "Likely pathogenic", "traits": ["Beta-thalassemia"]}
  ]}
Idempotent: per-gene JSON files in external/clinvar/ are cached.
"""
import json, re, sys, time, urllib.request, urllib.parse
from pathlib import Path

ROOT = Path(__file__).parent.parent
CACHE = Path(__file__).parent / "clinvar"
OUT = ROOT / "internal" / "knowledge" / "data" / "clinvar.json"
EUTILS = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils"
UA = {"User-Agent": "Mozilla/5.0 (research; mailto:test@example.com)"}

GENES = ["HBB", "HBA1", "HBA2", "G6PD"]
BATCH = 100


def get(url):
    req = urllib.request.Request(url, headers=UA)
    for i in range(3):
        try:
            with urllib.request.urlopen(req, timeout=90) as r:
                return r.read().decode("utf-8", "replace")
        except Exception as e:
            if i == 2:
                raise
            time.sleep(4)


def esearch(gene, retmax):
    q = urllib.parse.urlencode({
        "db": "clinvar",
        "term": f'{gene}[gene] AND (pathogenic OR "likely pathogenic")',
        "retmax": retmax, "retmode": "json"})
    return json.loads(get(f"{EUTILS}/esearch.fcgi?{q}"))


def esummary(ids):
    q = urllib.parse.urlencode({"db": "clinvar", "id": ",".join(ids), "retmode": "json"})
    for i in range(4):
        try:
            d = json.loads(get(f"{EUTILS}/esummary.fcgi?{q}"))
            if "result" in d and d.get("result"):
                return d
        except Exception:
            pass
        time.sleep(4 * (i + 1))
    return {"result": {}}


def extract_traits(classification):
    """Pull trait names out of germline_classification.trait_set."""
    out = []
    for t in (classification or {}).get("trait_set", []):
        name = t.get("trait_name") or ""
        if not name and t.get("trait_xrefs"):
            for x in t["trait_xrefs"]:
                if "clinvar" in (x.get("db_source") or "") or x.get("db_name"):
                    name = x.get("dbtag") or x.get("db_tag") or ""
                    if name:
                        break
        if name and name not in out:
            out.append(name)
    return out


def fetch_gene(gene):
    cache = CACHE / f"{gene}.json"
    if cache.exists():
        return json.load(open(cache))
    total = int(esearch(gene, 0)["esearchresult"]["count"])
    print(f"{gene}: {total} 条", file=sys.stderr)
    ids = esearch(gene, total)["esearchresult"]["idlist"]
    variants = []
    for i in range(0, len(ids), BATCH):
        chunk = ids[i:i + BATCH]
        d = esummary(chunk)
        got = 0
        for cid in chunk:
            r = d.get("result", {}).get(cid, {})
            if not r:
                continue
            got += 1
            cls = r.get("germline_classification") or {}
            genes = [g.get("symbol") for g in r.get("genes", []) if g.get("symbol")]
            vs = r.get("variation_set") or [{}]
            variants.append({
                "clinvar_id": cid,
                "gene": gene,
                "variation": r.get("title", ""),
                "cdna": vs[0].get("cdna_change", ""),
                "clinical_significance": cls.get("description", ""),
                "traits": extract_traits(cls),
            })
        print(f"  {gene} 批 {i+len(chunk)}/{len(ids)} (返回 {got}/{len(chunk)})", file=sys.stderr)
        time.sleep(1.5)
    CACHE.mkdir(parents=True, exist_ok=True)
    cache.write_text(json.dumps(variants, ensure_ascii=False, indent=1), encoding="utf-8")
    return variants


def main():
    all_variants = []
    for gene in GENES:
        vs = fetch_gene(gene)
        all_variants.extend(vs)
        print(f"{gene}: {len(vs)} 条已缓存", file=sys.stderr)
    # 去重(clinvar_id 跨基因罕见重复)
    seen, uniq = set(), []
    for v in all_variants:
        if v["clinvar_id"] not in seen:
            seen.add(v["clinvar_id"])
            uniq.append(v)
    doc = {"source": "clinvar", "updated": "2026-08-08", "variants": uniq}
    OUT.write_text(json.dumps(doc, ensure_ascii=False, indent=1), encoding="utf-8")
    print(f"✅ {OUT}: {len(uniq)} 条变异 ({OUT.stat().st_size/1024:.0f} KB)", file=sys.stderr)


if __name__ == "__main__":
    main()
