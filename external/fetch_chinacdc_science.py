#!/usr/bin/env python3
"""中国疾控中心 健康科普栏目正文抓取 → external/chinacdc_science/raw/*.txt

列表页 JS 渲染: 用 playwright 打开每个栏目页, 从渲染后 DOM 取 <a> 的**绝对
href**(关键: 相对 href 会丢掉子栏目目录如 /jkkp/yyjk/rqyy/, 直接拼接必 404)。
文章页静态: urllib 直连, 正文取 div.TRS_Editor, 元信息取 div.xqCon
(时间/供稿)。正文中文数 < 400 的丢弃。每抓成一篇立即写盘并追加 MANIFEST。

用法: python3 external/fetch_chinacdc_science.py [栏目slug ...]
"""
import asyncio
import html as htmlmod
import re
import sys
import time
import urllib.request
from pathlib import Path

BASE = Path(__file__).parent
OUT = BASE / "chinacdc_science" / "raw"
UA = {"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
                    "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"}

# 优先级排序: 慢病/营养/烟草/环境在前
COLUMNS = [
    ("mxfcrb", "慢性病与防治", "https://www.chinacdc.cn/jkkp/mxfcrb/"),
    ("yyjk",   "营养健康",     "https://www.chinacdc.cn/jkkp/yyjk/"),
    ("yckz",   "烟草控制",     "https://www.chinacdc.cn/jkkp/yckz/"),
    ("hjjk",   "环境健康",     "https://www.chinacdc.cn/jkkp/hjjk/"),
    ("zyjk",   "职业健康",     "https://www.chinacdc.cn/jkkp/zyjk/"),
    ("ggws",   "公共卫生",     "https://www.chinacdc.cn/jkkp/ggws/"),
    ("crb",    "传染病",       "https://www.chinacdc.cn/jkkp/crb/"),
    ("mygh",   "母婴健康",     "https://www.chinacdc.cn/jkkp/mygh/"),
    ("fsws",   "消毒",         "https://www.chinacdc.cn/jkkp/fsws/"),
    ("jkts",   "健康提示",     "https://www.chinacdc.cn/jkts/"),
]
ART_RE = re.compile(r"^https://www\.chinacdc\.cn/(jkkp|jkts)/[A-Za-z0-9_/]*t(\d{8})_(\d+)\.html$")
MIN_CJK = 400


def http_get(url: str, timeout: int = 30) -> str:
    req = urllib.request.Request(url, headers=UA)
    for i in range(3):
        try:
            with urllib.request.urlopen(req, timeout=timeout) as r:
                return r.read().decode("utf-8", "replace")
        except Exception:
            if i == 2:
                raise
            time.sleep(1.5 * (i + 1))
    return ""


def cjk_count(s: str) -> int:
    return sum(1 for c in s if "\u4e00" <= c <= "\u9fff")


def clean_block(seg: str) -> str:
    """HTML 片段 → 纯文本: 块级标签换行, 行内标签直接抹除(避免引文断裂)。"""
    s = re.sub(r"<(script|style)[^>]*>.*?</\1>", "", seg, flags=re.S)
    s = re.sub(r"<br\s*/?>|</(p|div|h\d|li|tr|blockquote)>", "\x00", s)
    s = re.sub(r"<[^>]+>", "", s)
    s = htmlmod.unescape(s)
    lines = []
    for ln in s.split("\x00"):
        ln = re.sub(r"\s*\n\s*", "", ln).strip()
        ln = re.sub(r"[ \t\u3000]+", lambda m: " " if " " in m.group(0) else "\u3000", ln)
        if ln:
            lines.append(ln)
    return "\n".join(lines)


def _arts_from(html_urls):
    out, seen = [], set()
    for href, text in html_urls:
        href = href.split("#")[0]
        m = ART_RE.match(href)
        if not m or href in seen:
            continue
        seen.add(href)
        col = href.split("/")[4] if m.group(1) == "jkkp" else "jkts"
        out.append((href, text, col))
    return out


