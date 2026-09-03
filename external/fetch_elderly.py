#!/usr/bin/env python3
"""Fetch elderly-care (老年护理) official documents into plain-text caches.

Sources are fixed gov.cn / institutional pages (all verified direct-HTTP):
  - 老年失能预防核心信息 (国卫办老龄函〔2019〕689号, gov.cn 政策库)
  - 阿尔茨海默病预防与干预核心信息 (国卫办老龄函〔2019〕738号, 转载全文)
  - 中国老年人膳食指南(2022) 核心推荐 (中国营养学会 cnsoc 官网)
  - 防治骨质疏松知识要点 (卫办疾控函〔2011〕542号, 区政府转载全文)

Output: external/elderly/{slug}.txt  (first line = title, blank, body)
Idempotent: skips existing files.

  python3 external/fetch_elderly.py [--refresh]
"""
import argparse
import html as htmlmod
import re
import subprocess
import sys
import tempfile
import time
import urllib.parse
import urllib.request
from pathlib import Path

OUT = Path(__file__).parent / "elderly"
UA = {"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
                    "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"}

# slug -> (title, url, body-container regex hints)
DOCS = {
    "disability-prevention-2019": (
        "老年失能预防核心信息(国卫办老龄函〔2019〕689号)",
        "https://www.gov.cn/zhengce/zhengceku/2019-11/18/content_5453051.htm",
        [r'(?s)<div[^>]*id="UCAP-CONTENT"[^>]*>(.*?)</div>\s*(?:<div|<!--)',
         r'(?s)class="pages_content"[^>]*>(.*?)</div>']),
    "alzheimer-2019": (
        "阿尔茨海默病预防与干预核心信息(国卫办老龄函〔2019〕738号)",
        "https://www.crsi.com.cn/Html/News/Articles/642.html",
        [r'(?s)<div[^>]*class="[^"]*(?:article|content|detail)[^"]*"[^>]*>(.*?)</div>\s*<div',
         r'(?s)<td[^>]*class="[^"]*content[^"]*"[^>]*>(.*?)</td>']),
    "elderly-dietary-2022": (
        "中国老年人膳食指南(2022)核心推荐",
        "http://dg.cnsoc.org/article/04/op9MZtpBQHehHCo0SSqsmw.html",
        [r'(?s)<div[^>]*class="right-con news-show"[^>]*>(.*?)</div>',
         r'(?s)<div[^>]*class="[^"]*(?:left-con|news-show)[^"]*"[^>]*>(.*?)</div>']),
    "osteoporosis-2011": (
        "防治骨质疏松知识要点(卫办疾控函〔2011〕542号)",
        "http://www.bjchy.gov.cn/affair/domain/yl/8a24fe83302019b101311d7b75e80e4e.html",
        [r'(?s)<div[^>]*class="content_article"[^>]*>(.*?)</div>\s*(?:<div|$)',
         r'(?s)<div[^>]*(?:id|class)="[^"]*(?:UCAP-CONTENT|pages_content|content)[^"]*"[^>]*>(.*?)</div>']),
}


def get(url: str, timeout: int = 30) -> str:
    req = urllib.request.Request(url, headers=UA)
    for i in range(3):
        try:
            with urllib.request.urlopen(req, timeout=timeout) as r:
                raw = r.read()
                charset = "utf-8"
                ct = r.headers.get("Content-Type", "")
                m = re.search(r"charset=([\w-]+)", ct)
                if not m:
                    # fall back to the page's <meta charset> declaration
                    m = re.search(rb'charset=["\']?([\w-]+)', raw[:2048])
                if m:
                    charset = m.group(1)
                    if isinstance(charset, bytes):
                        charset = charset.decode("ascii")
                text = raw.decode(charset, "replace")
                # a heavy replacement-char ratio means the declared charset lied
                if text[:4000].count("�") > 40 and charset.lower() not in ("gbk", "gb2312", "gb18030"):
                    text = raw.decode("gb18030", "replace")
                return text
        except Exception:
            if i == 2:
                raise
            time.sleep(1.5 * (i + 1))


