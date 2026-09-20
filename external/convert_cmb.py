#!/usr/bin/env python3
"""Convert the FreedomIntelligence/CMB exam test split into an evals Chinese
MCQ golden set (evals/questions_cmb.json).

Inputs (downloaded once into external/cmb/):
  CMB/CMB-Exam/CMB-test/CMB-test-choice-question-merge.json  (11,200 questions)
  CMB-test-choice-answer.json                                 (answer letters)

Only 单项选择题 (single-answer) items are used; 多项选择题 need exact set
matching which the harness cannot express. Sampling is stratified across
exam_subject, deterministic (fixed seed), default 200 items.

Run: python3 external/convert_cmb.py [-n 200]
"""

import argparse
import json
import random
from collections import defaultdict
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
CMB = ROOT / "external" / "cmb"
OUT = ROOT / "evals" / "questions_cmb.json"


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("-n", type=int, default=200)
    ap.add_argument("-seed", type=int, default=42)
    args = ap.parse_args()

    questions = json.load(
        open(CMB / "CMB/CMB-Exam/CMB-test/CMB-test-choice-question-merge.json")
    )
    answers = {a["id"]: a for a in json.load(open(CMB / "CMB-test-choice-answer.json"))}

    singles = [q for q in questions if q.get("question_type") == "单项选择题"]
    print(f"single-choice items: {len(singles)}")

    by_subject = defaultdict(list)
    for q in singles:
        a = answers.get(q["id"])
        if not a or not str(a.get("answer", "")).strip():
            continue
        by_subject[q["exam_subject"]].append((q, a["answer"].strip()))

    rng = random.Random(args.seed)
    subjects = sorted(by_subject)
    quota = max(1, args.n // len(subjects))
    picked = []
    for sub in subjects:
        pool = by_subject[sub]
        rng.shuffle(pool)
        picked.extend(pool[:quota])
    # fill the remainder round-robin from larger pools
    leftovers = [p for sub in subjects for p in by_subject[sub][quota:]]
    rng.shuffle(leftovers)
    picked.extend(leftovers[: args.n - len(picked)])
    picked = picked[: args.n]
    rng.shuffle(picked)

    items = []
    for i, (q, ans) in enumerate(picked):
        opts = q["option"]
        letters = "".join(opts)
        body = q["question"] + "\n" + "\n".join(
            f"{k}. {v}" for k, v in sorted(opts.items())
        ) + "\n请直接给出正确选项字母（" + "/".join(letters) + "）。"
        items.append(
            {
                "id": f"cmb-{q['id']:05d}",
                "category": "mcq_zh",
                "question": body,
                "expected_keywords": [],
                "must_not_contain": [],
                "should_refuse": False,
                "expected_option": ans,
                "notes": f"CMB {q['exam_type']}/{q['exam_class']}/{q['exam_subject']}",
            }
        )

    out = {
        "meta": {
            "source": "FreedomIntelligence/CMB (Chinese Medical Benchmark) CMB-Exam test split, 单选题抽样",
            "total": len(items),
        },
        "questions": items,
    }
    OUT.write_text(json.dumps(out, ensure_ascii=False, indent=1), encoding="utf-8")
    print(f"wrote {OUT} — {len(items)} items across {len(subjects)} subjects")


if __name__ == "__main__":
    main()
