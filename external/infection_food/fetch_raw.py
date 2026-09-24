#!/usr/bin/env python3
"""Fetch official Chinese health-education pages and dump VERBATIM body text.

Usage:
  python3 external/infection_food/fetch_raw.py <url> <topic-slug> [--debug]

Writes external/infection_food/raw/<topic-slug>.txt :
  line 1 = "<SOURCE: url>"  line 2 = "<TITLE: ...>"  then one paragraph per line.

Handles:
  - chinacdc.cn / nhc.gov.cn / generic TRS pages (div id=Zoom / trs_editor_view)
  - gbk or utf-8 pages
  - attached .docx (unzip word/document.xml, join <w:t> per <w:p>)
"""
import html as H
import io
import re
import ssl
import sys
import time
import urllib.request
import zipfile
from pathlib import Path

RAW = Path(__file__).parent / "raw"
RAW.mkdir(parents=True, exist_ok=True)

UA = {"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
                    "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36",
      "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
      "Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8"}

CTX = ssl.create_default_context()
CTX.check_hostname = False
CTX.verify_mode = ssl.CERT_NONE


def get_bytes(url, timeout=40, tries=3):
    last = None
    for i in range(tries):
        try:
            req = urllib.request.Request(url, headers=UA)
            with urllib.request.urlopen(req, timeout=timeout, context=CTX) as r:
                return r.read()
        except Exception as e:  # noqa: BLE001
            last = e
            time.sleep(1.5 * (i + 1))
    raise last


def decode(data: bytes) -> str:
    for enc in ("utf-8", "gbk", "gb18030"):
        try:
            return data.decode(enc)
        except UnicodeDecodeError:
            continue
    return data.decode("utf-8", "replace")


def strip_tags(s: str) -> str:
    s = re.sub(r"<script[\s\S]*?</script>|<style[\s\S]*?</style>", "", s, flags=re.I)
    s = re.sub(r"<br\s*/?>|</p>|</div>|</li>|</h[1-6]>|</td>|</tr>|</th>", "\n", s, flags=re.I)
    s = re.sub(r"<[^>]+>", "", s)
    return H.unescape(s)


def paragraphs(text: str):
    out = []
    for ln in text.splitlines():
        ln = re.sub(r"[ \t\u3000]+", lambda m: " " if m.group(0).strip() else "", ln).strip()
        ln = re.sub(r"\s*\n\s*", "", ln)
        if ln:
            out.append(ln)
    return out


def extract_body(page: str):
    """Return (title, body_paragraphs) from an HTML page."""
    t = re.search(r"<title>([\s\S]*?)</title>", page, re.I)
    title = strip_tags(t.group(1)).strip() if t else ""
    cands = []
    # most-specific article containers first
    for pat in (
        r'<div[^>]+id=["\']Zoom["\'][^>]*>([\s\S]*?)</div>\s*(?:<div|<script|<!--)',
        r'<div[^>]+class="[^"]*trs_editor_view[^"]*"[^>]*>([\s\S]*?)</div>\s*</div>',
        r'<div[^>]+class="[^"]*(?:article-content|art_con|content_main|TRS_Editor|pp_content|article_content|detail-content)[^"]*"[^>]*>([\s\S]*?)</div>\s*(?:<div|</div>\s*</div>)',
        r'<div[^>]+id=["\'](?:content|zoom|UCAP-CONTENT|Article_Content|ivs_content)["\'][^>]*>([\s\S]*?)</div>',
        # Fujian/gov article areas, cut at footer markers
        r'<div[^>]+class="[^"]*article_(?:area|content)[^"]*"[^>]*>([\s\S]*?)(?=中文域名|新媒体矩阵|版权所有)',
        r'<main[^>]*>([\s\S]*?)</main>',
        r'<div[^>]+class="[^"]*sf-detail-body[^"]*"[^>]*>([\s\S]*?)</div>\s*</div>',
        # gov.cn
        r'<div[^>]+class="[^"]*zoom[^"]*"[^>]*>([\s\S]*?)</div>\s*(?:<div|<script)',
        r'<td[^>]+id="UCAP-CONTENT"[^>]*>([\s\S]*?)</td>',
    ):
        for m in re.finditer(pat, page, re.I):
            cands.append(m.group(1))
    # fallback: whole page minus nav
    best, bestlen = "", -1
    for c in cands:
        txt = strip_tags(c)
        n = len(re.findall(r"[\u4e00-\u9fff]", txt))
        if n > bestlen:
            best, bestlen = txt, n
    if bestlen < 200:
        # last-resort: strip whole page
        body = strip_tags(page)
        paras = paragraphs(body)
        # drop obvious nav lines but keep bare numeric table cells (≥80%, 70/10万…)
        paras = [p for p in paras if len(re.findall(r"[\u4e00-\u9fff]", p)) >= 4
                 or re.fullmatch(r"[≥≤<>＜＞]?\s*\d+(\.\d+)?\s*(%|／10万|/10万)?", p.replace(" ", "")
                                 .replace("＜", "<").replace("＞", ">"))
                 or re.fullmatch(r"[≥≤<>＜＞]?\d+(\.\d+)?(%|／10万|/10万)", p)]
        return title, paras, True
    # also find h1 title
    h1 = re.search(r"<h1[^>]*>([\s\S]*?)</h1>", page, re.I)
    if h1:
        title = strip_tags(h1.group(1)).strip() or title
    return title, paragraphs(best), False


