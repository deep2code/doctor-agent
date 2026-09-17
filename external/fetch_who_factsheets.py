#!/usr/bin/env python3
"""Batch-download WHO fact sheets (English) to external/who_factsheets/*.md"""
import re, sys, time, urllib.request
from pathlib import Path
from html.parser import HTMLParser

OUT = Path(__file__).parent / "who_factsheets"
INDEX = "https://www.who.int/news-room/fact-sheets"
UA = {"User-Agent": "Mozilla/5.0 (research; contact: self)"}

class TextExtractor(HTMLParser):
    def __init__(self):
        super().__init__()
        self.parts = []
        self.skip = 0
        self.in_article = False
        self.article_depth = 0
    def handle_starttag(self, tag, attrs):
        # WHO fact-sheet body lives in <article class="sf-detail-body-wrapper">,
        # NOT inside <main>; grab all text within the first <article>.
        if tag == "article":
            if self.article_depth == 0:
                self.in_article = True
            self.article_depth += 1
        if self.in_article and tag in ("script", "style", "nav"):
            self.skip += 1
    def handle_endtag(self, tag):
        if tag in ("script", "style", "nav") and self.skip:
            self.skip -= 1
        if tag == "article":
            self.article_depth -= 1
            if self.article_depth == 0:
                self.in_article = False
    def handle_data(self, data):
        if self.in_article and not self.skip:
            t = data.strip()
            if t:
                self.parts.append(t)

def get(url):
    req = urllib.request.Request(url, headers=UA)
    for i in range(3):
        try:
            with urllib.request.urlopen(req, timeout=30) as r:
                return r.read().decode("utf-8", "replace")
        except Exception as e:
            if i == 2: raise
            time.sleep(2)

def main():
    OUT.mkdir(parents=True, exist_ok=True)
    html = get(INDEX)
    links = sorted(set(re.findall(r'href="(/news-room/fact-sheets/detail/[^"#?]+)"', html)))
    print(f"发现 {len(links)} 个 fact sheet", file=sys.stderr)
    ok = 0
    for i, link in enumerate(links, 1):
        slug = link.rstrip("/").split("/")[-1]
        out = OUT / f"{slug}.md"
        if out.exists():
            ok += 1
            continue
        try:
            page = get("https://www.who.int" + link)
            ex = TextExtractor()
            ex.feed(page)
            text = "\n\n".join(ex.parts)
            if len(text) < 200:
                continue
            out.write_text(f"# {slug}\n\n{text}\n")
            ok += 1
            print(f"[{i}/{len(links)}] {slug} ({len(text)} chars)", file=sys.stderr)
        except Exception as e:
            print(f"[{i}/{len(links)}] FAIL {slug}: {e}", file=sys.stderr)
        time.sleep(0.2)
    print(f"DONE: {ok} fact sheets", file=sys.stderr)

if __name__ == "__main__":
    main()
