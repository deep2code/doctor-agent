#!/usr/bin/env python3
"""Continuation crawl for 健康报网 (jkb.com.cn, 国家卫健委主管) — 慢速续抓新文章.

external/fetch_jkb.py already archived 828 articles under external/jkb_health/
(read-only reference; this script never writes there). The host rate-limited us
with HTTP 521 right after that batch, so this pass is deliberately polite:
single-threaded, >=2.0s between requests, <=25 list pages per channel, and a
hard cap on new details.

Output: external/jkb_more/raw/*.txt + raw/MANIFEST.txt (verbatim text, no rewrite).
Touches nothing under internal/knowledge/.

  python3 external/fetch_jkb_more.py            # normal run (cap 150 new articles)
  python3 external/fetch_jkb_more.py -limit 6   # smoke test
"""
import html
import json
import os
import re
import sys
import time

import requests

BASE = "https://www.jkb.com.cn"
HERE = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.join(HERE, "jkb_more")
RAW = os.path.join(OUT, "raw")
PREV_RAW = os.path.join(HERE, "jkb_health", "raw")  # read-only dedupe source

CHANNELS = {
    "science": "医学科普",
    "TCM": "中医中药",
    "horizon": "医药视界",
}
MAX_LIST_PAGES = 25
DELAY = 2.0          # seconds between every HTTP request
NEW_CAP = 150        # hard cap on newly written articles per run
MIN_CN = 400         # drop bodies under this many Chinese characters

UA = {
    "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
    "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36",
    "Accept-Language": "zh-CN,zh;q=0.9",
    "Referer": f"{BASE}/science/index.html",
}

_last = [0.0]
_state = {"blocked": False}


def polite_get(url):
    """Throttled GET. Returns text, or None on 404/521/error (no retry storm)."""
    wait = DELAY - (time.time() - _last[0])
    if wait > 0:
        time.sleep(wait)
    try:
        r = requests.get(url, headers=UA, timeout=25)
        _last[0] = time.time()
    except Exception as e:  # noqa: BLE001
        print(f"  ! {url} {e}", file=sys.stderr)
        return None
    if r.status_code == 521:
        print(f"  !! 521 rate-limited at {url} — stopping crawl", file=sys.stderr)
        _state["blocked"] = True
        return None
    if r.status_code in (404, 410):
        return None
    if r.status_code != 200 or not r.content:
        print(f"  ! {r.status_code} {url}", file=sys.stderr)
        return None
    r.encoding = "utf-8"
    return r.text


def extract_block(doc, anchor):
    """Return the inner HTML of the <div ... anchor ...> block, div-depth matched.

    The site inlines illustrations as <div style="text-align:center"><img/></div>
    inside the body, so a non-greedy `(.*?)</div>` cut every illustrated article
    at its first image (defect inherited from external/fetch_jkb.py). Counting
    nested <div>/</div> instead yields the whole body.
    """
    pos = doc.find(anchor)
    if pos < 0:
        return ""
    open_start = doc.rfind("<div", 0, pos)
    if open_start < 0:
        return ""
    tag_end = doc.find(">", open_start)
    if tag_end < 0:
        return ""
    depth = 0
    cursor = open_start
    while True:
        nxt_open = doc.find("<div", cursor)
        nxt_close = doc.find("</div>", cursor)
        if nxt_close < 0:
            return doc[tag_end + 1 :]
        if 0 <= nxt_open < nxt_close:
            depth += 1
            cursor = nxt_open + 4
        else:
            depth -= 1
            cursor = nxt_close + 6
            if depth == 0:
                return doc[tag_end + 1 : nxt_close]


def clean(fragment):
    text = re.sub(r"<(script|style)[^>]*>.*?</\1>", " ", fragment, flags=re.S | re.I)
    text = re.sub(r"<br\s*/?>|</p>|</div>|</li>|</h\d>", "\n", text, flags=re.I)
    text = re.sub(r"<[^>]+>", " ", text)
    text = html.unescape(text)
    lines = [re.sub(r"[ \t\u3000]+", " ", ln).strip() for ln in text.splitlines()]
    return "\n".join(ln for ln in lines if ln)


def known_ids():
    """article ids already archived by external/fetch_jkb.py (never rewritten)."""
    out = set()
    if os.path.isdir(PREV_RAW):
        for f in os.listdir(PREV_RAW):
            m = re.match(r"jkb_(\d+)_", f)
            if m:
                out.add(m.group(1))
    if os.path.isdir(RAW):
        for f in os.listdir(RAW):
            m = re.match(r"jkbmore_(\d+)_", f)
            if m:
                out.add(m.group(1))
    return out


def harvest(chan):
    """{id: {title, date, url}} from list pages /{chan}/index.html + /{chan}/<N>.html."""
    items = {}
    for page in range(1, MAX_LIST_PAGES + 1):
        url = f"{BASE}/{chan}/" if page == 1 else f"{BASE}/{chan}/{page}.html"
        doc = polite_get(url)
        if doc is None:
            break
        m = re.search(r'id="ttde_data"[^>]*>\s*(\[.*?\])\s*</div>', doc, re.S)
        if not m:
            break
        try:
            data = json.loads(m.group(1))
        except json.JSONDecodeError:
            break
        for it in data:
            aid, rel = str(it.get("id", "")), it.get("url") or ""
            if not aid or aid in items or not rel:
                continue
            upd = it.get("update_time") or 0
            items[aid] = {
                "title": (it.get("title") or "").strip(),
                "date": time.strftime("%Y-%m-%d", time.localtime(upd)) if upd else "",
                "url": f"{BASE}/{chan}/{rel}",
            }
        if len(data) < 5:
            break
        if _state["blocked"]:
            break
    return items


