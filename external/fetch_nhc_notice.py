#!/usr/bin/env python3
"""通用: 抓取 nhc.gov.cn 通知页正文 + 附件 PDF/doc, 转 JSON 入 nhc/guides.

用法: python3 fetch_nhc_notice.py <NOTICE_URL> <TITLE> [YEAR]
输出: external/nhc/guides/<slug>.json ({title,url,year,content})
正文=通知页 inline; 若 inline 无指南正文则取第一个附件抽文本;
附件无文本层时存 external/nhc/scanned/ 供 ocr_nhc_scanned.py.
"""
import json
import re
import sys
import time
import urllib.parse
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from fetch_nhc_all import build_opener, extract_inline, harvest_cookies, looks_like_guide, pdf_text  # noqa: E402

OUT = Path(__file__).parent / "nhc" / "guides"
SCANNED = Path(__file__).parent / "nhc" / "scanned"


def slugify(t: str) -> str:
    return re.sub(r"[^\w一-鿿]", "-", t)[:60]


def playwright_fetch(url: str):
    """浏览器直开: WAF JS 挑战页 (HTTP 412) 的兜底.

    返回 (html, getter) — getter(att_url) 用同一浏览器上下文下载附件字节.
    """
    from playwright.sync_api import sync_playwright
    pw = sync_playwright().start()
    browser = pw.chromium.launch(headless=True)
    ctx = browser.new_context(user_agent=(
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 "
        "(KHTML, like Gecko) Chrome/124.0 Safari/537.36"))
    page = ctx.new_page()
    page.goto(url, wait_until="domcontentloaded", timeout=60000)
    page.wait_for_timeout(3000)
    html = page.content()

    def getter(att_url: str, referer: str) -> bytes:
        resp = ctx.request.get(att_url, headers={"Referer": referer}, timeout=300000)
        resp.raise_for_status()
        return resp.body()

    return html, getter, (pw, browser)


def main() -> int:
    if len(sys.argv) < 3:
        print(__doc__)
        return 2
    notice, title = sys.argv[1], sys.argv[2]
    year_m = re.search(r"/(20\d{2})/", notice)
    year = sys.argv[3] if len(sys.argv) > 3 else (year_m.group(1) if year_m else "")

    OUT.mkdir(parents=True, exist_ok=True)
    SCANNED.mkdir(parents=True, exist_ok=True)
    print("获取 WAF cookie...", file=sys.stderr)
    opener = build_opener(harvest_cookies())

    try:
        html = opener.open(notice, timeout=60).read().decode("utf-8", "replace")
        getter = lambda att_url, referer: opener.open(att_url, timeout=300).read()  # noqa: E731
        cleanup = None
    except Exception as e:
        print(f"直抓失败({str(e)[:60]}), playwright 兜底...", file=sys.stderr)
        html, getter, cleanup = playwright_fetch(notice)
    content = extract_inline(html)
    if looks_like_guide(content):
        text, via = content, "inline"
    else:
        atts = re.findall(r'href="([^"]*\.(?:pdf|docx?))"', html, flags=re.I)
        if not atts:
            print(f"❌ {title}: 无正文无附件", file=sys.stderr)
            if cleanup:
                cleanup[1].close()
                cleanup[0].stop()
            return 1
        base_dir = notice.rsplit("/", 1)[0]
        text = via = ""
        for rel in atts:
            if rel.startswith("http"):
                att_url = rel
            else:
                att_url = urllib.parse.quote(base_dir + "/" + rel.lstrip("/"), safe=":/?&=%")
            print(f"下载附件 {att_url}", file=sys.stderr)
            raw = getter(att_url, notice)
            time.sleep(1)
            suffix = Path(urllib.parse.unquote(rel)).suffix.lower()
            if suffix != ".pdf":
                (SCANNED / f"{slugify(title)}{suffix}").write_bytes(raw)
                print(f"📄 非PDF附件存 scanned/ 待转换: {suffix}", file=sys.stderr)
                continue
            try:
                text = pdf_text(raw)
            except Exception as e:
                print(f"⚠️ pdf 解析失败: {e}", file=sys.stderr)
                text = ""
            if looks_like_guide(text):
                via = f"pdf({len(raw)//1024}KB)"
                break
            (SCANNED / f"{slugify(title)}{suffix}").write_bytes(raw)
            print(f"📄 无文本层, 存 scanned/{slugify(title)}{suffix} 待 OCR", file=sys.stderr)
            text = ""
        if not text:
            return 1

    slug = slugify(title)
    out = OUT / f"{slug}.json"
    out.write_text(json.dumps(
        {"title": title, "url": notice, "year": year, "content": text},
        ensure_ascii=False), encoding="utf-8")
    print(f"✅ guides/{slug}.json via {via} ({len(text)} 字)", file=sys.stderr)
    if cleanup:
        cleanup[1].close()
        cleanup[0].stop()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