async def harvest() -> dict:
    """返回 {col_slug: [(abs_url, anchor_text, col), ...]} — 栏目页 + 其子栏目页。"""
    from playwright.async_api import async_playwright
    result: dict[str, list] = {}
    async with async_playwright() as pw:
        b = await pw.chromium.launch(headless=True)
        p = await (await b.new_context(user_agent=UA["User-Agent"])).new_page()

        async def page_arts(url):
            await p.goto(url, wait_until="domcontentloaded", timeout=45000)
            await p.wait_for_timeout(3200)
            arts = await p.evaluate(
                """() => Array.from(document.querySelectorAll('a'))
                    .filter(a => /\\/t\\d{8}_\\d+\\.html/.test(a.href))
                    .map(a => [a.href, a.textContent.trim()])"""
            )
            subs = await p.evaluate(
                """() => [...new Set(Array.from(document.querySelectorAll('a'))
                    .map(a => a.href.replace(/index\\.s?html$/,''))
                    .filter(h => /^https:\\/\\/www\\.chinacdc\\.cn\\/jkkp\\/[a-z]+\\/[a-z]+\\/$/.test(h)))]"""
            )
            return arts, subs

        only = set(sys.argv[1:])
        for slug, name, url in COLUMNS:
            if only and slug not in only:
                continue
            for attempt in (1, 2):  # 每栏目整页最多 2 次尝试
                try:
                    arts, subs = await page_arts(url)
                    collected = _arts_from(arts)
                    base_col = url.rstrip("/").split("/")[-1]
                    sub_urls = [s for s in subs if f"/jkkp/{base_col}/" in s][:6]
                    for su in sub_urls:
                        try:
                            await asyncio.sleep(0.9)
                            sarts, _ = await page_arts(su)
                            collected += _arts_from(sarts)
                        except Exception as e:
                            print(f"  [{slug}] 子栏目失败 {su}: {str(e)[:60]}", file=sys.stderr)
                    print(f"[{slug}] attempt{attempt}: {len(collected)} 候选"
                          f" ({len(sub_urls)} 子栏目)", file=sys.stderr)
                    if collected:
                        merged = {x[0]: x for x in result.get(slug, [])}
                        for x in collected:
                            merged.setdefault(x[0], x)
                        result[slug] = list(merged.values())
                        break
                except Exception as e:
                    print(f"[{slug}] attempt{attempt} 列表失败: {str(e)[:80]}", file=sys.stderr)
                    await asyncio.sleep(2)
            else:
                print(f"[{slug}] 放弃", file=sys.stderr)
            await asyncio.sleep(1)
        await b.close()
    return result


