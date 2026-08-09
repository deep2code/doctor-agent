#!/usr/bin/env python3
"""Fetch the 香港卫生署 家庭健康服务 (Family Health Service, fhs.gov.hk)
简体中文育儿资讯 pages into external/fhs/pages/*.json.

Scope: fhs.gov.hk/sc_chi/ pages under the child-health / breastfeeding /
parenting-corner / child_health sections. BFS from seed URLs, same-host
sc_chi .html links only. Body text is extracted from #container-wrapper with
nav/breadcrumb/footer blocks stripped (FHS 2019 template keeps the article
text outside #content). Idempotent: already-fetched pages are skipped.

Usage: python3 fetch_fhs.py [--limit N]
"""
import argparse
import html as htmlmod
import json
import re
import sys
import time
import urllib.request
from pathlib import Path

ROOT = Path(__file__).parent.parent
OUT_DIR = ROOT / "external" / "fhs" / "pages"
HOST = "https://www.fhs.gov.hk"
UA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (doctor-agent knowledge fetch)"

SEEDS = [
    f"{HOST}/sc_chi/health_info/child/",
    f"{HOST}/sc_chi/breastfeeding/",
    f"{HOST}/sc_chi/parenting_corner/",
    f"{HOST}/sc_chi/parenting_corner/expert_tips/index.html",
]

URLS_FILE = ROOT / "external" / "fhs" / "urls.txt"

# URL substrings that are not article pages (booking, events, services, nav).
EXCLUDE = [
    "/news/", "/centre_det/", "/booking", "/talk_workshop", "timetable",
    "application", "/doc_br/", "/main_ser/", "index.html", "sitemap",
    "/other_languages/", "/wbw", "youtube", ".pdf", ".jpg", ".png", "mailto:",
    "javascript:", "#", "?",
]


def get(url: str, retries: int = 2) -> str | None:
    for i in range(retries + 1):
        try:
            req = urllib.request.Request(url, headers={"User-Agent": UA})
            with urllib.request.urlopen(req, timeout=25) as r:
                data = r.read()
            for enc in ("utf-8", "big5"):
                try:
                    return data.decode(enc)
                except UnicodeDecodeError:
                    continue
            return data.decode("utf-8", "replace")
        except Exception as e:
            if i == retries:
                print(f"  FAIL {url}: {e}", file=sys.stderr)
                return None
            time.sleep(2)


def extract(html: str) -> tuple[str, str]:
    """Return (title, body_text) from an FHS page."""
    title = ""
    m = re.search(r'<h[12][^>]*class="title"[^>]*>(.*?)</h[12]>', html, re.S)
    if not m:
        m = re.search(r"<h1[^>]*>(.*?)</h1>", html, re.S)
    if m:
        title = re.sub(r"<[^>]+>", "", m.group(1)).strip()

    # Take the main container, then drop nav/footer/side blocks.
    m = re.search(r'<main[^>]*>(.*?)</main>', html, re.S)
    body = m.group(1) if m else html
    body = re.sub(r"<script.*?</script>|<style.*?</style>", " ", body, flags=re.S)
    body = re.sub(
        r'<(?:nav|footer|header|aside)\b[^>]*>.*?</(?:nav|footer|header|aside)>',
        " ", body, flags=re.S)
    body = re.sub(
        r'<[^>]+(?:class|id)="[^"]*(?:breadcrumb|sub-nav|qrcode|related-sections|content-rev-date|d-print-none)[^"]*"[^>]*>.*?</[^>]+>',
        " ", body, flags=re.S)
    body = re.sub(r"<[^>]+>", " ", body)
    body = htmlmod.unescape(body)
    body = re.sub(r"\s+", " ", body).strip()
    return title, body


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--limit", type=int, default=600)
    args = ap.parse_args()
    OUT_DIR.mkdir(parents=True, exist_ok=True)

    queue = list(SEEDS)
    if URLS_FILE.exists():
        queue = [l.strip() for l in URLS_FILE.read_text(encoding="utf-8").splitlines()
                 if l.strip()] + queue
    seen = set()
    fetched = 0
    while queue and fetched < args.limit:
        url = queue.pop(0)
        if url in seen:
            continue
        seen.add(url)
        slug = url.replace(f"{HOST}/sc_chi/", "").rstrip("/")
        slug = slug.replace("/", "_")
        out = OUT_DIR / f"{slug}.json"
        if out.exists():
            fetched += 1
            continue
        html = get(url)
        if not html:
            continue
        title, body = extract(html)
        if not body:
            continue
        out.write_text(json.dumps({"url": url, "title": title, "content": body},
                                  ensure_ascii=False, indent=1), encoding="utf-8")
        fetched += 1
        print(f"[{fetched}] {title[:40] or url} ({len(body)} chars)")

        # Discover links: same-host /sc_chi/ .html pages.
        for m in re.finditer(r'href="([^"]+)"', html):
            u = m.group(1)
            if u.startswith("/"):
                u = HOST + u
            if not u.startswith(HOST + "/sc_chi/"):
                continue
            if not u.endswith(".html"):
                continue
            if any(x in u for x in EXCLUDE):
                continue
            if u not in seen and len(queue) < args.limit * 4:
                queue.append(u)
        time.sleep(0.5)
    print(f"done: {fetched} pages")
    return 0


if __name__ == "__main__":
    sys.exit(main())
