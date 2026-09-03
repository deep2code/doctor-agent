#!/usr/bin/env python3
"""Fetch infectious-disease (传染病) diagnosis/treatment guidelines into
external/nhc/guides/*.json (convert_nhc.py input format).

Sources (all gov.cn 政策库, direct HTTP, no WAF):
  - 麻疹诊疗方案(2024年版)      PDF attachment
  - 登革热诊疗方案(2024年版)    PDF attachment
  - 人感染禽流感诊疗方案(2024年版) PDF attachment
  - 手足口病诊疗指南(2018年版)  .doc attachment (textutil)
狂犬病暴露预防处置工作规范(2023年版) was manually cached earlier (page is
JS-rendered; content captured via server-side reader).

Output: external/nhc/guides/关于印发{title}-的通知.json  {title,url,year,content}
Idempotent: skips existing files.

  python3 external/fetch_id_guidelines.py [--refresh] [--pythonpath PATH]
"""
import argparse
import io
import json
import re
import subprocess
import sys
import tempfile
import time
import urllib.request
from pathlib import Path

OUT = Path(__file__).parent / "nhc" / "guides"
UA = {"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
                    "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"}

NOTICE_URL = "https://www.gov.cn/zhengce/zhengceku/202407/content_6965110.htm"
# attachment href -> (entry title, year)
MEASLES_ATTACHMENTS = {
    "./P020240729756402437679.pdf": ("关于印发麻疹诊疗方案(2024年版)的通知", "2024"),
    "./P020240729756402615362.pdf": ("关于印发登革热诊疗方案(2024年版)的通知", "2024"),
    "./P020240729756402751695.pdf": ("关于印发人感染禽流感诊疗方案(2024年版)的通知", "2024"),
}
HFMD = {
    "url": "https://www.gov.cn/zhengce/zhengceku/2018-12/31/content_5435156.htm",
    "title": "关于印发手足口病诊疗指南(2018年版)的通知",
    "year": "2018",
    "doc": "https://www.gov.cn/zhengce/zhengceku/2018-12/31/5435156/files/0f23cb033f1a402ebd0fe20ca2d54dc6.doc",
}


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


def pdf_to_text(data: bytes, pypdf_path: str) -> str:
    if pypdf_path:
        sys.path.insert(0, pypdf_path)
    from pypdf import PdfReader
    reader = PdfReader(io.BytesIO(data))
    return "\n".join((p.extract_text() or "") for p in reader.pages).strip()


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


def write_entry(title: str, url: str, year: str, content: str) -> bool:
    fname = re.sub(r"[\\/:*?\"<>|]", "-", title) + ".json"
    out = OUT / fname
    if len(content) < 800:
        print(f"❌ {title}: 文本过短 {len(content)} 字", file=sys.stderr)
        return False
    out.write_text(json.dumps(
        {"title": title, "url": url, "year": year, "content": content},
        ensure_ascii=False, indent=1), encoding="utf-8")
    print(f"✅ {fname} ({len(content)} 字)", file=sys.stderr)
    return True


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--refresh", action="store_true")
    parser.add_argument("--pythonpath", default="",
                        help="path containing pypdf (e.g. .cache/pylibs)")
    args = parser.parse_args()
    OUT.mkdir(parents=True, exist_ok=True)

    ok, fail = 0, 0
    base = NOTICE_URL.rsplit("/", 1)[0]

    # 1) 麻疹/登革热/禽流感 2024 PDFs
    html = get_bytes(NOTICE_URL).decode("utf-8", "replace")
    hrefs = set(re.findall(r'href="(\./[^"]+\.pdf)"', html))
    for rel, (title, year) in MEASLES_ATTACHMENTS.items():
        fname = re.sub(r"[\\/:*?\"<>|]", "-", title) + ".json"
        if (OUT / fname).exists() and not args.refresh:
            print(f"SKIP {fname}", file=sys.stderr)
            ok += 1
            continue
        if rel not in hrefs:
            print(f"⚠️ 附件 {rel} 未在通知页找到(可能改名),跳过", file=sys.stderr)
            fail += 1
            continue
        try:
            data = get_bytes(base + rel[1:])
            if data[:4] != b"%PDF":
                raise ValueError(f"非 PDF {len(data)}B")
            content = pdf_to_text(data, args.pythonpath)
            ok += write_entry(title, NOTICE_URL, year, content)
        except Exception as e:
            print(f"❌ {title}: {str(e)[:100]}", file=sys.stderr)
            fail += 1
        time.sleep(0.5)

    # 2) 手足口 2018 .doc
    fname = re.sub(r"[\\/:*?\"<>|]", "-", HFMD["title"]) + ".json"
    if (OUT / fname).exists() and not args.refresh:
        print(f"SKIP {fname}", file=sys.stderr)
        ok += 1
    else:
        try:
            data = get_bytes(HFMD["doc"])
            content = doc_to_text(data)
            ok += write_entry(HFMD["title"], HFMD["url"], HFMD["year"], content)
        except Exception as e:
            print(f"❌ 手足口: {str(e)[:100]}", file=sys.stderr)
            fail += 1

    print(f"DONE: ok={ok} fail={fail}", file=sys.stderr)
    return 0 if fail == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
