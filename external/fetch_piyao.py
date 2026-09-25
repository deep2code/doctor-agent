#!/usr/bin/env python3
"""Bulk-download 科学辟谣 (piyao.kepuchina.cn) health refutation articles.

Pure download step: writes text extracts under external/piyao_health/raw/ plus an
index.tsv. Touches nothing under internal/knowledge/.

  python3 external/fetch_piyao.py            # full run over health categories
  python3 external/fetch_piyao.py -limit 5   # smoke test
"""
import html
import os
import re
import sys
import time
from urllib.parse import quote

from concurrent.futures import ThreadPoolExecutor

import requests

BASE = "https://piyao.kepuchina.cn"
OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "piyao_health")
RAW = os.path.join(OUT, "raw")

# category type ids worth having for a medical 科普 knowledge base
TYPES = {
    1: "疾病防治",
    2: "食品安全",
    15: "营养健康",
    16: "美容健身",
    6: "生活解惑",
    20: "心理学",
    7: "生物",
}
YEARS = [0]  # 0 = all years
MAX_PAGES = int(os.environ.get("PIYAO_MAX_PAGES", "80"))

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
            if r.status_code == 200 and r.text:
                return r.text
            if r.status_code in (404, 410):
                return None
        except Exception as e:  # noqa: BLE001
            print(f"  ! {url} {e}", file=sys.stderr)
        time.sleep(1.5 * (i + 1))
    return None


ITEM_RE = re.compile(
    r'href="https?://piyao\.kepuchina\.cn/rumor/rumordetail\?id=([A-Za-z0-9]+)"'
)


def harvest(t, year):
    """Return {id: (title, date)} from the paginated list fragment."""
    found = {}
    for page in range(1, MAX_PAGES + 1):
        url = f"{BASE}/rumor/rumorajaxlist?pageType=1&type={t}&year={year}&page={page}"
        doc = get(url)
        if doc is None:
            break
        ids = ITEM_RE.findall(doc)
        if not ids:
            break
        for m in re.finditer(
            r'rumordetail\?id=([A-Za-z0-9]+)"(.*?)rumor-list_item-title[^>]*>\s*([^<]+?)\s*</div>',
            doc,
            re.S,
        ):
            if m.group(1) not in found:
                found[m.group(1)] = (m.group(3).strip(), "")
        if len(ids) < 15:
            break
        time.sleep(0.3)
    return found


def clean(fragment):
    text = re.sub(r"<(script|style)[^>]*>.*?</\1>", " ", fragment, flags=re.S | re.I)
    text = re.sub(r"<br\s*/?>|</p>|</div>|</li>", "\n", text, flags=re.I)
    text = re.sub(r"<[^>]+>", " ", text)
    text = html.unescape(text)
    lines = [re.sub(r"[ \t\u3000]+", " ", ln).strip() for ln in text.splitlines()]
    return "\n".join(ln for ln in lines if ln)


def slug(title):
    t = re.sub(r"[^\w\u4e00-\u9fff]+", "_", title)[:40].strip("_")
    return t or "untitled"


def fetch_detail(rid, hint):
    doc = get(f"{BASE}/rumor/rumordetail?id={rid}")
    if doc is None:
        return None
    m = re.search(r'<div class="rumor-title">(.*?)</div>', doc, re.S)
    title = clean(m.group(1)) if m else hint
    m = re.search(r'<div class="rumor-info">(.*?)</div>', doc, re.S)
    info = clean(m.group(1)) if m else ""
    m = re.search(r'<div class="rumor-content"(.*?)</div>\s*<div class="content_publish-tips"',
                  doc, re.S)
    if not m:
        m = re.search(r'<div class="rumor-content"(.*)', doc, re.S)
    body = clean(m.group(1)) if m else ""
    body = re.sub(r"\s*\d+\s*科普中国", " 科普中国", body)
    body = body.split("分享到：")[0]
    body = body.replace("图库版权图片，转载使用可能引发版权纠纷", "").strip()
    m2 = re.search(r"所属分类：\s*([^\n]+)", body)
    keyword = m2.group(1).strip() if m2 else ""
    m3 = re.search(r"(作者[丨|].*?)\n审核[丨|](.*?)(?=\n|本文由)", body, re.S)
    credit = ""
    if m3:
        credit = f"作者: {m3.group(1).strip()} | 审核: {m3.group(2).strip()}"
    m4 = re.search(r"时间：(\d{4}-\d{2}-\d{2})", body)
    if m4:
        credit = (credit + " | " if credit else "") + f"发布: {m4.group(1)}"
    return {
        "id": rid,
        "title": title.strip(),
        "info": info.strip(),
        "keyword": keyword.strip(),
        "credit": credit,
        "body": body.strip(),
        "url": f"{BASE}/rumor/rumordetail?id={rid}",
    }


def main():
    os.makedirs(RAW, exist_ok=True)
    limit = 0
    if "-limit" in sys.argv:
        limit = int(sys.argv[sys.argv.index("-limit") + 1])

    index_path = os.path.join(OUT, "index.tsv")
    existing = set(os.listdir(RAW))
    index_rows = []
    if os.path.exists(index_path):
        with open(index_path, encoding="utf-8") as fh:
            index_rows = [ln.rstrip("\n") for ln in fh if ln.strip()]
    seen_urls = {r.split("\t")[-1] for r in index_rows if r.count("\t") >= 3}

    written = 0
    for t, name in TYPES.items():
        items = harvest(t, 0)
        print(f"[{name}] list items: {len(items)}", file=sys.stderr)
        todo = []
        for rid, (title, _) in items.items():
            url = f"{BASE}/rumor/rumordetail?id={rid}"
            if url in seen_urls:
                continue
            fname = f"py{t}_{rid}_{slug(title)}.txt"
            if fname in existing:
                continue
            todo.append((rid, title, fname, url))
        if limit:
            todo = todo[: max(0, limit - written)]

        def work(job):
            rid, title, fname, url = job
            art = fetch_detail(rid, title)
            if not art or len(art["body"]) < 120:
                return None
            head = (
                f"来源: 科学辟谣平台（中国科协/中央网信办）· 辟谣文章「{art['title']}」\n"
                f"分类: {name}\n"
                + (f"分类明细: {art['keyword']}\n"
                   if art["keyword"] and art["keyword"] != name else "")
                + (f"{art['credit']}\n" if art["credit"] else "")
                + f"URL: {url}\n"
                f"抓取方式: 直连 + 正文提取，未改写\n"
                f"{'=' * 60}\n"
            )
            with open(os.path.join(RAW, fname), "w", encoding="utf-8") as fh:
                fh.write(head + art["body"] + "\n")
            return "\t".join([fname, name, art["title"], url])

        with ThreadPoolExecutor(max_workers=8) as pool:
            for row in pool.map(work, todo):
                if row:
                    index_rows.append(row)
                    seen_urls.add(row.split("\t")[-1])
                    written += 1
                if written and written % 50 == 0:
                    with open(index_path, "w", encoding="utf-8") as fh:
                        fh.write("\n".join(index_rows) + "\n")
                    print(f"  .. {written} written", file=sys.stderr)
        if limit and written >= limit:
            break

    with open(index_path, "w", encoding="utf-8") as fh:
        fh.write("\n".join(index_rows) + "\n")
    print(f"DONE piyao: +{written} new files, {len(index_rows)} indexed", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
