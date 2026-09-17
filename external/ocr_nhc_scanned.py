#!/usr/bin/env python3
"""OCR scanned nhc guideline PDFs with the local my-ocr tool (Apple Vision).

Pipeline: PyMuPDF renders each page to PNG (150 dpi) -> my-ocr recognizes
the page -> texts are concatenated into external/nhc/guides_ocr/{slug}.json
(same schema as guides/: title/url/year/content).
"""
import json, subprocess, sys, tempfile, time
from pathlib import Path

SCANNED = Path(__file__).parent / "nhc" / "scanned"
OUT = Path(__file__).parent / "nhc" / "guides_ocr"
MY_OCR = "/usr/local/bin/my-ocr"


def ocr_pdf(pdf_path):
    import fitz  # PyMuPDF
    doc = fitz.open(str(pdf_path))
    pages = []
    with tempfile.TemporaryDirectory() as tmp:
        for i, page in enumerate(doc):
            pix = page.get_pixmap(dpi=150)
            img = Path(tmp) / f"p{i:03d}.png"
            pix.save(str(img))
            r = subprocess.run([MY_OCR, str(img)], capture_output=True, text=True, timeout=120)
            if r.returncode != 0:
                print(f"  OCR 失败 页{i}: {r.stderr[:80]}", file=sys.stderr)
                continue
            pages.append(r.stdout.strip())
            time.sleep(0.2)
    return "\n\n".join(pages)


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    pdfs = sorted(SCANNED.glob("*.pdf"))
    print(f"共 {len(pdfs)} 个扫描版 PDF", file=sys.stderr)
    for i, pdf in enumerate(pdfs, 1):
        slug = pdf.stem
        out = OUT / f"{slug}.json"
        if out.exists():
            print(f"[{i}/{len(pdfs)}] SKIP {slug[:40]}", file=sys.stderr)
            continue
        try:
            text = ocr_pdf(pdf)
            if len(text) < 800:
                print(f"[{i}/{len(pdfs)}] ⚠️ OCR 结果过短 ({len(text)}): {slug[:40]}", file=sys.stderr)
                continue
            out.write_text(json.dumps(
                {"title": slug.replace("-", " "), "url": "", "year": "", "content": text},
                ensure_ascii=False), encoding="utf-8")
            print(f"[{i}/{len(pdfs)}] ✅ {slug[:44]} ({len(text)}字)", file=sys.stderr)
        except Exception as e:
            print(f"[{i}/{len(pdfs)}] ❌ {slug[:40]}: {str(e)[:80]}", file=sys.stderr)


if __name__ == "__main__":
    main()
