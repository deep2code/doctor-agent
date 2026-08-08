#!/usr/bin/env python3
"""Auto-discover and download ALL clinical guidelines from nhc.gov.cn.

Strategy:
  1. Playwright loads the homepage -> harvests WAF cookies (JS challenge).
  2. urllib reuses cookies to walk the 政策文件 (ylyjs/zcwj) and 工作动态
     (ylyjs/gzdt) list pages (new_list{_N}.shtml pagination).
  3. Filter links whose titles look like clinical guidelines (诊疗方案/
     诊疗指南/防治指南/救治规范/技术规范/操作规程/防控).
  4. For each notice page: extract inline body text, or download the PDF
     attachment and extract text via pypdf (scanned PDFs are skipped).

Output: external/nhc/guides/{slug}.json {title, url, year, content}
Idempotent: skips existing files.
"""
import json, os, re, sys, time, io, urllib.request, urllib.parse, http.cookiejar, html as H
from pathlib import Path

OUT = Path(__file__).parent / "nhc" / "guides"
CHROME = os.path.expanduser(
    "~/Library/Caches/ms-playwright/chromium-1234/chrome-mac-arm64/"
    "Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing")
UA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/151.0.0.0 Safari/537.36"
BASE = "https://www.nhc.gov.cn"
MAX_PAGES = 8  # per section

COLS = ["ylyjs/zcwj", "ylyjs/gzdt"]
GUIDE_RE = re.compile(r"(诊疗方案|诊疗指南|防治指南|防控方案|防控指南|救治规范|技术规范|操作规程|诊疗规范|防治规范|管理指南)")
SKIP_TITLE_RE = re.compile(r"(解读|宣传|活动|倡议书|表彰|公告|通报|名单|培训|试点|名单|公示)")


def harvest_cookies():
    from playwright.sync_api import sync_playwright
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True, executable_path=CHROME,
                                    args=["--disable-blink-features=AutomationControlled", "--no-sandbox"])
        ctx = browser.new_context(user_agent=UA, locale="zh-CN")
        page = ctx.new_page()
        page.goto(BASE + "/", timeout=60000, wait_until="domcontentloaded")
        page.wait_for_timeout(8000)
        cookies = ctx.cookies()
        browser.close()
    return cookies


def build_opener(cookies):
    cj = http.cookiejar.CookieJar()
    for c in cookies:
        try:
            cj.set_cookie(http.cookiejar.Cookie(
                0, c["name"], c["value"], None, False,
                c.get("domain", "").lstrip("."), True, False,
                c.get("path", "/"), True, bool(c.get("secure")),
                c.get("expires"), False, None, None, {}))
        except Exception:
            pass
    return urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))


_opener = None


def fetch(opener, url, referer=BASE + "/"):
    """Fetch with WAF-cookie refresh on 412 (cookies expire mid-run)."""
    global _opener
    for attempt in range(3):
        try:
            req = urllib.request.Request(url, headers={
                "User-Agent": UA, "Referer": referer,
                "Accept": "text/html,*/*;q=0.8", "Accept-Language": "zh-CN,zh;q=0.9"})
            return opener.open(req, timeout=60).read()
        except urllib.error.HTTPError as e:
            if e.code == 412 and attempt < 2:
                print("   [cookie 刷新]", file=sys.stderr)
                _opener = build_opener(harvest_cookies())
                opener = _opener
                time.sleep(3)
                continue
            raise


def extract_inline(html):
    """Extract the guideline body text from a notice page."""
    body = re.sub(r"<script[^>]*>.*?</script>", "", html, flags=re.S)
    body = re.sub(r"<style[^>]*>.*?</style>", "", body, flags=re.S)
    text = H.unescape(re.sub(r"<[^>]+>", "\n", body))
    lines = [l.strip() for l in text.split("\n") if l.strip()]
    full = "\n".join(lines)
    for marker in ["一、病原学", "一、流行病学", "一、概述", "一、总则",
                   "一、定义", "一、病因", "二、流行病学", "一、疾病概述"]:
        i = full.find(marker)
        if i > 0:
            return full[i:]
    return full


