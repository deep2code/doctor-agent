#!/usr/bin/env python3
"""Download WHO vaccine position papers (preferring official Chinese PDFs)
from the WHO IRIS repository (iris.who.int, DSpace 7 REST API).

Idempotent: skips existing files. For each vaccine we search IRIS for the
"WHO position paper" item, pick the most recent, and download the -chi.pdf
bitstream when present (else the main English PDF). The TEXT bundle .txt is
also saved when available so downstream structuring needs no PDF parsing.
"""
import json, os, re, sys, time, urllib.request, urllib.parse
from pathlib import Path

OUT = Path(__file__).parent / "position_papers"
API = "https://iris.who.int/server/api"
UA = {"User-Agent": "Mozilla/5.0 (research; contact: self)"}

# vaccine -> (search term, 中文名). Search targets the IRIS item title.
VACCINES = [
    ("rabies", "Rabies vaccines WHO position paper", "狂犬病疫苗"),
    ("japanese-encephalitis", "Japanese Encephalitis vaccines WHO position paper", "乙脑疫苗"),
    ("hpv", "Human papillomavirus vaccines WHO position paper", "HPV疫苗"),
    ("hepatitis-b", "Hepatitis B vaccines WHO position paper", "乙肝疫苗"),
    ("dengue", "Dengue vaccines WHO position paper", "登革热疫苗"),
    ("influenza", "Influenza vaccines WHO position paper", "流感疫苗"),
    ("typhoid", "Typhoid vaccines WHO position paper", "伤寒疫苗"),
    ("cholera", "Cholera vaccines WHO position paper", "霍乱疫苗"),
    ("tetanus-diphtheria", "Tetanus diphtheria vaccine WHO position paper", "破伤风白喉疫苗"),
    ("rotavirus", "Rotavirus vaccines WHO position paper", "轮状病毒疫苗"),
    ("measles", "Measles vaccines WHO position paper", "麻疹疫苗"),
    ("pneumococcal", "Pneumococcal vaccines WHO position paper", "肺炎球菌疫苗"),
]


def get_json(url):
    req = urllib.request.Request(url, headers=UA)
    for i in range(3):
        try:
            with urllib.request.urlopen(req, timeout=30) as r:
                return json.loads(r.read())
        except Exception as e:
            if i == 2:
                raise
            time.sleep(2)


def search_items(query, size=10):
    url = f"{API}/discover/search/objects?query={urllib.parse.quote(query)}&size={size}"
    d = get_json(url)
    out = []
    for o in d.get("_embedded", {}).get("searchResult", {}).get("_embedded", {}).get("objects", []):
        io = o.get("_embedded", {}).get("indexableObject", {})
        out.append({"uuid": io.get("uuid"), "name": io.get("name", ""), "handle": io.get("handle", "")})
    return out


def item_bitstreams(uuid):
    bs = []
    bundles = get_json(f"{API}/core/items/{uuid}/bundles")
    for b in bundles.get("_embedded", {}).get("bundles", []):
        if b.get("name") not in ("ORIGINAL", "TEXT"):
            continue
        d = get_json(f"{API}/core/bundles/{b['uuid']}/bitstreams")
        for bit in d.get("_embedded", {}).get("bitstreams", []):
            bs.append({"bundle": b["name"], "name": bit["name"], "uuid": bit["uuid"]})
    return bs


def download(uuid, dest):
    url = f"{API}/core/bitstreams/{uuid}/content"
    req = urllib.request.Request(url, headers=UA)
    with urllib.request.urlopen(req, timeout=120) as r:
        data = r.read()
    dest.write_bytes(data)
    return len(data)


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    for name, query, zh in VACCINES:
        pdf = OUT / f"{name}.pdf"
        if pdf.exists():
            print(f"SKIP {name} (已存在)", file=sys.stderr)
            continue
        try:
            items = search_items(query, 12)
            # Filter to items whose name looks like the position paper itself
            # (not "summary", not "background document", not "module").
            cands = [it for it in items
                     if re.search(r"position paper", it["name"], re.I)
                     and not re.search(r"summary|background|module|Q&A|gr\s?ade", it["name"], re.I)]
            if not cands:
                cands = items
            # Prefer the item whose name contains a 4-digit year (recent ones do).
            dated = [it for it in cands if re.search(r"(19|20)\d{2}", it["name"])]
            pick = (dated or cands)[0]
            print(f"[{name}] 条目: {pick['name'][:80]} ({pick.get('handle')})", file=sys.stderr)

            bits = item_bitstreams(pick["uuid"])
            chi = next((b for b in bits if b["bundle"] == "ORIGINAL" and re.search(r"chi", b["name"], re.I)), None)
            eng = next((b for b in bits if b["bundle"] == "ORIGINAL" and b["name"].lower().endswith(".pdf")), None)
            txt = next((b for b in bits if b["bundle"] == "TEXT" and "chi" in b["name"].lower()), None)
            if not txt:
                txt = next((b for b in bits if b["bundle"] == "TEXT" and b["name"].lower().endswith(".txt")), None)

            src = chi or eng
            if not src:
                print(f"[{name}] ❌ 无 PDF bitstream", file=sys.stderr)
                continue
            n = download(src["uuid"], pdf)
            if txt:
                download(txt["uuid"], OUT / f"{name}.txt")
                print(f"[{name}] ✅ {src['name']} ({n//1024}KB) + 文本 {txt['name']}", file=sys.stderr)
            else:
                print(f"[{name}] ✅ {src['name']} ({n//1024}KB, 无文本bundle)", file=sys.stderr)
        except Exception as e:
            print(f"[{name}] ❌ {str(e)[:120]}", file=sys.stderr)
        time.sleep(1)


if __name__ == "__main__":
    main()
