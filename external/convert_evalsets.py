#!/usr/bin/env python3
"""Convert external MedQA + PubMedQA eval sets into evals/questions_en.json.

- MedQA: four-option English MCQs (train+dev+test, 12k+). We sample a balanced
  subset (option letters A-D evenly) so online eval stays affordable.
- PubMedQA: yes/no/maybe articles. Sample balanced by final_decision.

Output schema matches evals/eval.go Question, with the extra
`expected_option` field (the correct option letter / yes-no-maybe label).
"""
import json, random, re, sys
from pathlib import Path

EVAL = Path(__file__).parent.parent / "evals"
MEDQA = Path(__file__).parent / "evalsets" / "medqa"
PUBMEDQA = Path(__file__).parent / "evalsets" / "pubmedqa" / "labeled.json"
OUT = EVAL / "questions_en.json"

MEDQA_N = 200          # MCQs to sample
PUBMEDQA_N = 100       # yes/no/maybe items to sample
random.seed(42)

STOPWORDS = set("""the a an of and or to in for with on at by from as is are was were
be been being this that these those it its their there they which who whom what when
where how about into over after before between during without within than then so but
not no can could should would may might must will shall do does did have has had most
more less such both each other any all some new study studies patient patients""".split())

def sig_words(text, n=4):
    words = [w for w in re.findall(r"[A-Za-z][A-Za-z\-]{3,}", text)
             if w.lower() not in STOPWORDS and not w.isdigit()]
    seen, out = set(), []
    for w in words:
        lw = w.lower()
        if lw not in seen:
            seen.add(lw)
            out.append(w)
        if len(out) >= n:
            break
    return out

def medqa_sample():
    questions = []
    for split in ("train", "dev", "test"):
        path = MEDQA / f"{split}.json"
        if not path.exists():
            continue
        d = json.load(open(path))
        questions.extend(d["data"])
    print(f"MedQA 共 {len(questions)} 题", file=sys.stderr)

    by_opt = {"A": [], "B": [], "C": [], "D": []}
    for q in questions:
        opt = q.get("Correct Option", "")
        if opt in by_opt:
            by_opt[opt].append(q)

    per = MEDQA_N // 4
    picked = []
    for opt in "ABCD":
        pool = by_opt[opt]
        picked.extend(random.sample(pool, min(per, len(pool))))
    picked = picked[:MEDQA_N]
    random.shuffle(picked)

    out = []
    for i, q in enumerate(picked):
        opt = q["Correct Option"]
        correct = q["Options"][opt]
        kws = sig_words(correct)
        out.append({
            "id": f"medqa-{i+1:04d}",
            "category": "mcq_en",
            "question": q["Question"] + "\n\nOptions:\n" +
                        "\n".join(f"{k}. {q['Options'][k]}" for k in "ABCD"),
            "expected_keywords": kws,
            "expected_option": opt,
            "notes": f"正确答案: {opt} — {correct[:80]}",
        })
    return out

def pubmedqa_sample():
    d = json.load(open(PUBMEDQA))
    items = list(zip(d["pubid"], d["question"], d["context"], d["final_decision"]))
    print(f"PubMedQA 共 {len(items)} 条", file=sys.stderr)

    by_dec = {"yes": [], "no": [], "maybe": []}
    for it in items:
        dec = str(it[3]).strip().lower()
        if dec in by_dec:
            by_dec[dec].append(it)

    per = PUBMEDQA_N // 3
    picked = []
    for dec in ("yes", "no", "maybe"):
        pool = by_dec[dec]
        picked.extend(random.sample(pool, min(per, len(pool))))
    picked = picked[:PUBMEDQA_N]
    random.shuffle(picked)

    out = []
    for i, (pmid, question, context, decision) in enumerate(picked):
        # context entries are dicts with a 'contexts' list; join the article texts.
        ctx_text = ""
        if isinstance(context, dict):
            ctx_text = " ".join(context.get("contexts", []))[:2000]
        out.append({
            "id": f"pubmedqa-{i+1:04d}",
            "category": "pubmedqa_en",
            "question": f"PubMed abstract:\n{ctx_text}\n\nQuestion: {question}",
            # PubMedQA is a pure yes/no/maybe task: grade on the option only,
            # not on echoing question wording.
            "expected_option": str(decision).strip().lower(),
            "notes": f"PMID {pmid}",
        })
    return out

def main():
    qs = {
        "meta": {
            "name": "英文医学评测集 (MedQA + PubMedQA)",
            "version": "1.0.0",
            "updated": "2026-08-08",
            "description": f"英文 MCQ/三分类评测：MedQA {MEDQA_N} 题（四选一，expected_option 为 A-D）+ PubMedQA {PUBMEDQA_N} 条（yes/no/maybe）。用于验证英文文献检索与知识库外医学问题的回答能力。",
        },
        "questions": medqa_sample() + pubmedqa_sample(),
    }
    OUT.write_text(json.dumps(qs, ensure_ascii=False, indent=1), encoding="utf-8")
    print(f"✅ 写出 {OUT}  ({len(qs['questions'])} 题)", file=sys.stderr)

if __name__ == "__main__":
    main()
