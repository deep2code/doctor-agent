#!/usr/bin/env python3
"""Second pass over external/gov_health: harvest EVERY annex PDF of each document.

fetch_gov_health.py caps at two attachments per document; 指南/方案 的正文往往就在
剩下的附件里. This script walks index.tsv, re-reads each document page, and
extracts any /P0….pdf that is not already on disk.

  python3 external/fetch_gov_annex.py            # all documents
  python3 external/fetch_gov_annex.py -limit 10  # sample
"""
import glob
import html
import os
import re
import sys
from concurrent.futures import ThreadPoolExecutor
from urllib.parse import urljoin

import requests

from fetch_gov_health import fix_surrogates

OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "gov_health")
RAW = os.path.join(OUT, "raw")
PDFDIR = os.path.join(OUT, "pdf")
UA = {
    "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
    "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
}
ATTACH_RE = re.compile(r'href="([^"]+?\.pdf)"', re.I)
CJK = re.compile(r"[\u4e00-\u9fff]")

s = requests.Session()
s.headers.update(UA)


def get(url, binary=False, tries=2):
    for i in range(tries):
        try:
            r = s.get(url, timeout=30)
            r.encoding = "utf-8"
            if r.status_code == 200 and r.content:
                return r.content if binary else r.text
            if r.status_code in (404, 410):
                return None
        except Exception:  # noqa: BLE001
            pass
    return None


def pdf_text(path):
    try:
        from pypdf import PdfReader
        rd = PdfReader(path)
        return "\n".join((pg.extract_text() or "") for pg in rd.pages)
    except Exception:  # noqa: BLE001
        return ""


def clean(text):
    text = re.sub(r"<[^>]+>", "\n", text)
    text = fix_surrogates(html.unescape(text))
    return "\n".join(
        ln.strip() for ln in text.splitlines()
        if CJK.search(ln) or re.search(r"\d", ln)
    )


def header(title, org, pubtime, url, extra=""):
    return (
        f"来源: {org or '中国政府网'}《{title}》\n"
        f"发布时间: {pubtime}\n"
        f"URL: {url}\n" + extra +
        "抓取方式: 直连 gov.cn 政策文件库 + 附件 PDF 全文提取，未改写\n"
        + "=" * 60 + "\n"
    )


def main():
    limit = 0
    if "-limit" in sys.argv:
        limit = int(sys.argv[sys.argv.index("-limit") + 1])
    index_path = os.path.join(OUT, "index.tsv")
    rows = [ln.rstrip("\n").split("\t") for ln in open(index_path, encoding="utf-8") if ln.strip()]
    docs = [r for r in rows if len(r) >= 4 and "_att_" not in r[0]]
    have = {os.path.basename(p)[:-4] for p in glob.glob(os.path.join(RAW, "gov_*_att_*.txt"))}
    have_basenames = {os.path.basename(p) for p in glob.glob(os.path.join(PDFDIR, "*.pdf"))}
    print(f"documents to scan: {len(docs)}", file=sys.stderr)

    def work(row):
        fname, org, title, url = row[0], row[1], row[2], row[3]
        pubtime = ""
        page = get(url)
        if not page:
            return []
        cid = re.search(r"content_(\d+)", url)
        cid = cid.group(1) if cid else fname.split("_")[1]
        out = []
        for au in dict.fromkeys(ATTACH_RE.findall(page)):
            au = urljoin(url, au)
            base = au.rsplit("/", 1)[-1]
            stem = f"gov_{cid}_att_{base[:-4]}"
            if stem in have:
                continue
            pth = os.path.join(PDFDIR, base)
            if not os.path.exists(pth):
                raw = get(au, binary=True)
                if not raw or len(raw) < 8000 or raw[:4] != b"%PDF":
                    continue
                os.makedirs(PDFDIR, exist_ok=True)
                with open(pth, "wb") as fh:
                    fh.write(raw)
                have_basenames.add(base)
            txt = pdf_text(pth)
            if len(re.sub(r"\s", "", txt)) < 800:
                continue
            with open(os.path.join(RAW, stem + ".txt"), "w", encoding="utf-8") as fh:
                fh.write(header(title, org, pubtime, url, extra=f"附件原件: {au}\n") + clean_txt(txt) + "\n")
            out.append("\t".join([stem + ".txt", org, f"{title} (附件 {base})", au]))
            have.add(stem)
        return out

    written = 0
    jobs = docs[:limit] if limit else docs
    with ThreadPoolExecutor(max_workers=5) as pool:
        for out in pool.map(work, jobs):
            if not out:
                continue
            with open(index_path, "a", encoding="utf-8") as fh:
                fh.write("\n".join(out) + "\n")
            written += len(out)
            if written % 20 < 20:
                print(f"  .. {written} annex extracts", file=sys.stderr)
    print(f"DONE gov_annex: +{written} annex files", file=sys.stderr)
    return 0


def clean_txt(text):
    return "\n".join(ln.rstrip() for ln in fix_surrogates(text).splitlines() if ln.strip())


if __name__ == "__main__":
    sys.exit(main())
