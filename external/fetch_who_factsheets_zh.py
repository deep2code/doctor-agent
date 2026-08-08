#!/usr/bin/env python3
"""Download official Chinese (zh) WHO fact sheets to external/who_factsheets_zh/*.md.

Idempotent: skips existing files. The English index page (already fetched by
fetch_who_factsheets.py) lists all slugs; each is fetched from the /zh/ locale.
Pages without a Chinese translation are skipped (their file is absent).
"""
import re, sys, time, urllib.request
from html.parser import HTMLParser
from pathlib import Path

OUT = Path(__file__).parent / "who_factsheets_zh"
EN_DIR = Path(__file__).parent / "who_factsheets"
UA = {"User-Agent": "Mozilla/5.0 (research; contact: self)"}

class TextExtractor(HTMLParser):
    """Grab the text inside <article class="sf-detail-body-wrapper"> (same
    structure in both locales)."""
    def __init__(self):
        super().__init__()
        self.parts = []
        self.skip = 0
        self.in_article = False
        self.depth = 0
    def handle_starttag(self, tag, attrs):
        if tag == "article":
            if self.depth == 0:
                self.in_article = True
            self.depth += 1
        if self.in_article and tag in ("script", "style", "nav"):
            self.skip += 1
    def handle_endtag(self, tag):
        if tag in ("script", "style", "nav") and self.skip:
            self.skip -= 1
        if tag == "article":
            self.depth -= 1
            if self.depth == 0:
                self.in_article = False
    def handle_data(self, data):
        if self.in_article and not self.skip:
            t = data.strip()
            if t:
                self.parts.append(t)

def extract(html):
    ex = TextExtractor()
    ex.feed(html)
    return "\n\n".join(ex.parts)

def get(url):
    req = urllib.request.Request(url, headers=UA)
    for i in range(3):
        try:
            with urllib.request.urlopen(req, timeout=30) as r:
                return r.read().decode("utf-8", "replace")
        except Exception as e:
            if i == 2:
                raise
            time.sleep(2)

def main():
    OUT.mkdir(parents=True, exist_ok=True)
    slugs = sorted(f.stem for f in EN_DIR.glob("*.md"))
    print(f"发现 {len(slugs)} 个英文 slug", file=sys.stderr)
    ok, missing = 0, []
    for i, slug in enumerate(slugs, 1):
        out = OUT / f"{slug}.md"
        if out.exists():
            ok += 1
            continue
        url = f"https://www.who.int/zh/news-room/fact-sheets/detail/{slug}"
        try:
            html = get(url)
            text = extract(html)
            if len(re.findall(r"[\u4e00-\u9fff]", text)) < 500:
                missing.append(slug)
                print(f"[{i}/{len(slugs)}] SKIP {slug} (无中文版)", file=sys.stderr)
                continue
            out.write_text(f"# {slug}\n\n{text}\n")
            ok += 1
            print(f"[{i}/{len(slugs)}] {slug} ({len(text)} chars)", file=sys.stderr)
        except Exception as e:
            missing.append(slug)
            print(f"[{i}/{len(slugs)}] FAIL {slug}: {e}", file=sys.stderr)
        time.sleep(0.2)
    print(f"DONE: {ok} 中文版, 缺失 {len(missing)}: {missing}", file=sys.stderr)

if __name__ == "__main__":
    main()
