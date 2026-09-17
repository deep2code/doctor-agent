#!/usr/bin/env python3
"""Fetch China CDC (中国疾控中心) health-education articles.

Column pages list articles as relative links like
`./202608/t2026XXXX_XXXX.html`; article bodies live in
`<div class="trs_editor_view ...">`. Idempotent: skips existing files.

  python3 external/fetch_cdc.py                 # all configured columns
  python3 external/fetch_cdc.py --columns jkts  # one column
  python3 external/fetch_cdc.py --pages 3       # up to 3 list pages per column

Output: external/cdc/{column}/{yyyymm}_{articleId}.txt
  first line = title, blank line, then the article body (UTF-8).
"""
import argparse
import html as htmlmod
import re
import sys
import time
import urllib.request
from pathlib import Path

OUT = Path(__file__).parent / "cdc"
UA = {"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
                    "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"}

COLUMNS = {
    # slug: (中文名, 栏目列表 URL)
    "jkts": ("健康提示", "https://www.chinacdc.cn/jkts/"),
    # Other columns (jkkp/crb 传染病, jkkp/mxfcrb 慢病, jkyj/...) render their
    # article lists via JS and need a browser; add their list URLs here once
    # a working entry page is found.
}

ARTICLE_RE = re.compile(r"href=['\"]\./(\d{6})/(t\d+_\d+\.html)['\"]")
TITLE_RES = [
    re.compile(r"<h1[^>]*>(.*?)</h1>", re.S),
    re.compile(r'class="[^"]*(?:wzTit|artTitle|title)[^"]*"[^>]*>\s*([^<]{5,120})'),
    re.compile(r"<title>([^<]{5,120})</title>"),
]


def get(url: str, timeout: int = 30) -> str:
    req = urllib.request.Request(url, headers=UA)
    for i in range(3):
        try:
            with urllib.request.urlopen(req, timeout=timeout) as r:
                return r.read().decode("utf-8", "replace")
        except Exception as e:
            if i == 2:
                raise
            time.sleep(1.5 * (i + 1))


def list_articles(list_url: str) -> list[tuple[str, str]]:
    """Return [(title_hint, abs_url), ...] parsed from a column list page."""
    html = get(list_url)
    found = []
    for yyyymm, fname in ARTICLE_RE.findall(html):
        abs_url = list_url.rstrip("/") + "/" + yyyymm + "/" + fname
        found.append((fname, abs_url))
    # dedupe, keep order
    return list(dict.fromkeys(found))


def extract_article(html: str) -> tuple[str, str]:
    """Return (title, body_text) from an article page."""
    m = re.search(r'<div class="trs_editor_view[^"]*"[^>]*>(.*?)</div>\s*(?:<div class="wzFooter|$)',
                  html, re.S)
    if not m:
        m = re.search(r'<div class="trs_editor_view[^"]*"[^>]*>(.*)', html, re.S)
    body = m.group(1) if m else ""
    end = body.find("wzFooter")
    if end > 0:
        body = body[:end]
    body = re.sub(r"<script.*?</script>", " ", body, flags=re.S)
    body = re.sub(r"<style.*?</style>", " ", body, flags=re.S)
    body = re.sub(r"<[^>]+>", " ", body)
    body = htmlmod.unescape(body)
    body = re.sub(r"[ \t\u3000]+", " ", body)
    body = re.sub(r"\n\s*\n+", "\n\n", body).strip()

    # The article title is a <h5><a>…</a></h5> before #articleCon.
    title = ""
    m = re.search(r'id="articleCon"', html)
    pre = html[max(0, m.start() - 4000): m.start()] if m else html[:4000]
    for r in (re.compile(r"<h[1-6][^>]*>\s*<a[^>]*>(.*?)</a>\s*</h[1-6]>", re.S),
              re.compile(r"<h[1-6][^>]*>(.*?)</h[1-6]>", re.S)):
        mm = r.search(pre)
        if mm:
            title = re.sub(r"<[^>]+>", "", mm.group(1)).strip()
            if title:
                break
    if not title:
        m2 = re.search(r"<title>([^<]{5,120})</title>", html)
        if m2 and "疾控中心" not in m2.group(1)[:6]:
            title = m2.group(1)
    if not title:
        title = body.split("\n")[0][:60] if body else "untitled"
    return title, body


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--columns", nargs="*", default=list(COLUMNS))
    parser.add_argument("--pages", type=int, default=2)
    parser.add_argument("--refresh", action="store_true",
                        help="re-fetch existing files")
    args = parser.parse_args()

    total_ok, total_skip, fails = 0, 0, []
    for slug in args.columns:
        if slug not in COLUMNS:
            print(f"未知栏目 {slug}，可选: {list(COLUMNS)}", file=sys.stderr)
            continue
        name, list_url = COLUMNS[slug]
        col_dir = OUT / slug
        col_dir.mkdir(parents=True, exist_ok=True)
        seen = set()
        for page in range(1, args.pages + 1):
            url = list_url if page == 1 else f"{list_url}index_{page}.html"
            try:
                arts = list_articles(url)
            except Exception as e:
                print(f"[{slug}] 列表页 {page} 失败: {e}", file=sys.stderr)
                break
            if not arts:
                break  # no more pages
            for fname, abs_url in arts:
                if fname in seen:
                    continue
                seen.add(fname)
                out = col_dir / f"{fname[:-5]}.txt"
                if out.exists() and not args.refresh:
                    total_skip += 1
                    continue
                try:
                    html = get(abs_url)
                    title, body = extract_article(html)
                    if len(body) < 300:
                        fails.append((fname, "正文过短"))
                        continue
                    out.write_text(f"{title}\n\n{body}\n", encoding="utf-8")
                    total_ok += 1
                    print(f"  ✅ {slug}/{fname} ({len(body)} 字) {title[:30]}",
                          file=sys.stderr)
                except Exception as e:
                    fails.append((fname, str(e)[:80]))
                time.sleep(0.2)
            print(f"[{slug}] 页 {page}: 累计 {len(seen)} 篇", file=sys.stderr)
    print(f"DONE: 新增 {total_ok}, 跳过 {total_skip}, 失败 {len(fails)}",
          file=sys.stderr)
    if fails:
        print("失败示例:", fails[:10], file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
