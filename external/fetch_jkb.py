#!/usr/bin/env python3
"""Bulk-download 健康报网 (jkb.com.cn, 国家卫健委主管) 科普/中医/医药视界 articles.

Pure download step: writes text extracts under external/jkb_health/raw/ plus
index.tsv. Touches nothing under internal/knowledge/.

  python3 external/fetch_jkb.py            # all channels
  python3 external/fetch_jkb.py -limit 5   # smoke test
"""
import html
import json
import os
import re
import sys
import time
from concurrent.futures import ThreadPoolExecutor

import requests

BASE = "https://www.jkb.com.cn"
OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "jkb_health")
RAW = os.path.join(OUT, "raw")

CHANNELS = {
    "science": "医学科普",
    "TCM": "中医中药",
    "horizon": "医药视界",
}
MAX_LIST_PAGES = 60

UA = {
    "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
    "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
}

s = requests.Session()
s.headers.update(UA)


def get(url, tries=3):
    for i in range(tries):
        try:
            r = s.get(url, timeout=25)
            r.encoding = "utf-8"
            if r.status_code == 200 and r.content:
                return r.text
            if r.status_code in (404, 410):
                return None
        except Exception as e:  # noqa: BLE001
            print(f"  ! {url} {e}", file=sys.stderr)
        time.sleep(1.5 * (i + 1))
    return None


def clean(fragment):
    text = re.sub(r"<(script|style)[^>]*>.*?</\1>", " ", fragment, flags=re.S | re.I)
    text = re.sub(r"<br\s*/?>|</p>|</div>|</li>|</h\d>", "\n", text, flags=re.I)
    text = re.sub(r"<[^>]+>", " ", text)
    text = html.unescape(text)
    lines = [re.sub(r"[ \t\u3000]+", " ", ln).strip() for ln in text.splitlines()]
    return "\n".join(ln for ln in lines if ln)


def harvest(chan):
    """Return {id: {title, date, url}} from the server-rendered #ttde_data JSON."""
    out = {}
    for page in range(1, MAX_LIST_PAGES + 1):
        url = f"{BASE}/{chan}/" if page == 1 else f"{BASE}/{chan}/{page}.html"
        doc = get(url)
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
            if not aid or aid in out or not rel:
                continue
            upd = it.get("update_time") or 0
            out[aid] = {
                "title": (it.get("title") or "").strip(),
                "date": time.strftime("%Y-%m-%d", time.localtime(upd)) if upd else "",
                "url": f"{BASE}/{chan}/{rel}",
            }
        if len(data) < 5:
            break
    return out


def slug(title):
    t = re.sub(r"[^\w\u4e00-\u9fff]+", "_", title)[:40].strip("_")
    return t or "untitled"


def fetch_detail(job, chan_name):
    aid, meta = job
    if not meta["url"]:
        return None
    doc = get(meta["url"])
    if doc is None:
        return None
    m = re.search(r'id="ttde_con"(.*?)</div>', doc, re.S)
    if not m:
        m = re.search(r'<div[^>]*class="[^"]*(?:content|detail)[^"]*"[^>]*>(.*?)</div>', doc, re.S)
    body = clean(m.group(1)) if m else ""
    if len(body) < 120:
        return None
    info = ""
    mi = re.search(r'id="ttde_info"(.*?)</div>', doc, re.S)
    if mi:
        mj = re.search(r"\{.*\}", mi.group(0), re.S)
        if mj:
            try:
                meta_json = json.loads(mj.group(0))
                info = clean(meta_json.get("condition") or "").strip()
                if meta_json.get("crtime"):
                    info = f"{info} {meta_json['crtime']}".strip()
                authors = " ".join(
                    clean(x).strip()
                    for x in re.findall(
                        r'<span[^>]*class="[^"]*(?:author|zuozhe)[^"]*"[^>]*>(.*?)</span>',
                        doc, re.S) [: 2]
                )
                if authors:
                    info = f"{authors} | {info}".strip(" |")
            except json.JSONDecodeError:
                info = ""
        if not info:
            info = clean(mi.group(1)).replace("\n", " ")[:200]
    title = meta["title"] or (
        re.search(r"<title>(.*?)</title>", doc, re.S).group(1).split("－")[0].strip()
        if "<title" in doc else "untitled"
    )
    fname = f"jkb_{aid}_{slug(title)}.txt"
    head = (
        f"来源: 健康报（国家卫生健康委员会主管）· {chan_name}\n"
        f"标题: {title}\n"
        f"发布: {meta['date']}"
        + (f" | {info}" if info else "")
        + "\n"
        f"URL: {meta['url']}\n"
        f"抓取方式: 直连 + 正文提取，未改写\n"
        f"{'=' * 60}\n"
    )
    with open(os.path.join(RAW, fname), "w", encoding="utf-8") as fh:
        fh.write(head + body + "\n")
    return "\t".join([fname, chan_name, title, meta["url"]])


def main():
    os.makedirs(RAW, exist_ok=True)
    limit = 0
    if "-limit" in sys.argv:
        limit = int(sys.argv[sys.argv.index("-limit") + 1])

    index_path = os.path.join(OUT, "index.tsv")
    index_rows = []
    if os.path.exists(index_path):
        with open(index_path, encoding="utf-8") as fh:
            index_rows = [ln.rstrip("\n") for ln in fh if ln.strip()]
    done_urls = {r.split("\t")[-1] for r in index_rows if r.count("\t") >= 3}
    on_disk = set(os.listdir(RAW))

    written = 0
    for chan, chan_name in CHANNELS.items():
        items = harvest(chan)
        print(f"[{chan_name}] list items: {len(items)}", file=sys.stderr)
        todo = [
            (aid, meta)
            for aid, meta in items.items()
            if meta["url"] not in done_urls
            and not any(f.startswith(f"jkb_{aid}_") for f in on_disk)
        ]
        if limit:
            todo = todo[: max(0, limit - written)]
        with ThreadPoolExecutor(max_workers=6) as pool:
            for row in pool.map(lambda j: fetch_detail(j, chan_name), todo):
                if row:
                    index_rows.append(row)
                    done_urls.add(row.split("\t")[-1])
                    written += 1
                    if written % 50 == 0:
                        with open(index_path, "w", encoding="utf-8") as fh:
                            fh.write("\n".join(index_rows) + "\n")
                        print(f"  .. {written} written", file=sys.stderr)
        if limit and written >= limit:
            break

    with open(index_path, "w", encoding="utf-8") as fh:
        fh.write("\n".join(index_rows) + "\n")
    print(f"DONE jkb: +{written} new files, {len(index_rows)} indexed", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
