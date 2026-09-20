#!/usr/bin/env python3
"""Convert external/kg/medical_dialogues/*.csv (源芯医患对话语料) into
internal/knowledge/data/medical_qa_pairs.json (dataset "medical_qa").

The shipped CSVs are GBK-encoded; the previous medical_qa_pairs.json was
produced with the wrong encoding — 100% of its sampled rows were mojibake
(0/2000 contained valid CJK). This script rebuilds it from the source with
correct gb18030 decoding, row-level filtering and exact-question dedupe.

Row filters (documented so re-runs stay comparable):
  - drop rows with empty title/question/answer or a replacement char (U+FFFD)
  - drop question < 8 chars or answer < 15 chars (too short to be useful)
  - dedupe on exact question text (keep the first, longest answer wins)

After running: python3 external/make_gz.py && go run . seed-knowledge
"""

import csv
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SRC_DIR = ROOT / "external" / "kg" / "medical_dialogues"
OUT = ROOT / "internal" / "knowledge" / "data" / "medical_qa_pairs.json"

FILES = [
    "Andriatria_男科.csv",
    "IM_内科.csv",
    "OAGD_妇产科.csv",
    "Oncology_肿瘤科.csv",
    "Pediatric_儿科.csv",
    "Surgical_外科.csv",
]

csv.field_size_limit(10**9)


def load_file(name: str):
    path = SRC_DIR / name
    # department column is authoritative per-row; filename label is the
    # top-level fallback when the cell is empty/garbled.
    label = name.split("_", 1)[1].removesuffix(".csv")
    with open(path, encoding="gb18030", errors="replace", newline="") as fh:
        for row in csv.DictReader(fh):
            dept = (row.get("department") or "").strip() or label
            yield dept, row


def main() -> None:
    pairs = {}
    skipped_short = skipped_bad = 0
    total_rows = 0
    for name in FILES:
        for dept, row in load_file(name):
            total_rows += 1
            title = (row.get("title") or "").strip()
            question = (row.get("ask") or "").strip()
            answer = (row.get("answer") or "").strip()
            if not title or not question or not answer:
                skipped_bad += 1
                continue
            if "\ufffd" in dept + title + question + answer:
                skipped_bad += 1
                continue
            if len(question) < 8 or len(answer) < 15:
                skipped_short += 1
                continue
            existing = pairs.get(question)
            if existing and len(existing["answer"]) >= len(answer):
                continue
            pairs[question] = {
                "department": dept,
                "title": title,
                "question": question,
                "answer": answer,
            }

    departments = {}
    for p in pairs.values():
        departments[p["department"]] = departments.get(p["department"], 0) + 1
    doc = {
        "qa_pairs": list(pairs.values()),
        "total_count": len(pairs),
        "departments": dict(sorted(departments.items(), key=lambda x: -x[1])),
    }
    with open(OUT, "w", encoding="utf-8") as fh:
        json.dump(doc, fh, ensure_ascii=False, separators=(",", ":"))
    print(
        f"rows={total_rows} kept={len(pairs)} "
        f"skipped_short={skipped_short} skipped_bad={skipped_bad} "
        f"departments={len(departments)}",
        file=sys.stderr,
    )


if __name__ == "__main__":
    main()
