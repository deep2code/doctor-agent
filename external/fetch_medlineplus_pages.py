#!/usr/bin/env python3
"""Fetch MedlinePlus page text (English topics) to external/medlineplus/pages/{slug}.md"""
import json, re, sys, time, urllib.request
from pathlib import Path
from html.parser import HTMLParser

BASE = Path(__file__).parent
PAGES = BASE / "medlineplus" / "pages"
TOPICS = BASE / "medlineplus" / "topics.json"
UA = {"User-Agent": "Mozilla/5.0 (research; contact: self)"}

class TextExtractor(HTMLParser):
    def __init__(self):
        super().__init__()
        self.parts, self.skip, self.in_article = [], 0, False
    def handle_starttag(self, tag, attrs):
        d = dict(attrs)
        if tag == "article":
            self.in_article = True
        if self.in_article and tag in ("script","style","nav"):
            self.skip += 1
    def handle_endtag(self, tag):
        if tag in ("script","style","nav") and self.skip: self.skip -= 1
        if tag == "article": self.in_article = False
    def handle_data(self, data):
        if self.in_article and not self.skip:
            t = data.strip()
            if t: self.parts.append(t)

def get(url):
    req = urllib.request.Request(url, headers=UA)
    for i in range(3):
        try:
            with urllib.request.urlopen(req, timeout=30) as r:
                return r.read().decode("utf-8","replace")
        except Exception:
            if i == 2: return ""
            time.sleep(2)

def main():
    PAGES.mkdir(parents=True, exist_ok=True)
    topics = [t for t in json.load(open(TOPICS)) if t["language"]=="English" and t["url"]]
    print(f"{len(topics)} English topics", file=sys.stderr)
    ok = 0
    for i, t in enumerate(topics, 1):
        slug = t["url"].rstrip("/").split("/")[-1].replace(".html","")
        out = PAGES / f"{slug}.md"
        if out.exists():
            ok += 1; continue
        html = get(t["url"])
        if not html: continue
        ex = TextExtractor()
        ex.feed(html)
        text = "\n\n".join(ex.parts)
        if len(text) < 150:
            continue
        out.write_text(f"# {t['title']}\nURL: {t['url']}\n\n{text}\n")
        ok += 1
        if i % 50 == 0:
            print(f"[{i}/{len(topics)}] ok={ok}", file=sys.stderr)
        time.sleep(0.15)
    print(f"DONE: {ok} pages", file=sys.stderr)

if __name__ == "__main__":
    main()