def looks_like_guide(content):
    """Guide body contains numbered sections like 一、/二、/三、."""
    if len(content) < 800:
        return False
    return bool(re.search(r"[一二三四五六七八九十]、", content))


def pdf_text(data):
    from pypdf import PdfReader
    r = PdfReader(io.BytesIO(data))
    text = "\n".join((p.extract_text() or "") for p in r.pages)
    return text.strip()


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    print("获取 WAF cookie...", file=sys.stderr)
    global _opener
    _opener = build_opener(harvest_cookies())
    opener = _opener

    # 1) 收集候选通知链接
    candidates = {}  # url -> title
    for col in COLS:
        for page_n in range(1, MAX_PAGES + 1):
            list_url = f"{BASE}/{col}/new_list.shtml" if page_n == 1 else f"{BASE}/{col}/new_list_{page_n}.shtml"
            try:
                html = fetch(opener, list_url, referer=f"{BASE}/{col}/new_list.shtml").decode("utf-8", "replace")
            except Exception as e:
                print(f"列表页 {list_url[-40:]}: {str(e)[:40]}", file=sys.stderr)
                break
            items = re.findall(r'href="(/[^"]*/20\d{4}/[^"]*\.shtml)"[^>]*>([^<]{6,100})</a>', html)
            new = 0
            for u, t in items:
                t = H.unescape(t).strip().replace("\n", " ")
                full = BASE + u
                if full in candidates:
                    continue
                if not GUIDE_RE.search(t) or SKIP_TITLE_RE.search(t):
                    continue
                candidates[full] = t
                new += 1
            print(f"[{col}] 页{page_n}: {new} 个新候选 (累计 {len(candidates)})", file=sys.stderr)
            if new == 0:
                break
            time.sleep(0.5)

    print(f"\n共发现 {len(candidates)} 个指南通知", file=sys.stderr)

    # 2) 逐个抓取
    ok = skip = 0
    for i, (url, title) in enumerate(candidates.items(), 1):
        m = re.search(r"/(20\d{4})/", url)
        year = m.group(1)[:4] if m else ""
        slug = re.sub(r"[^\w\u4e00-\u9fff]", "-", title)[:40]
        out = OUT / f"{slug}.json"
        if out.exists():
            skip += 1
            continue
        try:
            data = fetch(opener, url, referer=BASE + "/")
            html = data.decode("utf-8", "replace")
            content = extract_inline(html)
            if not looks_like_guide(content):
                pdfs = re.findall(r'href="([^"]*\.(?:pdf|docx?))"', html)
                if pdfs:
                    rel = pdfs[0]
                    base_dir = url.rsplit("/", 1)[0]
                    pdf_url = urllib.parse.quote(base_dir + "/" + rel.lstrip("/"), safe=":/?&=%")
                    try:
                        pdf_data = fetch(opener, pdf_url, referer=url)
                        content = pdf_text(pdf_data)
                    except Exception:
                        content = ""
            if not looks_like_guide(content):
                print(f"[{i}/{len(candidates)}] ⚠️ 跳过(无方案正文/扫描版): {title[:40]}", file=sys.stderr)
                continue
            out.write_text(json.dumps(
                {"title": title, "url": url, "year": year, "content": content},
                ensure_ascii=False), encoding="utf-8")
            ok += 1
            print(f"[{i}/{len(candidates)}] ✅ {title[:44]} ({len(content)}字)", file=sys.stderr)
        except Exception as e:
            print(f"[{i}/{len(candidates)}] ❌ {title[:40]}: {str(e)[:60]}", file=sys.stderr)
        time.sleep(0.8)

    print(f"\n完成: 成功 {ok}, 已存在 {skip}, 共 {len(candidates)}", file=sys.stderr)


if __name__ == "__main__":
    main()