def find_attachments(page: str, base: str):
    """Return absolute URLs of .docx/.doc/.pdf links in the page."""
    out = []
    for m in re.finditer(r'href=["\']([^"\']+\.(?:docx|pdf))["\']', page, re.I):
        u = urllib.request.urljoin(base, m.group(1))
        if u not in out:
            out.append(u)
    return out


def docx_text(data: bytes):
    z = zipfile.ZipFile(io.BytesIO(data))
    xml = z.read("word/document.xml").decode("utf-8", "replace")
    paras = []
    for p in re.findall(r"<w:p[ >][\s\S]*?</w:p>|<w:p/>", xml):
        runs = re.findall(r"<w:t[^>]*>([\s\S]*?)</w:t>", p)
        line = H.unescape("".join(runs)).strip()
        if line:
            paras.append(line)
    return paras


def pdf_paras(data: bytes):
    from pypdf import PdfReader
    r = PdfReader(io.BytesIO(data))
    out = []
    for pg in r.pages:
        try:
            txt = pg.extract_text() or ""
        except Exception:  # noqa: BLE001
            continue
        out.extend(paragraphs(txt))
    return out


def main():
    url, slug = sys.argv[1], sys.argv[2]
    debug = "--debug" in sys.argv
    data = get_bytes(url)
    out = RAW / f"{slug}.txt"
    if url.lower().endswith(".pdf") or data[:4] == b"%PDF":
        ps = pdf_paras(data)
        c = sum(len(re.findall(r"[\u4e00-\u9fff]", p)) for p in ps)
        with out.open("w", encoding="utf-8") as f:
            f.write(f"<SOURCE: {url}>\n<TITLE: (pdf)>\n")
            f.write("\n".join(ps) + "\n")
        print(f"WROTE {out} from PDF paras={len(ps)} cjk={c}")
        return
    page = decode(data)
    if debug:
        Path("/tmp/fetchdbg.html").write_text(page, encoding="utf-8")
        print("HTTP ok, len", len(data), "status page saved /tmp/fetchdbg.html")
    title, paras, noisy = extract_body(page)
    cjk = sum(len(re.findall(r"[\u4e00-\u9fff]", p)) for p in paras)
    print(f"page extract: title={title!r} paras={len(paras)} cjk={cjk} fallback={noisy}")
    out = RAW / f"{slug}.txt"
    if cjk >= 800 and not noisy:
        with out.open("w", encoding="utf-8") as f:
            f.write(f"<SOURCE: {url}>\n<TITLE: {title}>\n")
            f.write("\n".join(paras) + "\n")
        print("WROTE", out, "cjk", cjk)
        return
    # try attachments (docx/pdf) — print candidates
    att = find_attachments(page, url)
    print("attachments:", att)
    for a in att:
        try:
            b = get_bytes(a)
        except Exception as e:  # noqa: BLE001
            print("fail", a, e)
            continue
        try:
            if a.lower().endswith(".docx"):
                ps = docx_text(b)
                c = sum(len(re.findall(r"[\u4e00-\u9fff]", p)) for p in ps)
                print(f"docx {a}: paras={len(ps)} cjk={c}")
                if c > cjk:
                    with out.open("w", encoding="utf-8") as f:
                        f.write(f"<SOURCE: {a}>\n<TITLE: {title} (attached docx)>\n")
                        f.write("\n".join(ps) + "\n")
                    print("WROTE", out, "from docx, cjk", c)
                    return
        except Exception as e:  # noqa: BLE001
            print("docx parse fail", a, e)
    if noisy or cjk < 800:
        # still write page text so a human can inspect, marked as such
        with out.open("w", encoding="utf-8") as f:
            f.write(f"<SOURCE: {url}>\n<TITLE: {title}>\n")
            f.write("\n".join(paras) + "\n")
        print(f"WARN wrote LOW-QUALITY {out} cjk={cjk}")


main()
