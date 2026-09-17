#!/usr/bin/env python3
"""中国疾控中心 健康科普栏目 (JS 渲染列表) → external/cdc/<col>/*.txt →
internal/knowledge/data/corpus_cdc.json (CorpusDoc, 零 Go 改动).

列表页 JS 渲染, 用 playwright 取链接; 文章页静态, 用 urllib 取正文.
列: jkkp/crb 传染病 | mxfcrb 慢病 | mygh 母婴 | hjjk 环境健康 |
    ggws 公共卫生 | fsws 消毒 | jkts 健康提示(补页)

用法: python3 fetch_cdc_cols.py
"""
import asyncio
import json
import re
import sys
import time
import urllib.request
from pathlib import Path

BASE = Path(__file__).parent
OUT = BASE / "cdc"
UA = {"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
                    "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"}

# slug -> (栏目名, 列表 URL 前缀)
COLUMNS = {
    "crb":    ("传染病", "https://www.chinacdc.cn/jkkp/crb/"),
    "mxfcrb": ("慢性病", "https://www.chinacdc.cn/jkkp/mxfcrb/"),
    "mygh":   ("母婴健康", "https://www.chinacdc.cn/jkkp/mygh/"),
    "hjjk":   ("环境健康", "https://www.chinacdc.cn/jkkp/hjjk/"),
    "ggws":   ("公共卫生", "https://www.chinacdc.cn/jkkp/ggws/"),
    "fsws":   ("消毒", "https://www.chinacdc.cn/jkkp/fsws/"),
    "jkts":   ("健康提示", "https://www.chinacdc.cn/jkts/"),
}
PAGES_PER_COL = 3  # index_N.shtml 尝试页数

ARTICLE_RE = re.compile(r'href="(\.{1,2}/(?:\d{6}/)?t\d+_\d+\.html)"')
TITLE_RES = [
    re.compile(r"<h1[^>]*>(.*?)</h1>", re.S),
    re.compile(r'<meta name="ArticleTitle" content="([^"]{5,120})"'),
    re.compile(r"<title>([^<]{5,120})</title>"),
]
BODY_RE = re.compile(
    r'<div[^>]*class="[^"]*(?:trs_editor_view|TRS_Editor|wzcon|article)[^"]*"[^>]*>(.*?)</div>\s*(?:<div class="(?:footer|next|page)|<script)',
    re.S)


def http_get(url: str, timeout: int = 30) -> str:
    req = urllib.request.Request(url, headers=UA)
    for i in range(3):
        try:
            with urllib.request.urlopen(req, timeout=timeout) as r:
                return r.read().decode("utf-8", "replace")
        except Exception:
            if i == 2:
                raise
            time.sleep(1.5 * (i + 1))
    return ""


def strip_html(s: str) -> str:
    s = re.sub(r"<script[^>]*>.*?</script>", "", s, flags=re.S)
    s = re.sub(r"<style[^>]*>.*?</style>", "", s, flags=re.S)
    import html as htmlmod
    s = htmlmod.unescape(re.sub(r"<[^>]+>", "\n", s))
    lines = [l.strip() for l in s.split("\n") if l.strip()]
    return "\n".join(lines)


async def harvest_lists() -> dict[str, tuple[str, str, str]]:
    """playwright 渲染列表页, 返回 {abs_url: (col_slug, col_name, page_url)}."""
    from playwright.async_api import async_playwright
    found: dict[str, tuple[str, str, str]] = {}
    async with async_playwright() as pw:
        b = await pw.chromium.launch(headless=True)
        ctx = await b.new_context(user_agent=UA["User-Agent"])
        p = await ctx.new_page()
        for slug, (name, base) in COLUMNS.items():
            for page_i in range(PAGES_PER_COL):
                url = base if page_i == 0 else f"{base}index_{page_i}.shtml"
                try:
                    await p.goto(url, wait_until="domcontentloaded", timeout=45000)
                    await p.wait_for_timeout(3500)
                except Exception as e:
                    print(f"  [{slug}] p{page_i}: {str(e)[:60]}", file=sys.stderr)
                    break
                html = await p.content()
                arts = set()
                for rel in ARTICLE_RE.findall(html):
                    parts = rel.replace("../", "").split("/")
                    if len(parts) == 2:
                        abs_url = base + parts[1]
                    else:
                        abs_url = base + "/".join(parts[1:] if parts[0] == "." else parts)
                    arts.add(abs_url)
                new = 0
                for a in arts:
                    if a not in found:
                        found[a] = (slug, name, url)
                        new += 1
                print(f"  [{slug}] {url} : {new} 篇 (累计 {len(found)})", file=sys.stderr)
                if page_i > 0 and new == 0:
                    break
        await b.close()
    return found


def fetch_article(url: str) -> str | None:
    try:
        html = http_get(url)
    except Exception as e:
        print(f"  ⚠️ {url[-50:]}: {str(e)[:50]}", file=sys.stderr)
        return None
    body_m = BODY_RE.search(html)
    body = strip_html(body_m.group(1)) if body_m else ""
    if len(body) < 200:
        body = strip_html(html)
    if len(body) < 200:
        return None
    title = ""
    for r in TITLE_RES:
        m = r.search(html)
        if m:
            title = strip_html(m.group(1)).strip()
            if title:
                break
    return title + "\n\n" + body


def main() -> int:
    OUT.mkdir(parents=True, exist_ok=True)
    found = asyncio.run(harvest_lists())
    print(f"共 {len(found)} 篇候选", file=sys.stderr)

    ok = skip = 0
    arts = []
    for url, (slug, name, page_url) in sorted(found.items()):
        article_id = re.sub(r"[./]", "", url)[-20:]
        out = OUT / slug / f"{article_id}.txt"
        if out.exists():
            skip += 1
            arts.append((out, url, slug, name))
            continue
        text = fetch_article(url)
        if not text:
            continue
        out.parent.mkdir(parents=True, exist_ok=True)
        out.write_text(text, encoding="utf-8")
        arts.append((out, url, slug, name))
        ok += 1
        time.sleep(0.4)
    print(f"新抓 {ok}, 已有 {skip}", file=sys.stderr)

    # 转换: 全量 (新+旧) → corpus_cdc.json
    docs = []
    for out, url, slug, name in arts:
        text = out.read_text(encoding="utf-8")
        title = text.split("\n", 1)[0].strip()
        docs.append({
            "source": "cdc_kp",
            "id": "cdc_kp-" + out.stem,
            "lang": "zh",
            "title": title if title else name,
            "kind": "topic",
            "url": url,
            "keywords": [name],
            "body": text[:24000],
        })
    data = BASE.parent / "internal" / "knowledge" / "data" / "corpus_cdc.json"
    data.write_text(json.dumps({
        "source": "cdc_kp",
        "updated": "2026-09",
        "entries": docs,
    }, ensure_ascii=False), encoding="utf-8")
    print(f"wrote {data} ({len(docs)} docs)", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