def slug(title):
    t = re.sub(r"[^\w\u4e00-\u9fff]+", "_", title)[:40].strip("_")
    return t or "untitled"


def detail_meta(doc, meta):
    """byline / paper edition date from #ttde_info JSON + author spans."""
    info = ""
    mi = re.search(r'id="ttde_info"(.*?)</div>', doc, re.S)
    if mi:
        mj = re.search(r"\{.*\}", mi.group(0), re.S)
        if mj:
            try:
                raw = json.loads(mj.group(0))
                info = clean(raw.get("condition") or "").strip()
                if raw.get("crtime"):
                    info = f"{info} {raw['crtime']}".strip()
            except json.JSONDecodeError:
                info = clean(mi.group(1)).replace("\n", " ")[:200]
        else:
            info = clean(mi.group(1)).replace("\n", " ")[:200]
    authors = " ".join(
        clean(x).strip()
        for x in re.findall(
            r'<span[^>]*class="[^"]*(?:author|zuozhe)[^"]*"[^>]*>(.*?)</span>', doc, re.S
        )[:2]
    )
    if authors:
        info = f"{authors} | {info}".strip(" |")
    return info.strip(" |")


def fetch_detail(job, chan_name):
    aid, meta = job
    doc = polite_get(meta["url"])
    if doc is None:
        return None, "unreachable"
    m = extract_block(doc, 'id="ttde_con"') or ""
    body = clean(m)
    if not body:
        alt = re.search(
            r'<div[^>]*class="[^"]*(?:content|detail)[^"]*"[^>]*>(.*?)</div>', doc, re.S
        )
        body = clean(alt.group(1)) if alt else ""
    n_cn = len(re.findall(r"[\u4e00-\u9fff]", body))
    if n_cn < MIN_CN:
        return None, "short"
    info = detail_meta(doc, meta)
    title = meta["title"] or "untitled"
    fname = f"jkbmore_{aid}_{slug(title)}.txt"
    pub = "健康报（国家卫生健康委员会主管）· %s" % chan_name
    if info:
        pub = f"{pub} | {info}"
    date = meta["date"] or ""
    head = f"# URL: {meta['url']}\n# 标题: {title}\n# 发布方/日期: {pub} {date}\n\n"
    with open(os.path.join(RAW, fname), "w", encoding="utf-8") as fh:
        fh.write(head + body + "\n")
    return f"{fname} | {title} | {meta['url']} | {pub} | {date}", "ok"


def main():
    os.makedirs(RAW, exist_ok=True)
    limit = NEW_CAP
    if "-limit" in sys.argv:
        limit = int(sys.argv[sys.argv.index("-limit") + 1])
    if "-pages" in sys.argv:
        global MAX_LIST_PAGES
        MAX_LIST_PAGES = int(sys.argv[sys.argv.index("-pages") + 1])

    done = known_ids()
    manifest_path = os.path.join(RAW, "MANIFEST.txt")
    rows = []
    skipped = len(done)
    dropped_short = 0
    written = 0

    for chan, chan_name in CHANNELS.items():
        if written >= limit or _state["blocked"]:
            break
        items = harvest(chan)
        print(f"[{chan_name}] listed: {len(items)}", file=sys.stderr)
        todo = [(a, m) for a, m in items.items() if a not in done]
        todo.sort(key=lambda j: int(j[0]), reverse=True)  # newest first
        print(f"[{chan_name}] new (not in jkb_health): {len(todo)}", file=sys.stderr)
        for job in todo:
            if written >= limit or _state["blocked"]:
                break
            row, status = fetch_detail(job, chan_name)
            done.add(job[0])
            if row:
                rows.append(row)
                written += 1
            elif status == "short":
                dropped_short += 1

    header = (
        "# external/jkb_more/raw/MANIFEST.txt\n"
        "# 健康报网 (jkb.com.cn) 续抓批次 — 纯文字素材 (source-fetching only)\n"
        "# 格式: 文件名 | 标题 | 直链 | 发布机构 | 日期\n"
        "#\n"
    )
    existing = ""
    if os.path.exists(manifest_path):
        with open(manifest_path, encoding="utf-8") as fh:
            existing = "".join(
                ln for ln in fh if not ln.startswith("# 去重跳过") and not ln.startswith("# 已排除")
                and not ln.startswith("# 不可达")
            )
    if not existing.strip():
        existing = header
    with open(manifest_path, "w", encoding="utf-8") as fh:
        fh.write(header if not existing.startswith("#") else "")
        body = existing + "\n" if existing and not existing.endswith("\n") else existing
        fh.write(body)
        for r in rows:
            fh.write(r + "\n")
        fh.write(f"# 去重跳过: {skipped} (external/jkb_health 已有 id)\n")
        fh.write(f"# 已排除: {dropped_short} 篇正文不足 {MIN_CN} 中文字\n")
        if _state["blocked"]:
            fh.write("# 不可达: 521 限速再次触发，本轮提前停止\n")

    print(
        f"DONE jkb_more: +{written} new, {dropped_short} short-dropped, "
        f"{skipped} deduped, blocked={_state['blocked']}",
        file=sys.stderr,
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
