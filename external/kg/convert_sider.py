#!/usr/bin/env python3
"""
Convert SIDER drug side effects data to JSON format for doctor-agent.
"""

import json
import gzip
from collections import defaultdict
from pathlib import Path

SIDER_DIR = Path(__file__).parent / "SIDER"
OUTPUT_DIR = Path(__file__).parent.parent.parent / "internal" / "knowledge" / "data"

def parse_drug_names():
    """Parse drug names from indications file."""
    drug_names = {}
    indications_file = SIDER_DIR / "SIDER_meddra_all_indications.tsv"
    
    print(f"Parsing drug names from {indications_file}...")
    with open(indications_file, 'r', encoding='utf-8') as f:
        for line in f:
            parts = line.strip().split('\t')
            if len(parts) >= 4:
                drug_id = parts[0]
                drug_name = parts[3]  # Use indication name as proxy
                if drug_id not in drug_names:
                    drug_names[drug_id] = set()
                # Don't use indication names as drug names
    
    return drug_names

def parse_side_effects():
    """Parse side effects from SE file."""
    se_file = SIDER_DIR / "SIDER_meddra_all_se.tsv.gz"
    drug_se = defaultdict(set)  # drug_id -> set of side effects
    
    print(f"Parsing side effects from {se_file}...")
    with gzip.open(se_file, 'rt', encoding='utf-8') as f:
        for line in f:
            parts = line.strip().split('\t')
            if len(parts) >= 6:
                drug_id = parts[0]
                se_name = parts[5]  # Side effect name
                drug_se[drug_id].add(se_name)
    
    return drug_se

def parse_indications():
    """Parse indications from indications file."""
    ind_file = SIDER_DIR / "SIDER_meddra_all_indications.tsv"
    drug_ind = defaultdict(set)  # drug_id -> set of indications
    
    print(f"Parsing indications from {ind_file}...")
    with open(ind_file, 'r', encoding='utf-8') as f:
        for line in f:
            parts = line.strip().split('\t')
            if len(parts) >= 4:
                drug_id = parts[0]
                ind_name = parts[3]  # Indication name
                drug_ind[drug_id].add(ind_name)
    
    return drug_ind

def convert_to_json():
    """Convert SIDER data to JSON format."""
    drug_se = parse_side_effects()
    drug_ind = parse_indications()
    
    # Combine data
    all_drugs = set(drug_se.keys()) | set(drug_ind.keys())
    
    print(f"\nFound {len(all_drugs)} drugs with side effects or indications")
    print(f"Drug-SE pairs: {sum(len(v) for v in drug_se.values())}")
    print(f"Drug-Indication pairs: {sum(len(v) for v in drug_ind.values())}")
    
    # Create JSON structure
    sider_data = {
        "source": "SIDER 4.1",
        "description": "Drug side effects and indications from SIDER database",
        "drug_count": len(all_drugs),
        "drugs": []
    }
    
    for drug_id in sorted(all_drugs)[:100]:  # Limit to 100 drugs for demo
        drug_entry = {
            "id": drug_id,
            "side_effects": sorted(list(drug_se.get(drug_id, set())))[:20],  # Top 20 SE
            "indications": sorted(list(drug_ind.get(drug_id, set())))[:10]  # Top 10 indications
        }
        sider_data["drugs"].append(drug_entry)
    
    # Save to file
    output_file = OUTPUT_DIR / "sider_drugs.json"
    with open(output_file, 'w', encoding='utf-8') as f:
        json.dump(sider_data, f, ensure_ascii=False, indent=2)
    
    print(f"\nSaved to {output_file}")
    print(f"File size: {output_file.stat().st_size / 1024:.1f} KB")
    
    return sider_data

if __name__ == "__main__":
    convert_to_json()
