#!/usr/bin/env python3
"""Convert Chinese medical KG Excel files to JSON for the knowledge base."""

import json
import pandas as pd
import os

DATA_DIR = os.path.join(os.path.dirname(__file__), "kg/chinese-medical-kg/data")
OUT_DIR = os.path.join(os.path.dirname(__file__), "..", "internal/knowledge/data")

# ── ICD10 diseases ──
print("=== ICD10 ===")
df_icd = pd.read_excel(os.path.join(DATA_DIR, "icd10.xlsx"))
print(f"Columns: {list(df_icd.columns)}, shape: {df_icd.shape}")
print(df_icd.head(3).to_string())
print()

diseases = []
for _, row in df_icd.iterrows():
    diseases.append({
        "icd10_code": str(row["疾病诊断编码"]).strip(),
        "name_zh": str(row["疾病诊断名称"]).strip(),
        "category": "icd10",
    })

with open(os.path.join(OUT_DIR, "icd10_diseases.json"), "w", encoding="utf-8") as f:
    json.dump({"diseases": diseases}, f, ensure_ascii=False, indent=2)
print(f"icd10_diseases.json: {len(diseases)} diseases\n")

# ── NMPA drugs ──
all_drugs = []

# Domestic: header row is row index 0 (the label row), actual headers in row 1
df_dom = pd.read_excel(os.path.join(DATA_DIR, "drugs_domestic.xlsx"), header=1)
print("=== drugs_domestic ===")
print(f"Columns: {list(df_dom.columns)}, shape: {df_dom.shape}")
print(df_dom.head(3).to_string())
print()

for _, row in df_dom.iterrows():
    code = str(row.get("药品编码", "")).strip()
    name = str(row.get("产品名称", "")).strip()
    if not code or code == "nan" or not name or name == "nan":
        continue
    all_drugs.append({
        "drug_code": code,
        "name_zh": name,
        "source": "nmpa_domestic",
    })

# Imported: same header trick
df_imp = pd.read_excel(os.path.join(DATA_DIR, "drugs_imported.xlsx"), header=1)
print("=== drugs_imported ===")
print(f"Columns: {list(df_imp.columns)}, shape: {df_imp.shape}")
print(df_imp.head(3).to_string())
print()

for _, row in df_imp.iterrows():
    code = str(row.get("药品编码", "")).strip()
    name = str(row.get("产品名称", "")).strip()
    if not code or code == "nan" or not name or name == "nan":
        continue
    all_drugs.append({
        "drug_code": code,
        "name_zh": name,
        "source": "nmpa_imported",
    })

with open(os.path.join(OUT_DIR, "nmpa_drugs.json"), "w", encoding="utf-8") as f:
    json.dump({"drugs": all_drugs}, f, ensure_ascii=False, indent=2)

dom_count = sum(1 for d in all_drugs if d["source"] == "nmpa_domestic")
imp_count = sum(1 for d in all_drugs if d["source"] == "nmpa_imported")
print(f"nmpa_drugs.json: {len(all_drugs)} drugs (domestic: {dom_count}, imported: {imp_count})")
print(f"\n=== Final Counts ===")
print(f"Diseases: {len(diseases)}")
print(f"Drugs: {len(all_drugs)}")
