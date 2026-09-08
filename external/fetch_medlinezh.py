#!/usr/bin/env python3
"""MedlinePlus 多语言中文健康材料 (Health Infotranslations PDFs) →
internal/knowledge/data/corpus_medlinezh.json

源: medlineplus.gov/languages/chinesesimplifiedmandarindialect.html 列出的
storage.googleapis.com/healthinfotranslations-pdfdocs/*-SCh.pdf (简体中文).
材料多为俄亥俄州立大学 Wexner 医学中心编制, 经 MedlinePlus 分发, 供个人/教育
非商业使用 — 与 StatPearls 同为自用 KB.

用法: python3 fetch_medlinezh.py            # 下载+转换
      python3 fetch_medlinezh.py --pdfs-only # 只下载 PDF 不转换
"""
import json
import re
import sys
import time
import urllib.request
from pathlib import Path

BASE = Path(__file__).parent
PDF_DIR = BASE / "medlinezh" / "pdfs"
LIST_URL = "https://medlineplus.gov/languages/chinesesimplifiedmandarindialect.html"
UA = {"User-Agent": "Mozilla/5.0 (research; contact: self)"}


def get(url: str, binary: bool = False, tries: int = 3):
    req = urllib.request.Request(url, headers=UA)
    for i in range(tries):
        try:
            with urllib.request.urlopen(req, timeout=60) as r:
                data = r.read()
            return data if binary else data.decode("utf-8", "replace")
        except Exception as e:
            if i == tries - 1:
                print(f"  ⚠️ {url[-50:]}: {e}", file=sys.stderr)
                return None
            time.sleep(2)


def pdf_text(raw: bytes) -> str:
    import io
    from pypdf import PdfReader
    r = PdfReader(io.BytesIO(raw))
    return "\n".join((p.extract_text() or "") for p in r.pages).strip()


def list_items():
    html = get(LIST_URL)
    items = {}  # url -> (en_title)
    for m in re.finditer(
            r'<a href="(https://storage\.googleapis\.com/healthinfotranslations-pdfdocs/[^"]+?\.pdf)"[^>]*title="([^"<]+?)\s*-\s*(?:<span[^>]*>)?[^"]*?PDF',
            html):
        url, title = m.group(1), m.group(2)
        title = re.sub(r"\s*<span.*$", "", title)
        title = title.replace("&#x27;", "'").replace("&amp;", "&").strip()
        items[url] = title
    return items


def main() -> int:
    PDF_DIR.mkdir(parents=True, exist_ok=True)
    items = list_items()
    print(f"发现 {len(items)} 个简体中文 PDF", file=sys.stderr)
    if not items:
        return 1

    ok = 0
    docs = []
    for n, (url, title) in enumerate(sorted(items.items()), 1):
        name = url.rsplit("/", 1)[-1]
        pdf_path = PDF_DIR / name
        if not pdf_path.exists():
            raw = get(url, binary=True)
            if not raw or raw[:4] != b"%PDF":
                continue
            pdf_path.write_bytes(raw)
            time.sleep(0.4)
        ok += 1
        if len(sys.argv) > 1 and sys.argv[1] == "--pdfs-only":
            continue
        try:
            text = pdf_text(pdf_path.read_bytes())
        except Exception as e:
            print(f"  ⚠️ 文本抽取失败 {name}: {e}", file=sys.stderr)
            continue
        if len(text) < 300:
            continue
        docs.append({
            "source": "medlinezh",
            "id": "medlinezh-" + re.sub(r"\.pdf$", "", name),
            "lang": "zh",
            "title": title,
            "url": url,
            "body": text[:24000],
        })
        if n % 40 == 0:
            print(f"  … {n}/{len(items)}", file=sys.stderr)

    if not (len(sys.argv) > 1 and sys.argv[1] == "--pdfs-only"):
        out = BASE.parent / "internal" / "knowledge" / "data" / "corpus_medlinezh.json"
        out.write_text(json.dumps({
            "source": "medlinezh",
            "updated": "2026-09",
            "entries": docs,
        }, ensure_ascii=False), encoding="utf-8")
        print(f"wrote {out} ({len(docs)} docs)", file=sys.stderr)
    print(f"PDF 下载成功 {ok}/{len(items)}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