def html_to_text(fragment: str) -> str:
    fragment = re.sub(r"<script.*?</script>", " ", fragment, flags=re.S)
    fragment = re.sub(r"<style.*?</style>", " ", fragment, flags=re.S)
    # paragraph breaks before stripping tags
    fragment = re.sub(r"</(?:p|div|tr|li|h[1-6]|table)>", "\n", fragment)
    fragment = re.sub(r"<br\s*/?>", "\n", fragment)
    fragment = re.sub(r"<[^>]+>", "", fragment)
    fragment = htmlmod.unescape(fragment)
    fragment = re.sub(r"[ \t　]+", " ", fragment)
    fragment = re.sub(r"\n\s*\n+", "\n\n", fragment)
    return fragment.strip()


def extract(html: str, patterns: list[str]) -> str:
    for pat in patterns:
        m = re.search(pat, html)
        if m:
            text = html_to_text(m.group(1))
            if len(text) > 300:
                return text
    # fallback: whole page, then cut nav noise by keeping the longest block
    text = html_to_text(html)
    blocks = [b.strip() for b in text.split("\n\n") if len(b.strip()) > 120]
    return "\n\n".join(blocks) if blocks else text


def fetch_doc_attachment(html: str, page_url: str) -> bytes:
    """Download the first .doc/.docx attachment linked from the page."""
    m = re.search(r'href="([^"]+\.(?:doc|docx))"', html, re.I)
    if not m:
        raise ValueError("页面无 .doc 附件")
    rel = htmlmod.unescape(m.group(1))
    if rel.startswith("http"):
        url = rel
    elif rel.startswith("/"):
        origin = re.match(r"https?://[^/]+", page_url).group(0)
        url = origin + urllib.parse.quote(rel)
    else:
        url = page_url.rsplit("/", 1)[0] + "/" + urllib.parse.quote(rel)
    req = urllib.request.Request(url, headers=UA)
    with urllib.request.urlopen(req, timeout=60) as r:
        return r.read()


def doc_to_text(data: bytes) -> str:
    """Extract text from a legacy .doc via macOS textutil."""
    with tempfile.NamedTemporaryFile(suffix=".doc", delete=False) as tf:
        tf.write(data)
        path = tf.name
    try:
        out = subprocess.run(["textutil", "-convert", "txt", "-stdout", path],
                             capture_output=True, check=True, timeout=60)
        return out.stdout.decode("utf-8", "replace").strip()
    finally:
        Path(path).unlink(missing_ok=True)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--refresh", action="store_true")
    args = parser.parse_args()
    OUT.mkdir(parents=True, exist_ok=True)

    ok, fail = 0, []
    for slug, (title, url, patterns) in DOCS.items():
        out = OUT / f"{slug}.txt"
        if out.exists() and not args.refresh:
            print(f"SKIP {slug}", file=sys.stderr)
            ok += 1
            continue
        try:
            html = get(url)
            body = extract(html, patterns)
            if len(body) < 400:
                # some notice pages carry the real content as a .doc attachment
                body = doc_to_text(fetch_doc_attachment(html, url))
            if len(body) < 400:
                raise ValueError(f"正文过短 {len(body)} 字")
            out.write_text(f"{title}\n\n{body}\n", encoding="utf-8")
            ok += 1
            print(f"✅ {slug} ({len(body)} 字) 首段: {body[:60]!r}", file=sys.stderr)
        except Exception as e:
            fail.append((slug, str(e)[:100]))
            print(f"❌ {slug}: {str(e)[:100]}", file=sys.stderr)
        time.sleep(0.5)
    print(f"DONE: ok={ok} fail={len(fail)}", file=sys.stderr)
    return 0 if not fail else 1


if __name__ == "__main__":
    sys.exit(main())
