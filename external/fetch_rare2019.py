#!/usr/bin/env python3
"""抓取第一批罕见病诊疗指南(2019年版, 121 病种)通知页 + 附件 PDF.

复用 fetch_nhc_all 的 WAF cookie/抓取逻辑. 通知页:
https://www.nhc.gov.cn/yzygj/c100068/201902/073540e8f83b4a54a28684d23e2ae2f5.shtml

输出: external/nhc/guides/<slug>.json  ({title,url,year,content})
有文本层的 PDF 直接抽文本; 扫描版存 external/nhc/scanned/ 供 ocr_nhc_scanned.py.
"""
import io
import re
import sys
import time
import urllib.parse
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from fetch_nhc_all import build_opener, harvest_cookies, pdf_text  # noqa: E402

NOTICE = "https://www.nhc.gov.cn/yzygj/c100068/201902/073540e8f83b4a54a28684d23e2ae2f5.shtml"
OUT = Path(__file__).parent / "nhc" / "guides"
SCANNED = Path(__file__).parent / "nhc" / "scanned"

TITLE = "罕见病诊疗指南（2019年版）"


def main() -> int:
    OUT.mkdir(parents=True, exist_ok=True)
    SCANNED.mkdir(parents=True, exist_ok=True)
    print("获取 WAF cookie...", file=sys.stderr)
    opener = build_opener(harvest_cookies())

    data = opener.open(NOTICE, timeout=60).read()
    html = data.decode("utf-8", "replace")
    pdfs = re.findall(r'href="([^"]*\.(?:pdf|docx?))"', html, flags=re.I)
    if not pdfs:
        print("❌ 通知页未发现附件链接", file=sys.stderr)
        print(html[:2000], file=sys.stderr)
        return 1
    base_dir = NOTICE.rsplit("/", 1)[0]
    got = 0
    for rel in pdfs:
        pdf_url = urllib.parse.quote(base_dir + "/" + rel.lstrip("/"), safe=":/?&=%")
        print(f"下载 {pdf_url}", file=sys.stderr)
        raw = opener.open(pdf_url, timeout=300).read()
        time.sleep(1)
        slug = re.sub(r"[^\w一-鿿]", "-", TITLE)
        suffix = Path(urllib.parse.unquote(rel)).suffix.lower()
        try:
            text = pdf_text(raw)
        except Exception as e:
            print(f"⚠️ pdf 解析失败: {e}", file=sys.stderr)
            text = ""
        if len(text) >= 800 and re.search(r"[一二三四五六七八九十]、", text):
            out = OUT / f"{slug}{suffix}.json"
            out.write_text(
                __import__("json").dumps(
                    {"title": TITLE, "url": NOTICE, "year": "2019", "content": text},
                    ensure_ascii=False),
                encoding="utf-8")
            got += 1
            print(f"✅ guides/{slug}{suffix}.json ({len(text)} 字)", file=sys.stderr)
        else:
            p = SCANNED / f"{slug}{suffix}"
            p.write_bytes(raw)
            print(f"📄 无文本层/过短({len(text)}字), 存 scanned/{p.name} ({len(raw)//1024}KB) 待 OCR", file=sys.stderr)
    print(f"完成: {got} 个文本版", file=sys.stderr)
    return 0 if got else 1


if __name__ == "__main__":
    raise SystemExit(main())
