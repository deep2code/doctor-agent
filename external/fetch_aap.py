#!/usr/bin/env python3
"""Fetch AAP healthychildren.org articles (parenting encyclopedia, English)
into external/aap/pages/*.json.

URLs come from the site sitemap (UTF-16 XML). Scope: /English/ under
ages-stages, safety-prevention, family-life, tips-tools, healthy-living
(age-stage parenting topics first; health-issues overlaps MSD/MedlinePlus).
Body is extracted from #mainContent after "Page Content". Idempotent.

Usage: python3 fetch_aap.py [--limit N] [--sections a,b,c]
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
OUT_DIR = ROOT / "external" / "aap" / "pages"
UA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (doctor-agent knowledge fetch)"
SITEMAP = "https://www.healthychildren.org/sitemap.xml"
HOST = "https://www.healthychildren.org"

DEFAULT_SECTIONS = ["ages-stages", "safety-prevention", "family-life", "tips-tools", "healthy-living"]


def fetch(url: str, retries: int = 3) -> bytes | None:
    for i in range(retries + 1):
        try:
            req = urllib.request.Request(url, headers={"User-Agent": UA})
            with urllib.request.urlopen(req, timeout=30) as r:
                return r.read()
        except Exception as e:
            if i == retries:
                print(f"  FAIL {url}: {e}", file=sys.stderr)
                return None
            time.sleep(2)


def extract(html: str) -> tuple[str, str]:
    title = ""
    m = re.search(r"<h1[^>]*>(.*?)</h1>", html, re.S)
    if m:
        title = htmlmod.unescape(re.sub(r"<[^>]+>", "", m.group(1))).strip()
    i = html.find('id="mainContent"')
    if i < 0:
        return title, ""
    seg = html[i: i + 40000]
    # Cut at the trailing "Related Topics" block if present.
    cut = re.search(r"(Related Topics|Last Updated|Disclaimer|© Copyright)", seg)
    if cut:
        seg = seg[: cut.start()]
    text = re.sub(r"<script.*?</script>|<style.*?</style>", " ", seg, flags=re.S)
    text = re.sub(r"<[^>]+>", " ", text)
    text = htmlmod.unescape(text)
    text = re.sub(r"\s+", " ", text).strip()
    # Drop the breadcrumb prefix and "Page Content" marker.
    idx = text.find("Page Content")
    if idx >= 0:
        text = text[idx + len("Page Content"):].strip()
    return title, text


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--limit", type=int, default=500)
    ap.add_argument("--sections", default=",".join(DEFAULT_SECTIONS))
    args = ap.parse_args()
    sections = set(s.strip() for s in args.sections.split(",") if s.strip())
    OUT_DIR.mkdir(parents=True, exist_ok=True)

    data = fetch(SITEMAP)
    if not data:
        print("sitemap fetch failed", file=sys.stderr)
        return 1
    for enc in ("utf-16", "utf-8"):
        try:
            sm = data.decode(enc)
            break
        except UnicodeDecodeError:
            continue
    urls = re.findall(r"<loc>(.*?)</loc>", sm)
    wanted = []
    for u in urls:
        m = re.search(r"/English/([^/]+)/", u)
        if not m or m.group(1) not in sections:
            continue
        if not u.endswith(".aspx") or "default.aspx" in u:
            continue
        if "/Pages/" in u and u.endswith("/Pages/default.aspx"):
            continue
        wanted.append(u)
    print(f"sitemap urls: {len(urls)}, wanted: {len(wanted)}", file=sys.stderr)

    fetched = 0
    for u in wanted:
        if fetched >= args.limit:
            break
        slug = u.replace(HOST + "/English/", "").replace("/", "_").replace(".aspx", "")
        out = OUT_DIR / f"{slug}.json"
        if out.exists():
            fetched += 1
            continue
        html = fetch(u)
        if not html:
            continue
        txt = html.decode("utf-8", "replace")
        title, body = extract(txt)
        if len(body) < 200:
            continue
        out.write_text(json.dumps({"url": u, "title": title, "content": body},
                                  ensure_ascii=False, indent=1), encoding="utf-8")
        fetched += 1
        if fetched % 25 == 0:
            print(f"[{fetched}] {title[:50]}", file=sys.stderr)
        time.sleep(0.4)
    print(f"done: {fetched} pages")
    return 0


if __name__ == "__main__":
    sys.exit(main())
