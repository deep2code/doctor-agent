#!/usr/bin/env python3
"""Convert OpenAI's CMExam (Chinese medical licensing exam) test split into an
evals Chinese MCQ golden set (evals/questions_cmexam.json).

Input (downloaded once into external/cmtb/cmexam/):
  test_with_annotations.csv  (6,811 questions + Disease Group / Area of
  Competency / Clinical Department / Medical Discipline / Difficulty labels)

Only single-answer items are used (204 multi-answer rows dropped) — the
harness ExpectedOption check cannot express option sets. Sampling is
stratified across Area of Competency, deterministic (fixed seed), 200 items.

Run: python3 external/convert_cmexam.py [-n 200]
"""

import argparse
import csv
import json
import random
import re
from collections import defaultdict
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SRC = ROOT / "external" / "cmtb" / "cmexam" / "test_with_annotations.csv"
OUT = ROOT / "evals" / "questions_cmexam.json"

OPTION_LINE = re.compile(r"^([A-E])\s+(.+)$")


def parse_options(raw: str) -> dict[str, str]:
    opts = {}
    for line in raw.splitlines():
        m = OPTION_LINE.match(line.strip())
        if m:
            opts[m.group(1)] = m.group(2).strip()
    return opts


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("-n", type=int, default=200)
    ap.add_argument("-seed", type=int, default=42)
    args = ap.parse_args()

    rows = list(csv.DictReader(open(SRC, newline="", encoding="utf-8")))
    usable = []
    for r in rows:
        if len(r["Answer"]) != 1 or r["Answer"] not in "ABCDE":
            continue
        opts = parse_options(r["Options"])
        if r["Answer"] not in opts or len(opts) < 2:
            continue
        usable.append((r, opts))
    print(f"usable single-choice: {len(usable)} / {len(rows)}")

    by_domain = defaultdict(list)
    for item in usable:
        by_domain[item[0]["Area of Competency"]].append(item)

    rng = random.Random(args.seed)
    domains = sorted(by_domain)
    quota = max(1, args.n // len(domains))
    picked = []
    for d in domains:
        pool = by_domain[d]
        rng.shuffle(pool)
        picked.extend(pool[:quota])
    leftovers = [p for d in domains for p in by_domain[d][quota:]]
    rng.shuffle(leftovers)
    picked.extend(leftovers[: args.n - len(picked)])
    picked = picked[: args.n]
    rng.shuffle(picked)

    items = []
    for i, (r, opts) in enumerate(picked):
        letters = "".join(sorted(opts))
        body = (
            r["Question"]
            + "\n"
            + "\n".join(f"{k}. {v}" for k, v in sorted(opts.items()))
            + "\n请直接给出正确选项字母（"
            + "/".join(letters)
            + "）。"
        )
        items.append(
            {
                "id": f"cmexam-{i:05d}",
                "category": "mcq_zh",
                "question": body,
                "expected_keywords": [],
                "must_not_contain": [],
                "should_refuse": False,
                "expected_option": r["Answer"],
                "notes": (
                    f"CMExam {r['Medical Discipline']}/{r['Area of Competency']}"
                    f"/难度{r['Difficulty level']}"
                ),
            }
        )

    out = {
        "meta": {
            "source": "OpenAI CMExam test split (test_with_annotations.csv), 单选题按 Area of Competency 分层抽样",
            "total": len(items),
        },
        "questions": items,
    }
    OUT.write_text(json.dumps(out, ensure_ascii=False, indent=1), encoding="utf-8")
    print(f"wrote {OUT} — {len(items)} items across {len(domains)} domains")


if __name__ == "__main__":
    main()
