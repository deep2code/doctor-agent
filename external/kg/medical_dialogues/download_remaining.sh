#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"

# Actual filenames from the repo (with row ranges)
declare -A URLS=(
  ["Oncology_肿瘤科.csv"]="https://raw.githubusercontent.com/Toyhom/Chinese-medical-dialogue-data/master/Data_%E6%95%B0%E6%8D%AE/Oncology_%E8%82%BF%E7%98%A4%E7%A7%91/%E8%82%BF%E7%98%A4%E7%A7%915-10000.csv"
  ["Surgical_外科.csv"]="https://raw.githubusercontent.com/Toyhom/Chinese-medical-dialogue-data/master/Data_%E6%95%B0%E6%8D%AE/Surgical_%E5%A4%96%E7%A7%91/%E5%A4%96%E7%A7%915-14000.csv"
)

for name in "${!URLS[@]}"; do
  if [ -f "$name" ] && [ $(stat -f%z "$name") -gt 1000000 ]; then
    echo "SKIP $name ($(stat -f%z "$name") bytes)"
    continue
  fi
  echo "Downloading $name ..."
  curl -L --retry 3 --retry-delay 5 -o "$name" "${URLS[$name]}"
  echo "  $name: $(stat -f%z "$name") bytes ($(echo "scale=1; $(stat -f%z "$name") / 1048576" | bc) MB)"
done

echo ""
echo "All files:"
ls -lh *.csv