def parse_article(url: str) -> tuple | None:
    """返回 (title, date, source_org, body) 或 None。"""
    raw = http_get(url)
    # class 可能是 <div class=TRS_Editor>(无引号, TRS 老模板)或 trs_editor_view TRS_UEDITOR(新模板)
    BODY_CLS = r"(?:TRS_Editor|trs_editor_view|TRS_UEDITOR)"
    m = re.search(r'<div[^>]*class\s*=\s*"?[^">]*' + BODY_CLS + r'[^">]*"?\s*>(.*?)'
                  r'(?=<div[^>]*class="[^"]*(?:fanye|related|wzFooter|footer|xqlist)|<script)',
                  raw, re.S)
    if not m:
        m2 = re.search(r'<div[^>]*class\s*=\s*"?[^">]*' + BODY_CLS + r'[^">]*"?\s*>(.*)', raw, re.S)
        if not m2:
            # 兜底: id="articleCon" 容器
            m3 = re.search(r'<div[^>]*id="articleCon"[^>]*>(.*?)'
                           r'(?=<div[^>]*class="[^"]*(?:wzFooter|fanye|footer))', raw, re.S)
            if not m3:
                return None
            m2 = type("M", (), {"group": lambda self, n, s=m3: s.group(1)})()
        seg = m2.group(1)
        for stop in ["上一篇", "下一篇", "【打印", "相关文章", "版权声明", "wzFooter"]:
            i = seg.find(stop)
            if i > 500:
                seg = seg[:i]
        m_body = seg
    else:
        m_body = m.group(1)
    body = clean_block(m_body)
    if cjk_count(body) < MIN_CJK:
        return None
    # 标题: 页面 h5/h1 优先, 兜底 xqCon 前
    t = re.search(r"<h5[^>]*>(.*?)</h5>", raw, re.S) or re.search(r"<h1[^>]*>(.*?)</h1>", raw, re.S)
    title = clean_block(t.group(1)).split("\n")[0] if t else ""
    title = re.sub(r"\s*·?\s*中国疾病预防控制中心\s*$", "", title).strip()
    # 元信息块
    date, org = "", ""
    xm = re.search(r'<div[^>]*class="[^"]*xqCon[^"]*"[^>]*>(.{0,600}?)</div>', raw, re.S)
    if xm:
        blk = xm.group(1)
        dm = re.search(r"时间[:：]\s*<em>([^<]*)</em>", blk)
        if dm:
            date = dm.group(1).strip()
        sm = re.search(r"来\s*源[:：]\s*<em>([^<]*)</em>", blk)
        gm = re.search(r"供稿[:：]\s*<em>([^<]*)</em>", blk)
        org = (sm or gm or re.search(r"<em>([^<]{4,})</em>", blk))
        org = org.group(1).strip() if org else ""
    if not date:
        dm = re.search(r"t(\d{4})(\d{2})(\d{2})_\d+\.html", url)
        if dm:
            date = "%s-%s-%s" % dm.groups()
    if not title:
        tt = re.search(r"<title>([^<]*)</title>", raw)
        title = clean_block(tt.group(1)) if tt else url.rsplit("/", 1)[-1]
    return title, date, org, body


def main() -> int:
    OUT.mkdir(parents=True, exist_ok=True)
    manifest = OUT / "MANIFEST.txt"
    cols = asyncio.run(harvest())
    ok = dropped = 0
    done_urls = set()
    if manifest.exists():
        for ln in manifest.read_text(encoding="utf-8").splitlines():
            if ln.startswith("#"):  # 排除/不可达记录 — 修复解析后可重试
                continue
            done_urls.update(re.findall(r"https://\S+", ln))
    for slug, arts in cols.items():
        for url, anchor, col in arts:
            if url in done_urls:
                continue
            stem = re.sub(r"[./]", "", url.rsplit("/", 1)[-1])[:-4]  # t20260309_315315 -> t20260309_315315
            fname = "%s_%s.txt" % (col, stem)
            path = OUT / fname
            try:
                parsed = parse_article(url)
            except Exception as e:
                with manifest.open("a", encoding="utf-8") as fh:
                    fh.write("# 不可达: %s %s\n" % (url, str(e)[:60]))
                dropped += 1
                time.sleep(0.8)
                continue
            if not parsed:
                with manifest.open("a", encoding="utf-8") as fh:
                    fh.write("# 已排除: %s 正文<%d中文字或无正文容器\n" % (url, MIN_CJK))
                dropped += 1
                time.sleep(0.8)
                continue
            title, date, org, body = parsed
            source_line = "中国疾病预防控制中心 %s" % ("，".join([x for x in [org] if x and x not in title]))
            source_line = (source_line.rstrip("， ")).replace("中国疾病预防控制中心 ，", "中国疾病预防控制中心 ")
            text = "# URL: %s\n# 标题: %s\n# 发布方/日期: %s，%s\n\n%s\n" % (url, title, source_line, date, body)
            path.write_text(text, encoding="utf-8")
            with manifest.open("a", encoding="utf-8") as fh:
                fh.write("%s | %s | %s | %s | %s\n" % (fname, title, url, org or "中国疾病预防控制中心", date))
            ok += 1
            done_urls.add(url)
            print("OK", fname, cjk_count(body), file=sys.stderr)
            time.sleep(0.85)
    print(f"新抓 {ok}, 丢弃/不可达 {dropped}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
