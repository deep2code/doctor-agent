#!/usr/bin/env python3
"""Convert external/hpo/hp-base.obo -> internal/knowledge/data/hpo_terms.json.

Human Phenotype Ontology terms (non-obsolete) become a structured lookup:
hpo_id / English name / EXACT+RELATED synonyms / definition. Names are
translated to Chinese in batches via the free Zhipu glm-4-flash endpoint
(the same client the other structurize pipelines use), with a per-batch
disk cache so reruns are resumable and idempotent.

API key resolution: ZHIPU_API_KEY env, else OPENAI_COMPAT_API_KEY from
~/.doctor-agent/config.env (the zero-config CLI writes it there).

After running: python3 external/make_gz.py && go run . seed-knowledge
"""

import json
import os
import re
import sys
import time
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
OBO = ROOT / "external" / "hpo" / "hp-base.obo"
OUT = ROOT / "internal" / "knowledge" / "data" / "hpo_terms.json"
CACHE = ROOT / "external" / "hpo" / "zh_cache.json"

ZHIPU_URL_FALLBACK = "https://ark.cn-beijing.volces.com/api/v3/chat/completions"
DEFAULT_MODEL = "doubao-seed-2-1-pro-260628"
BATCH = 60


def llm_config() -> tuple:
    cfg = {}
    for src in (ROOT / ".env", Path.home() / ".doctor-agent" / "config.env"):
        if src.exists():
            for line in src.read_text().splitlines():
                m = re.match(r"(?:export\s+)?([A-Z_]+)\s*=\s*(.+)", line.strip())
                if m:
                    cfg.setdefault(m.group(1), m.group(2).strip().strip('"').strip("'"))
    key = os.environ.get("LLM_API_KEY") or cfg.get("OPENAI_COMPAT_API_KEY") or cfg.get("ZHIPU_API_KEY") or ""
    url = os.environ.get("LLM_CHAT_URL") or cfg.get("OPENAI_COMPAT_BASE_URL") or ZHIPU_URL_FALLBACK
    model = os.environ.get("LLM_MODEL") or cfg.get("OPENAI_COMPAT_MODEL") or DEFAULT_MODEL
    if not url.rstrip("/").endswith("chat/completions"):
        url = url.rstrip("/") + "/chat/completions"
    return url, key, model


def translate_batch(cfg: tuple, names: list) -> dict:
    """names -> {name: zh}; raises on failure (caller retries)."""
    url, key, model = cfg
    prompt = (
        "把下列医学英语术语逐个翻译成简短中文术语，只输出 JSON 数组"
        "（与输入等长、顺序一致），不要任何解释:\n"
        + json.dumps(names, ensure_ascii=False)
    )
    body = json.dumps({
        "model": model,
        "messages": [{"role": "user", "content": prompt}],
        "temperature": 0.1,
        "max_tokens": 4096,
        # doubao-seed pro is a reasoning model: without this each call burns
        # ~40s of thinking (and times out); disabling cuts it to ~3s.
        "thinking": {"type": "disabled"},
    }).encode()
    req = urllib.request.Request(url, data=body, headers={
        "Content-Type": "application/json",
        "Authorization": "Bearer " + key,
    })
    with urllib.request.urlopen(req, timeout=120) as resp:
        data = json.loads(resp.read())
    content = data["choices"][0]["message"]["content"].strip()
    m = re.search(r"\[.*\]", content, re.S)
    if not m:
        raise ValueError("no JSON array in reply: " + content[:100])
    out = json.loads(m.group(0))
    if len(out) != len(names):
        raise ValueError(f"length mismatch {len(out)} != {len(names)}")
    return dict(zip(names, out))


def parse_obo():
    terms = []
    cur = None
    section = None
    with open(OBO, encoding="utf-8") as fh:
        for line in fh:
            line = line.rstrip("\n")
            if line == "[Term]":
                section = "term"
                if cur and cur.get("id"):
                    terms.append(cur)
                cur = {"id": "", "name": "", "def": "", "synonyms": [], "obsolete": False}
                continue
            if line in ("[Typedef]", "[Relationship]"):
                section = "other"
                continue
            if section != "term" or cur is None:
                continue
            if line.startswith("id: "):
                cur["id"] = line[4:].strip()
            elif line.startswith("name: "):
                cur["name"] = line[6:].strip()
            elif line.startswith("is_obsolete: true"):
                cur["obsolete"] = True
            elif line.startswith("def: "):
                m = re.match(r'def: "(.*?)"', line)
                if m and not cur["def"]:
                    cur["def"] = m.group(1).replace('\\"', '"')
            elif line.startswith("synonym: "):
                m = re.match(r'synonym: "(.*?)"\s+(\w+)', line)
                if m and m.group(2) in ("EXACT", "RELATED"):
                    syn = m.group(1).replace('\\"', '"')
                    if not re.fullmatch(r"HP:\d+", syn):
                        cur["synonyms"].append(syn)
        if cur and cur.get("id"):
            terms.append(cur)
    return [t for t in terms if t["id"].startswith("HP:") and t["name"] and not t["obsolete"]]


def main() -> None:
    cfg = llm_config()
    if not cfg[1]:
        sys.exit("no LLM api key (LLM_API_KEY env / OPENAI_COMPAT_API_KEY in config.env)")
    raw = parse_obo()
    print(f"parsed {len(raw)} live terms", file=sys.stderr)

    cache = json.loads(CACHE.read_text()) if CACHE.exists() else {}
    todo = sorted({t["name"] for t in raw if t["name"] not in cache and t.get("name")})
    print(f"to translate: {len(todo)} names ({len(cache)} cached)", file=sys.stderr)
    for i in range(0, len(todo), BATCH):
        batch = todo[i:i + BATCH]
        for attempt in range(5):
            try:
                result = translate_batch(cfg, batch)
                cache.update(result)
                break
            except Exception as e:
                print(f"batch {i//BATCH} attempt {attempt}: {e}", file=sys.stderr)
                time.sleep(15 * (attempt + 1))
        else:
            # rate-limit / transient failure: skip this batch (untranslated
            # names stay out of the cache, so a re-run retries them) and
            # still emit the output file.
            print(f"batch {i//BATCH} failed; skipping", file=sys.stderr)
        CACHE.write_text(json.dumps(cache, ensure_ascii=False))
        if (i // BATCH) % 10 == 0:
            print(f"progress {i+len(batch)}/{len(todo)}", file=sys.stderr)
        time.sleep(0.5)
    CACHE.write_text(json.dumps(cache, ensure_ascii=False))

    docs = []
    for t in raw:
        if not t.get("name"):
            continue
        docs.append({
            "hpo_id": t["id"],
            "name": t["name"],
            "name_zh": cache.get(t["name"], ""),
            "synonyms": sorted(set(t.get("synonyms", []))),
            "definition": t.get("def", ""),
        })
    docs.sort(key=lambda d: d["hpo_id"])
    doc = {
        "source": "HPO (hp-base.obo, Human Phenotype Ontology)",
        "updated": time.strftime("%Y-%m-%d"),
        "terms": docs,
    }
    with open(OUT, "w", encoding="utf-8") as fh:
        json.dump(doc, fh, ensure_ascii=False, separators=(",", ":"))
    zh_ok = sum(1 for d in docs if d["name_zh"])
    print(f"wrote {len(docs)} terms, {zh_ok} with Chinese name", file=sys.stderr)


if __name__ == "__main__":
    main()
