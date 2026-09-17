#!/usr/bin/env python3
"""Fetch gynecology (妇科) official documents into plain-text caches.

Sources:
  - 宫颈癌筛查工作方案 (国卫办妇幼函〔2021〕635号 附件, PDF via nwccw.gov.cn)
  - 乳腺癌筛查工作方案 (same notice, PDF via nwccw.gov.cn)
  - 更年期女性健康教育核心信息 (国家妇幼健康中心 2025, 15 条 — manually
    transcribed into external/gyn/menopause-2025.txt from the official
    image cards; this script only verifies it exists)

Output: external/gyn/{slug}.txt  (first line = title, blank, body)
Idempotent: skips existing files.

  python3 external/fetch_gyn.py [--refresh]
"""
import argparse
import sys
import time
import urllib.request
from pathlib import Path

OUT = Path(__file__).parent / "gyn"
UA = {"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
                    "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"}

# slug -> (title, pdf url)
PDFS = {
    "cervical-screening-2021": (
        "宫颈癌筛查工作方案(国卫办妇幼函〔2021〕635号)",
        "https://www.cnwomen.com.cn/files/Resource_online/attachement_fgw/pdf/site22/20220217/acd1b89bf4a323782cf307.pdf"),
    "breast-screening-2021": (
        "乳腺癌筛查工作方案(国卫办妇幼函〔2021〕635号)",
        "https://www.cnwomen.com.cn/files/Resource_online/attachement_fgw/pdf/site22/20220217/acd1b89bf4a323782db409.pdf"),
}
NOTICE_URL = "https://www.nwccw.gov.cn/2022/02/17/99337480.html"


def get_bytes(url: str, timeout: int = 60) -> bytes:
    req = urllib.request.Request(url, headers=UA)
    for i in range(3):
        try:
            with urllib.request.urlopen(req, timeout=timeout) as r:
                return r.read()
        except Exception:
            if i == 2:
                raise
            time.sleep(2 * (i + 1))


def pdf_to_text(data: bytes) -> str:
    from pypdf import PdfReader
    import io
    reader = PdfReader(io.BytesIO(data))
    parts = []
    for page in reader.pages:
        parts.append(page.extract_text() or "")
    text = "\n".join(parts)
    return text.strip()


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--refresh", action="store_true")
    args = parser.parse_args()
    OUT.mkdir(parents=True, exist_ok=True)

    ok, fail = 0, []
    for slug, (title, url) in PDFS.items():
        out = OUT / f"{slug}.txt"
        if out.exists() and not args.refresh:
            print(f"SKIP {slug}", file=sys.stderr)
            ok += 1
            continue
        try:
            data = get_bytes(url)
            if data[:4] != b"%PDF":
                raise ValueError(f"非 PDF 响应 {len(data)}B")
            body = pdf_to_text(data)
            if len(body) < 800:
                raise ValueError(f"PDF 文本过短 {len(body)} 字(可能扫描版)")
            out.write_text(f"{title}\n\n{body}\n", encoding="utf-8")
            ok += 1
            print(f"✅ {slug} ({len(body)} 字) 首段: {body[:60]!r}", file=sys.stderr)
        except Exception as e:
            fail.append((slug, str(e)[:100]))
            print(f"❌ {slug}: {str(e)[:100]}", file=sys.stderr)
        time.sleep(0.5)

    meno = OUT / "menopause-2025.txt"
    if meno.exists():
        ok += 1
    else:
        fail.append(("menopause-2025", "手工转录缓存缺失"))
    print(f"DONE: ok={ok} fail={len(fail)}", file=sys.stderr)
    return 0 if not fail else 1


if __name__ == "__main__":
    sys.exit(main())
