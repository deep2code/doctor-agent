#!/usr/bin/env python3
"""Wave-4 harvest: MORE Chinese guideline full texts into external/yiigle_guides/raw/.

Title index source (changed 2026-10-01): the CMA knowledge base's own curated
guideline catalogue — https://cmab.yiigle.com/webs/zhinan/articlesList/ — whose
LIST pages are plain static HTML and need no login (the detail page does, so only
titles/discipline are read from it; no catalogue page text is stored). The previous
brief used guide.medlive.cn/guideline/list as the index, which now 302s to a login
page and yields nothing.

Why the 「最热」 ordering and not 「最新」: measured 2026-10-01, the newest 20 titles
hit guide_html 0/20 (2025-2026 guidelines and 「…指南解读」 commentaries have no
public full text there), while the most-popular sample hit 7/15 with 10k-74k CJK
characters each. So this script crawls newOrHeat=0 only.

Full text still comes from the route verified by rounds 1-3:
    https://cmab.yiigle.com/uploads/guide_html/<URL-encoded title>.html
    https://seleguide.yiigle.com/uploads/guide_html/<same>.html
A page counts as a full guideline when HTTP 200, >=1500 CJK and it holds one of
the clinical section words.

Reuses fetch/extract/write logic from external/fetch_yiigle_guides.py (imported,
NOT modified). Idempotent and resumable: wave4_state.json records every title as
hit/miss/skip, MANIFEST.txt only ever gets lines appended under 「第四轮」, and an
existing raw file is never overwritten. Sequential, >=1.25s between requests.

Usage:
    python3 external/fetch_yiigle_guides4.py               # crawl index + fetch
    python3 external/fetch_yiigle_guides4.py --pages 5     # deeper index
    python3 external/fetch_yiigle_guides4.py --crawl-only  # dump the title index only
"""
import hashlib
import json
import os
import re
import sys
import time
import urllib.parse
import urllib.request

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fetch_yiigle_guides as fg  # noqa: E402

BASE = fg.BASE
SELE = fg.SELE_BASE
SLEEP = 1.25
MIN_CJK = 1500
SUBSTRINGS = ("诊断", "治疗", "推荐", "筛查", "管理", "共识", "指南", "防治", "规范")
MANIFEST = os.path.join(fg.OUT_DIR, "MANIFEST.txt")
STATE = os.path.join(fg.OUT_DIR, "wave4_state.json")
INDEX = os.path.join(fg.OUT_DIR, "wave4_index.json")
LIST_URL = "https://cmab.yiigle.com/webs/zhinan/articlesList/"
ITEM_RE = re.compile(r'<h1><a href="/webs/zhinan/getPerInfo\?id=(\d+)">([^<]+)</a></h1>')

# (discipline name, xuekeId) — read off /webs/zhinan on 2026-10-01; 综合 carries an
# empty id on purpose. Each discipline is crawled with xueke= (all subcategories).
DISCIPLINES = [
    ("内科", "6E0F8E4DF6467297E050A8C01A015B1C"),
    ("外科", "6E0F8E4DF6477297E050A8C01A015B1C"),
    ("妇产科", "6E0F8E4DF64B7297E050A8C01A015B1C"),
    ("儿科", "6E0F8E4DF6397297E050A8C01A015B1C"),
    ("小儿外科", "6E0F8E4DF6487297E050A8C01A015B1C"),
    ("儿童保健科", "129e887759d511e9ac2c00163e2ee9fa"),
    ("眼科", "6E0F8E4DF6427297E050A8C01A015B1C"),
    ("耳鼻咽喉科", "6E0F8E4DF64C7297E050A8C01A015B1C"),
    ("肿瘤科", "467b1b5459d611e9ac2c00163e2ee9fa"),
    ("重症医学科", "857ee79859d611e9ac2c00163e2ee9fa"),
    ("急诊医学科", "ac601b2559d611e9ac2c00163e2ee9fa"),
    ("皮肤科", "6E0F8E4DF6497297E050A8C01A015B1C"),
    ("医疗美容科", "6E0F8E4DF69A7297E050A8C01A015B1C"),
    ("口腔科", "6E0F8E4DF64F7297E050A8C01A015B1C"),
    ("传染科", "6E0F8E4DF6507297E050A8C01A015B1C"),
    ("结核病科", "78E307E23B8DB610E050A8C01A0114D0"),
    ("麻醉科", "6E0F8E4DF64E7297E050A8C01A015B1C"),
    ("预防保健科", "6E0F8E4DF6407297E050A8C01A015B1C"),
    ("全科医疗科", "6E0F8E4DF6917297E050A8C01A015B1C"),
    ("精神科", "6E0F8E4DF6417297E050A8C01A015B1C"),
    ("病理科", "1e947afa59da11e9ac2c00163e2ee9fa"),
    ("医学影像科", "6E0F8E4DF63C7297E050A8C01A015B1C"),
    ("医学检验科", "929fbb8af4a111e9ac2c00163e2ee9fa"),
    ("康复医学科", "6E0F8E4DF64A7297E050A8C01A015B1C"),
    ("特种医学与军事医学科", "9864ce4f59dd11e9ac2c00163e2ee9fa"),
    ("疼痛科", "39fc967ef55b11e9ac2c00163e2ee9fa"),
    ("综合", ""),
]


