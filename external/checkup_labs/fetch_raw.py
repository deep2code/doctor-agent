#!/usr/bin/env python3
"""Fetch helper: download URL with browser UA, save raw bytes or extracted text."""
import sys, os, re, io, urllib.request

UA = {
    "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
    "Accept": "text/html,application/xhtml+xml,application/pdf,*/*;q=0.8",
    "Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
}
RAW = os.path.dirname(os.path.abspath(__file__))

def get(url, timeout=60):
    req = urllib.request.Request(url, headers=UA)
    return urllib.request.urlopen(req, timeout=timeout).read()

def save_bytes(url, slug):
    data = get(url)
    path = os.path.join(RAW, slug)
    with open(path, "wb") as f:
        f.write(data)
    print("saved", path, len(data), "bytes", "magic:", data[:8])
    return path

def pdf_to_text(path, out):
    from pypdf import PdfReader
    r = PdfReader(path)
    chunks = []
    for i, p in enumerate(r.pages):
        t = p.extract_text() or ""
        chunks.append(f"\n===== PAGE {i+1} =====\n{t}")
    text = "".join(chunks)
    with open(out, "w", encoding="utf-8") as f:
        f.write(text)
    print(f"{path}: {len(r.pages)} pages, {len(text)} chars -> {out}")
    return text

def html_to_text(data, out):
    try:
        h = data.decode("utf-8")
    except UnicodeDecodeError:
        h = data.decode("gbk", errors="replace")
    h = re.sub(r"(?is)<(script|style)[^>]*>.*?</\1>", "", h)
    h = re.sub(r"(?i)<(br|/p|/div|/tr|/li|/h[1-6]|/table)[^>]*>", "\n", h)
    h = re.sub(r"(?i)</t[dh][^>]*>", " | ", h)
    h = re.sub(r"<[^>]+>", "", h)
    h = (h.replace("&nbsp;", " ").replace("&amp;", "&").replace("&lt;", "<")
           .replace("&gt;", ">").replace("&ldquo;", "“").replace("&rdquo;", "”")
           .replace("&mdash;", "—").replace("&times;", "×").replace("&minus;", "−"))
    h = re.sub(r"[ \t\u3000]+", " ", h)
    h = re.sub(r"\n\s*\n+", "\n", h)
    with open(out, "w", encoding="utf-8") as f:
        f.write(h.strip() + "\n")
    print(f"{out}: {len(h)} chars")
    return h

if __name__ == "__main__":
    cmd = sys.argv[1]
    if cmd == "pdf":   # pdf <url> <slug>
        p = save_bytes(sys.argv[2], sys.argv[3] + ".pdf")
        pdf_to_text(p, os.path.join(RAW, sys.argv[3] + ".txt"))
    elif cmd == "html":  # html <url> <slug>
        data = get(sys.argv[2])
        html_to_text(data, os.path.join(RAW, sys.argv[3] + ".txt"))
    elif cmd == "head":  # head <url> — print first bytes / status
        try:
            data = get(sys.argv[2])
            print("OK", len(data), "bytes magic:", data[:16])
        except Exception as e:
            print("ERR", e)
