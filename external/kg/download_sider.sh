#!/usr/bin/env bash
# Download SIDER drug side effect datasets and TTD parser
# Run from project root: bash external/kg/download_sider.sh

set -euo pipefail

KG_DIR="$(cd "$(dirname "$0")" && pwd)"
echo "Downloading to: $KG_DIR"

echo "[1/3] SIDER indications (zenodo)..."
curl -L -o "$KG_DIR/SIDER_indications.tsv" \
  "https://zenodo.org/records/7877720/files/SIDER_meddra_all_indications.tsv"

echo "[2/3] SIDER side effects (EMBL)..."
curl -L -o "$KG_DIR/SIDER_side_effects.tsv.gz" \
  "http://sideeffects.embl.de/data/SIDER_meddra_all_se.tsv.gz"

echo "[3/3] TTD parser (hint-lab GitHub)..."
curl -L -o "$KG_DIR/parse_ttd.py" \
  "https://raw.githubusercontent.com/hint-lab/chinese-medical-kg/main/scripts/parse_ttd_data.py"

echo ""
echo "Done. File sizes:"
ls -lh "$KG_DIR"/SIDER_indications.tsv "$KG_DIR"/SIDER_side_effects.tsv.gz "$KG_DIR"/parse_ttd.py
