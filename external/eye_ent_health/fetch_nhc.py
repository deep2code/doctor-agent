#!/usr/bin/env python3
"""Fetch nhc.gov.cn pages (JS-cookie WAF) with playwright-harvested cookies.

Usage: python3 fetch_nhc.py <out.txt> <url> [pdf|html]
Writes text of the notice body to <out.txt> (first line added by hand later).
"""
import http.cookiejar, os, re, sys, urllib.request
from pathlib import Path

CHROME = os.path.expanduser(
    "~/Library/Caches/ms-playwright/chromium-1234/chrome-mac-arm64/"
    "Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing")
UA = ("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 "
      "(KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
_cache = {}


def opener():
    if "op" in _cache:
        return _cache["op"]
    from playwright.sync_api import sync_playwright
    with sync_playwright() as p:
        b = p.chromium.launch(headless=True, executable_path=CHROME,
                              args=["--disable-blink-features=AutomationControlled", "--no-sandbox"])
        ctx = b.new_context(user_agent=UA, locale="zh-CN")
        pg = ctx.new_page()
        pg.goto("https://www.nhc.gov.cn/", timeout=60000, wait_until="domcontentloaded")
        pg.wait_for_timeout(8000)
        cookies = ctx.cookies()
        b.close()
    cj = http.cookiejar.CookieJar()
    for c in cookies:
        try:
            cj.set_cookie(http.cookiejar.Cookie(
                0, c["name"], c["value"], None, False,
                c.get("domain", "").lstrip("."), True, False,
                c.get("path", "/"), True, bool(c.get("secure")),
                c.get("expires"), False, None, None, {}))
        except Exception:
            pass
    op = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))
    _cache["op"] = op
    return op


def fetch(url, referer="https://www.nhc.gov.cn/"):
    req = urllib.request.Request(url, headers={
        "User-Agent": UA, "Referer": referer,
        "Accept": "text/html,*/*;q=0.8", "Accept-Language": "zh-CN,zh;q=0.9"})
    return opener().open(req, timeout=60).read()


def strip_html(html):
    html = re.sub(r'(?is)<script.*?</script>|<style.*?</style>', '', html)
    m = re.search(r'(?is)<div[^>]*class="[^"]*(?:trs_editor_view|TRS_Editor|preview|content)[^"]*"(.*?)</div>\s*(?:<div|</body)', html)
    body = m.group(1) if m else html
    text = re.sub(r'(?s)<[^>]+>', '\n', body)
    text = (text.replace('&nbsp;', '\u3000').replace('&ldquo;', '“').replace('&rdquo;', '”')
                .replace('&lsquo;', '‘').replace('&rsquo;', '’').replace('&amp;', '&')
                .replace('&lt;', '<').replace('&gt;', '>'))
    lines = [l.strip() for l in text.splitlines()]
    return '\n'.join([l for l in lines if l])


if __name__ == "__main__":
    url = sys.argv[1]
    data = fetch(url)
    Path(sys.argv[2]).write_bytes(data)
    print(len(data), "bytes")


BLOCK_MARKS = {
    "content-body": 40, "xhbox": 40, "TRS_Editor": 40, "trs_editor_view": 40,
    "zoomdiv": 40, "xxgk_content": 40, "news_content": 40, "detail-content": 40,
    "con": 60,
}


def find_body(html):
    """Return the most likely notice body HTML, or None."""
    best, best_len = None, 0
    for cls, _ in BLOCK_MARKS.items():
        for m in re.finditer(r'<div[^>]*class="([^"]*)"[^>]*>', html):
            tokens = m.group(1).split()
            if cls not in tokens and not (cls == "con" and "con" in tokens):
                if cls not in m.group(1):
                    continue
            if cls == "con" and "con" not in tokens:
                continue
            low = m.group(1).lower()
            if any(b in low for b in ("footer", "header", "nav", "banner", "search")):
                continue
            start = m.end()
            depth, k = 1, start
            while k < len(html) and depth:
                mm = re.compile(r'</?div\b').search(html, k)
                if not mm:
                    k = len(html)
                    break
                depth += -1 if mm.group(0).startswith('</') else 1
                k = mm.end()
            seg = html[start:k]
            if len(seg) > best_len:
                best, best_len = seg, len(seg)
    return best


from html.parser import HTMLParser

_BLOCK = {"p", "div", "li", "h1", "h2", "h3", "h4", "h5", "h6", "tr", "table",
          "section", "blockquote", "dl", "dd", "dt", "pre"}


class _Text(HTMLParser):
    """Tolerant text extraction: bare '<' before digits stays literal text."""

    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.buf = []
        self.skip = 0

    def handle_starttag(self, tag, attrs):
        if tag in ("script", "style"):
            self.skip += 1
        elif tag == "br":
            self.buf.append("\x01")
        elif tag in _BLOCK:
            self.buf.append("\x00")

    def handle_endtag(self, tag):
        if tag in ("script", "style") and self.skip:
            self.skip -= 1
        elif tag in _BLOCK:
            self.buf.append("\x00")

    def handle_data(self, data):
        if not self.skip:
            self.buf.append(data)


def to_text(fragment):
    """Markup -> reflowed plain text (one output line per block)."""
    ps = _Text()
    ps.feed(fragment)
    ps.close()
    t = "".join(ps.buf).replace("\u00a0", " ").replace("\u3000", " ")
    t = re.sub(r"[ \t\r\n]+", " ", t)
    t = t.replace("\x00", "\n").replace("\x01", "\n")
    # drop padding spaces that only separate CJK characters, keep "5 s" style
    cjk = "[\u4e00-\u9fff\u3000-\u303ff\uff00-\uffef]"
    t = re.sub(r"(?<=%s) +(?=%s)" % (cjk, cjk), "", t)
    t = re.sub(r"(?<=%s) +(?=\d)" % cjk, "", t)
    t = re.sub(r"(?<=\d) +(?=%s)" % cjk, " ", t)
    return "\n".join(l for l in (x.strip() for x in t.splitlines()) if l)


def content_body(html):
    """Return cleaned text of a government notice body page."""
    frag = find_body(html)
    if frag is None:
        return to_text(html)
    return to_text(frag)


def attachments(html, base):
    """Absolute URLs of doc/pdf attachments referenced on a notice page."""
    out = []
    for m in re.finditer(r'href="([^"]*\.(?:pdf|docx?|wps|zip))"', html, re.I):
        u = m.group(1)
        if u.startswith('http'):
            out.append(u)
        elif u.startswith('/'):
            out.append(base.rstrip('/') + u)
        else:
            out.append(base.rstrip('/') + '/' + u.lstrip('./'))
    return out