def get(url):
    req = urllib.request.Request(url, headers={"User-Agent": fg.UA})
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return resp.read().decode("utf-8", "ignore")
    except Exception as e:  # a dead index page just skips that page
        print("    INDEX ERR %s (%s)" % (url[-70:], e), flush=True)
        return ""


def crawl(pages):
    """[(title, discipline), ...] from the 「最热」 ordering of every discipline."""
    seen, out = set(), []
    for name, xid in DISCIPLINES:
        kept = 0
        for page in range(1, pages + 1):
            url = "%s?xuekeId=%s&&xueke=&&name=%s&&newOrHeat=0&&page=%d" % (
                LIST_URL, xid, urllib.parse.quote(name), page)
            h = get(url)
            time.sleep(0.8)
            if not h:
                break
            found = 0
            for m in ITEM_RE.finditer(h):
                title = fg.html.unescape(m.group(2)).strip()
                if title and title not in seen:
                    seen.add(title)
                    out.append((title, name))
                    kept += 1
                    found += 1
            if found == 0:
                break  # past the last page of this discipline
        print("  %-14s %4d titles" % (name, kept), flush=True)
    return out


def load_state():
    if os.path.exists(STATE):
        with open(STATE, encoding="utf-8") as f:
            return json.load(f)
    return {}


def save_state(state):
    with open(STATE, "w", encoding="utf-8") as f:
        json.dump(state, f, ensure_ascii=False, indent=1)


def clinical(text):
    return any(s in text for s in SUBSTRINGS)


def manifest_blob():
    if not os.path.exists(MANIFEST):
        return ""
    with open(MANIFEST, encoding="utf-8") as f:
        return f.read()


def ensure_round_header():
    if "第四轮" not in manifest_blob():
        with open(MANIFEST, "a", encoding="utf-8") as f:
            f.write("# ===== 第四轮 (wave4, cmab 精选指南库「最热」目录作标题索引；"
                    "cmab+seleguide 双基址、全角/半角括号变体) =====\n")


def try_one(slug, title, disc):
    if os.path.exists(os.path.join(fg.OUT_DIR, slug + ".txt")):
        return ("skip", title)
    for base in (BASE, SELE):
        for variant in fg.name_variants(title):
            url = base + urllib.parse.quote(variant + ".html")
            status, body = fg.fetch(url)
            time.sleep(SLEEP)
            if status != 200 or not body:
                continue
            text, page_title = fg.extract_text(body.decode("utf-8", "replace"))
            c = fg.cjk_count(text)
            if c < MIN_CJK or not clinical(text):
                continue
            fg.write_hit(slug, variant, url, page_title or variant, text, disc)
            year = (re.search(r"(20\d{2})", variant) or [""])[0]
            with open(MANIFEST, "a", encoding="utf-8") as f:
                f.write("%s.txt | %s | %s | %s | %s\n" % (slug, variant, url, disc, year))
            return ("hit", "%s (%d cjk, %s)"
                    % (variant[:30], c, base.split("/")[2].split(".")[0]))
    return ("miss", title)


def main():
    args = sys.argv[1:]
    pages = int(args[args.index("--pages") + 1]) if "--pages" in args else 3

    os.makedirs(fg.OUT_DIR, exist_ok=True)
    print("crawling 精选指南库 最热 index: %d disciplines x %d pages"
          % (len(DISCIPLINES), pages), flush=True)
    titles = crawl(pages)
    print("index titles: %d" % len(titles), flush=True)
    with open(INDEX, "w", encoding="utf-8") as f:
        json.dump(titles, f, ensure_ascii=False, indent=1)
    if "--crawl-only" in args:
        return

    state, blob = load_state(), manifest_blob()
    ensure_round_header()
    hits = misses = skips = known = 0
    for i, (title, disc) in enumerate(titles, 1):
        if title in state:
            known += 1
            continue
        if any(v in blob for v in fg.name_variants(title)):
            # recorded by rounds 1-3 as a hit or a proven miss — never retried
            state[title] = "already-in-manifest"
            known += 1
            continue
        slug = "w4" + hashlib.sha1(title.encode("utf-8")).hexdigest()[:10]
        kind, info = try_one(slug, title, disc)
        state[title] = kind
        save_state(state)
        if kind == "hit":
            hits += 1
            blob = manifest_blob()
        elif kind == "skip":
            skips += 1
        else:
            misses += 1
        print("[%4d] %-5s %s | %s" % (i, kind.upper(), title[:44], info), flush=True)

    with open(MANIFEST, "a", encoding="utf-8") as f:
        f.write("# 第四轮未命中（cmab/seleguide 均无同名 guide_html 全文，含括号变体）:\n")
        for t, _d in titles:
            if state.get(t) == "miss":
                f.write("#   %s\n" % t)
    print("done: %d hits, %d misses, %d skips, %d known/already-recorded"
          % (hits, misses, skips, known), flush=True)


if __name__ == "__main__":
    main()
