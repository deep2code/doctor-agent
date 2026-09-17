#!/usr/bin/env python3
"""Fetch clinical guidelines (诊疗方案/临床指南) from nhc.gov.cn.

nhc.gov.cn sits behind a JS-cookie WAF (412 for plain curl). Strategy:
  1. Playwright (real Chromium) loads the homepage -> executes the JS
     challenge -> we harvest the WAF cookies.
  2. urllib reuses those cookies to fetch notice pages and their PDF
     attachments.

Output: external/nhc/{slug}.pdf (+ notice page text in external/nhc/pages/).
Idempotent: skips existing PDFs.
"""
import json, os, re, sys, time, urllib.request, http.cookiejar
from pathlib import Path

OUT = Path(__file__).parent / "nhc"
PAGES = OUT / "pages"
CHROME = os.path.expanduser(
    "~/Library/Caches/ms-playwright/chromium-1234/chrome-mac-arm64/"
    "Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing")
UA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/151.0.0.0 Safari/537.36"

# slug -> (标题, 通知页 URL)
GUIDES = {
    "cerebrovascular-2024": ("脑血管病防治指南(2024年版)",
        "https://www.nhc.gov.cn/ylyjs/zcwj/202412/ba037e931fff4870930f65ff667ea9ed.shtml"),
    "measles-dengue-avian-2024": ("麻疹/登革热/人感染禽流感诊疗方案(2024年版)",
        "https://www.nhc.gov.cn/ylyjs/pqt/202407/4662eb54f6f544338543bf053f9ce049.shtml"),
    "obesity-2024": ("肥胖症诊疗指南(2024年版)",
        "https://www.nhc.gov.cn/yzygj/s7659/202410/ae3948b3fc9444feb2ecd26fb2daa111.shtml"),
    "liver-cancer-2024": ("原发性肝癌诊疗指南(2024年版)",
        "https://www.nhc.gov.cn/yzygj/s7659/202404/653069140ddb4df28cdeba1ff1b86c66.shtml"),
}


def harvest_cookies():
    from playwright.sync_api import sync_playwright
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True, executable_path=CHROME,
                                    args=["--disable-blink-features=AutomationControlled", "--no-sandbox"])
        ctx = browser.new_context(user_agent=UA, locale="zh-CN")
        page = ctx.new_page()
        page.goto("https://www.nhc.gov.cn/", timeout=60000, wait_until="domcontentloaded")
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


def fetch(opener, url, referer="https://www.nhc.gov.cn/"):
    req = urllib.request.Request(url, headers={
        "User-Agent": UA, "Referer": referer,
        "Accept": "text/html,*/*;q=0.8", "Accept-Language": "zh-CN,zh;q=0.9"})
    return opener.open(req, timeout=60).read()


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    PAGES.mkdir(parents=True, exist_ok=True)
    print("获取 WAF cookie (playwright)...", file=sys.stderr)
    cookies = harvest_cookies()
    opener = build_opener(cookies)

    for slug, (title, url) in GUIDES.items():
        pdf_path = OUT / f"{slug}.pdf"
        if pdf_path.exists():
            print(f"SKIP {slug} (已存在)", file=sys.stderr)
            continue
        try:
            data = fetch(opener, url)
            html = data.decode("utf-8", "replace")
            (PAGES / f"{slug}.html").write_text(html, encoding="utf-8")
            # 提取附件 PDF(相对路径)
            m = re.search(r'href="([^"]*\.(?:pdf|docx?))"', html)
            if not m:
                m = re.search(r'"([^"]*(?:W0\d+|files/)[^"]*\.(?:pdf|docx?))"', html)
            if not m:
                print(f"[{slug}] ❌ 未找到附件: {title}", file=sys.stderr)
                continue
            rel = m.group(1)
            if rel.startswith("http"):
                pdf_url = rel
            else:
                base = url.rsplit("/", 1)[0]
                pdf_url = base + "/" + rel.lstrip("/")
            pdf_data = fetch(opener, pdf_url, referer=url)
            if pdf_data[:4] == b"%PDF" or len(pdf_data) > 20000:
                pdf_path.write_bytes(pdf_data)
                print(f"[{slug}] ✅ {title}: {len(pdf_data)//1024}KB ({rel[-40:]})", file=sys.stderr)
            else:
                print(f"[{slug}] ⚠️ 非 PDF 响应 {len(pdf_data)}B: {rel[:60]}", file=sys.stderr)
        except Exception as e:
            print(f"[{slug}] ❌ {str(e)[:100]}", file=sys.stderr)
        time.sleep(1)


if __name__ == "__main__":
    main()
