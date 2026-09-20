#!/usr/bin/env python3
"""Parse ICD-O-3 (3rd edition, WHO 2019 update) morphology codes from the
official English PDF into data/icdo3_morphology.json.

Source: external/icdo3/icdo3_eng_3rd_edition.pdf (252pp). The morphology
list starts at the "800 Neoplasms, NOS" heading and ends at "9989"
(whole field / residual). Layout per record:

  8000/0\t Neoplasm, benign
  Tumor, benign\t\t\t\t(unindented = synonym lines of the previous code)
  Unclassified, benign
  8001/0\t Tumor cells, benign

zh names: batch LLM translation via the same OpenAI-compatible endpoint as
convert_hpo.py (thinking disabled), cached in external/icdo3/zh_cache.json.

Output schema (registered as exact_lookup type=icdo3 in Go):
  {source, updated, terms: [{code, behavior, name_en, synonyms[], name_zh}]}
behavior: /0 benign, /1 uncertain, /2 carcinoma in situ, /3 malignant
primary, /6 metastatic, /9 unspecified primary/metastatic.
"""

import json
import os
import re
import sys
import time
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
PDF = ROOT / "external" / "icdo3" / "icdo3_eng_3rd_edition.pdf"
CACHE = ROOT / "external" / "icdo3" / "zh_cache.json"
OUT = ROOT / "internal" / "knowledge" / "data" / "icdo3_morphology.json"

RECORD = re.compile(r"^(\d{4})/([012369])\s*\t?\s*(.+)$")


def llm_config():
    env = dict(os.environ)
    for p in (ROOT / ".env", Path.home() / ".doctor-agent/config.env"):
        if p.exists():
            for line in p.read_text(encoding="utf-8").splitlines():
                line = line.strip()
                if line and not line.startswith("#") and "=" in line:
                    k, _, v = line.partition("=")
                    env.setdefault(k.strip(), v.strip().strip('"').strip("'"))
    base = env.get("OPENAI_COMPAT_BASE_URL", "")
    key = env.get("OPENAI_COMPAT_API_KEY", "")
    model = env.get("OPENAI_COMPAT_MODEL", "")
    if not (base and key and model):
        return None
    if not base.endswith("/chat/completions"):
        base = base.rstrip("/") + "/chat/completions"
    return {"url": base, "key": key, "model": model}


def translate_batch(cfg, names):
    prompt = (
        "你是医学名词翻译。把下列 ICD-O-3 肿瘤形态学英文名词翻译成简体中文"
        "（标准病理学术语，如 Neoplasm, benign->良性肿瘤）。"
        "只输出一个 JSON 数组，元素与输入一一对应，不要编号或解释。\n"
        + json.dumps(names, ensure_ascii=False)
    )
    body = json.dumps(
        {
            "model": cfg["model"],
            "max_tokens": 4096,
            "temperature": 0,
            "thinking": {"type": "disabled"},
            "messages": [{"role": "user", "content": prompt}],
        }
    ).encode()
    req = urllib.request.Request(
        cfg["url"],
        data=body,
        headers={
            "Authorization": "Bearer " + cfg["key"],
            "Content-Type": "application/json",
        },
    )
    with urllib.request.urlopen(req, timeout=180) as resp:
        out = json.load(resp)
    content = out["choices"][0]["message"]["content"].strip()
    m = re.search(r"\[.*\]", content, re.S)
    arr = json.loads(m.group(0) if m else content)
    if not isinstance(arr, list) or len(arr) != len(names):
        raise ValueError(f"length mismatch {len(arr)} != {len(names)}")
    return [str(x).strip() for x in arr]


def parse_pdf():
    import pymupdf

    doc = pymupdf.open(PDF)
    start = None
    for pno in range(len(doc)):
        if "800 Neoplasms, NOS" in doc[pno].get_text():
            start = pno
            break
    if start is None:
        sys.exit("morphology section start not found")

    terms = []

    def feed(line, cur):
        s = line.strip()
        if not s:
            return cur
        m = RECORD.match(s)
        if m and 8000 <= int(m.group(1)) <= 9995:
            terms.append(
                {
                    "code": f"{m.group(1)}/{m.group(2)}",
                    "behavior": m.group(2),
                    "name_en": m.group(3).strip(),
                    "synonyms": [],
                }
            )
            return terms[-1]
        if cur is None:
            return None
        if re.match(r"^\d{3}[ -]", s) or re.match(r"^\d{4}/", s):
            # 3-digit group headings and out-of-list codes: skip, don't synonym
            return cur
        # a wrapped fragment continues the previous line; synonyms are
        # Title-cased while wrap fragments frequently start lowercase
        if s[0].islower() or s[0] in ")]、," or s.startswith(("whether", "benign")):
            if cur["synonyms"]:
                cur["synonyms"][-1] += " " + s
            else:
                cur["name_en"] += " " + s
        else:
            cur["synonyms"].append(s)
        return cur

    done = False
    for pno in range(start, len(doc)):
        if done:
            break
        blocks = doc[pno].get_text("blocks")
        mid = doc[pno].rect.width / 2
        left = sorted((b for b in blocks if b[0] < mid - 40), key=lambda b: b[1])
        right = sorted((b for b in blocks if b[0] >= mid - 40), key=lambda b: b[1])
        for col in (left, right):
            cur = None
            for b in col:
                for line in b[4].splitlines():
                    cur = feed(line, cur)
        if terms and terms[-1]["code"][:4] >= "9992":
            done = True
    return terms


def main():
    terms = parse_pdf()
    # The PDF marks the first line of each record after the code; continuation
    # lines of the *record's own name* are rare — most plain following lines
    # are synonyms. We merged them into the last synonym/name; split back any
    # that clearly started a new synonym later during zh translation is not
    # needed: keep merged form (search tolerates it).
    print(f"parsed {len(terms)} morphology records")
    codes = [t["code"] for t in terms]
    assert len(codes) == len(set(codes)), "duplicate codes"

    cache = json.loads(CACHE.read_text()) if CACHE.exists() else {}
    cfg = llm_config()
    need = [t["name_en"] for t in terms if t["name_en"] not in cache]
    if cfg is None:
        print("no LLM config — writing English-only", file=sys.stderr)
    else:
        need = list(dict.fromkeys(need))
        B = 50
        for i in range(0, len(need), B):
            chunk = need[i : i + B]
            for attempt in range(3):
                try:
                    zs = translate_batch(cfg, chunk)
                    cache.update(dict(zip(chunk, zs)))
                    break
                except Exception as e:  # noqa: BLE001
                    print(f"batch {i} attempt {attempt}: {e}", file=sys.stderr)
                    time.sleep(3)
            CACHE.write_text(
                json.dumps(cache, ensure_ascii=False), encoding="utf-8"
            )
            print(f"zh progress {i + len(chunk)}/{len(need)}", file=sys.stderr)

    for t in terms:
        zh = cache.get(t["name_en"])
        if zh:
            t["name_zh"] = zh
    out = {
        "source": "WHO ICD-O-3 第三版(2019更新)形态学编码(8000-9989)",
        "updated": "2019-04",
        "terms": terms,
    }
    OUT.write_text(
        json.dumps(out, ensure_ascii=False, separators=(",", ":")), encoding="utf-8"
    )
    zh_n = sum(1 for t in terms if t.get("name_zh"))
    print(f"wrote {OUT} — {len(terms)} terms, {zh_n} translated")


if __name__ == "__main__":
    main()
