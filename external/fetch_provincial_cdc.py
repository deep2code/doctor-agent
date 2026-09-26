#!/usr/bin/env python3
"""Fetch public-health science articles from provincial CDC websites (text only).

Sources (7 entry points, incl. one paginated column):
  1. Sichuan CDC  科普园地   https://www.sccdc.cn/Article/List?id=267
  2. Sichuan CDC  疾病预防   https://www.sccdc.cn/Article/List?id=268
  3. Hubei CDC    健康提示   http://www.hbcdc.cn/info/iList.jsp?cat_id=10327
  4. Hubei CDC    健康提示 p2 ...&cur_page=2
  5. Hunan CDC    健康科普   http://www.hncdc.com/html/web//education/index.html
  6. Guizhou CDC  科普宣传   https://www.gzscdc.org.cn/ggfw/jkjy/kpxc
  7. Tianjin CDC  健康提示   https://www.cdctj.com.cn/gzdt/jkts/

Output: external/provincial_cdc/raw/<site>_<id>.txt (+ MANIFEST.txt rebuilt from files).
Verbatim text only; articles with <400 CJK chars in the body container are dropped.
Never touches internal/knowledge/**.
"""
import re
import sys
import time
import pathlib
from urllib.parse import urljoin

import requests
from bs4 import BeautifulSoup

UA = {
    "User-Agent": (
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
        "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
    ),
    "Accept-Language": "zh-CN,zh;q=0.9",
}
DELAY = 0.9
MIN_ZH = 400
ROOT = pathlib.Path(__file__).resolve().parent / "provincial_cdc" / "raw"

PUBLISHER = {
    "sccdc": "四川省疾病预防控制中心",
    "hbcdc": "湖北省疾病预防控制中心",
    "hncdc": "湖南省疾病预防控制中心",
    "gzscdc": "贵州省疾病预防控制中心",
    "tjcdc": "天津市疾病预防控制中心",
}


def fetch(url):
    last = None
    for attempt in range(3):
        try:
            r = requests.get(url, headers=UA, timeout=25, verify=False)  # noqa: S501 (some CDC certs broken)
            r.encoding = r.apparent_encoding
            if r.status_code == 200:
                return r.text
            last = RuntimeError(f"HTTP {r.status_code}")
        except Exception as e:  # network blips on flaky provincial hosts
            last = e
        time.sleep(1.5 + attempt)  # backoff before retry
    raise last


def zh_count(text):
    return len(re.findall(r"[\u4e00-\u9fff]", text))


def clean_body(container):
    """Verbatim-ish plain text of an article body container."""
    for bad in container.find_all(["script", "style", "img", "video", "audio", "source", "iframe", "button", "input"]):
        bad.decompose()
    parts = []
    blocks = container.find_all(["p", "li", "h1", "h2", "h3", "h4", "td", "div", "blockquote", "pre"])
    if blocks:
        # keep leaf-ish blocks in document order, skip wrappers already covered by inner blocks
        for b in blocks:
            if b.find_all(["p", "div", "li"]):
                continue
            t = b.get_text(" ", strip=True)
            t = re.sub(r"\s+", " ", t)
            if t:
                parts.append(t)
    if not parts:
        t = container.get_text("\n", strip=True)
        t = re.sub(r"[ \t\u3000]+", " ", t)
        parts = [line.strip() for line in t.splitlines() if line.strip()]
    # drop common page furniture patterns that leak in
    drop = re.compile(r"(版权所有|ICP备|京公网安备|网站地图|审核|编辑[:：]|分享[:：]|点击量|浏览次数|字号|打印|二维码|扫一扫)")
    kept = []
    for p in parts:
        if len(p) < 12 and drop.search(p):
            continue
        kept.append(p)
    text = "\n".join(kept)
    if container.find("p") is None and container.name in ("div", "td"):
        pass
    return text


def find_date(html, url, code=""):
    # Tianjin TRS pages carry the publish date in the /system/YYYY/MM/DD/ path
    urlm = re.search(r"/system/(\d{4})/(\d{2})/(\d{2})/", url)
    if urlm and code == "tjcdc":
        return f"{urlm.group(1)}-{urlm.group(2)}-{urlm.group(3)}"
    m = re.search(r"(?:发布时间|发布日期|发表日期)\s*[:：]\s*([0-9]{4}[-/年][0-9]{1,2}[-/月][0-9]{1,2}日?(?:\s*[0-9:]{5,8})?)", html)
    if m:
        return m.group(1).strip()
    if urlm:
        return f"{urlm.group(1)}-{urlm.group(2)}-{urlm.group(3)}"
    m = re.search(r"(\d{4}[-/]\d{1,2}[-/]\d{1,2})", html)
    return m.group(1) if m else ""


def find_title(html, soup, fallback_url, code=""):
    def norm(t):
        t = re.sub(r"\s+", " ", t).strip(" -—_|–—　")
        return t if len(t) >= 4 else None

    tg = re.search(r"<title[^>]*>([^<]{4,120})</title>", html)
    title_tag = None
    if tg:
        title_tag = norm(re.split(r"[-_|–—]\s*(?:四川省|湖北省|湖南省|贵州省|天津市)?疾病预防控制中心|_", tg.group(1).strip())[0])
    if code in ("hbcdc", "gzscdc") and title_tag:
        return title_tag
    for sel in ["h1", ".article-title", ".art_title", ".news-title", ".tit"]:
        el = soup.select_one(sel)
        if el:
            t = norm(el.get_text(" ", strip=True))
            if t:
                return t
    return title_tag or fallback_url


# ---------------- per-site list harvesting ----------------

def list_sccdc(url, code):
    html = fetch(url)
    out = []
    for m in re.finditer(r"/Article/View\?id=(\d+)", html):
        aid = m.group(1)
        if int(aid) < 20000 or aid in ("26002", "26003"):  # negative ids, disclaimers, static pages
            continue
        out.append((f"{code}:{aid}", "https://www.sccdc.cn/Article/View?id=" + aid))
    seen = set()
    dedup = []
    for k, u in out:
        if k in seen:
            continue
        seen.add(k)
        dedup.append((k, u))
    return dedup


def list_hbcdc(url, code):
    html = fetch(url)
    soup = BeautifulSoup(html, "lxml")
    out = []
    for a in soup.find_all("a", href=True):
        href = a["href"]
        if re.search(r"/jk(jy|kj)[^/]*/\d+\.htm", href) or re.search(r"/jkjy/(jkts|jkkp)(/jkts)?/\d+\.htm", href):
            out.append((f"{code}:{href}", urljoin(url, href)))
    return out


def list_hncdc(url, code):
    html = fetch(url)
    out = []
    seen = set()
    soup = BeautifulSoup(html, "lxml")
    for a in soup.find_all("a", href=True):
        u = urljoin(url, a["href"])
        m = re.search(r"education/jiankangtishi/(\d+)\.html", u)
        if m and m.group(1) not in seen:
            seen.add(m.group(1))
            out.append((f"{code}:{m.group(1)}", u))
    return out


def list_gzscdc(url, code):
    html = fetch(url)
    soup = BeautifulSoup(html, "lxml")
    out = []
    for a in soup.find_all("a", href=True):
        u = urljoin(url, a["href"])
        m = re.search(r"/ggfw/jkjy/kpxc/(?:[a-z]+/)?content_(\d+)", u)
        if m:
            out.append((f"{code}:{m.group(1)}", u))
    return out


def list_tjcdc(url, code):
    html = fetch(url)
    soup = BeautifulSoup(html, "lxml")
    out = []
    for a in soup.find_all("a", href=True):
        u = urljoin(url, a["href"])
        m = re.search(r"/system/\d{4}/\d{2}/\d{2}/(\d+)\.shtml", u)
        if m:
            out.append((f"{code}:{m.group(1)}", u))
    return out


# ---------------- article body selectors ----------------

def body_of(code, html, soup):
    if code == "sccdc":
        sels = ["div.view1-box-main-content"]
    elif code == "hbcdc":
        sels = ["div#articleBody"]
    elif code == "hncdc":
        sels = ["div.article-detail"]
    elif code == "gzscdc":
        sels = ["div.conTxt"]
    elif code == "tjcdc":
        # Tianjin TRS layout: several td.hanggao30 cells; the body cell also carries .xiahua
        sels = ["td.hanggao30.xiahua", "td.xiahua"]
    else:
        return None
    best = None
    for sel in sels:
        for el in soup.select(sel):
            body = clean_body(el)
            zc = zh_count(body)
            if best is None or zc > best[1]:
                best = (body, zc)
    return best[0] if best else None


ENTRIES = [
    ("sccdc", list_sccdc, "https://www.sccdc.cn/Article/List?id=267"),
    ("sccdc", list_sccdc, "https://www.sccdc.cn/Article/List?id=268"),
    ("hbcdc", list_hbcdc, "http://www.hbcdc.cn/info/iList.jsp?cat_id=10327"),
    ("hbcdc", list_hbcdc, "http://www.hbcdc.cn/info/iList.jsp?site_id=CMShbjkzx&cat_id=10327&cur_page=2"),
    ("hncdc", list_hncdc, "http://www.hncdc.com/html/web//education/index.html"),
    ("gzscdc", list_gzscdc, "https://www.gzscdc.org.cn/ggfw/jkjy/kpxc"),
    ("tjcdc", list_tjcdc, "https://www.cdctj.com.cn/gzdt/jkts/"),
]

# sites probed and abandoned before building ENTRIES (recorded in MANIFEST)
STANDING_NOTES = [
    "# 不可达: https://www.cdc.zj.cn/ 浙江省疾控——首页与栏目全JS渲染(0个静态<a>)，无静态列表可枚举，按最多2次尝试原则放弃",
    "# 不可达: https://www.jscdc.cn/ 江苏疾控——健康科普资源库为内网IP SPA(218.94.1.76)，站点栏目页正文<600字且无静态科普列表页，放弃",
    "# 不可达: https://www.gzcdc.org.cn/ 广州疾控——科普知识列表(/education/health.html)正文全部托管在微信公众号(mp.weixin.qq.com，图片版海报)，站内 /education/view/*.html 正文容器为空(<p>&nbsp;</p>)，无逐字文字可取，放弃",
    "# 说明: https://www.cdctj.com.cn/ 天津疾控——文章直链经CDN回源不稳定404，抓成的条目保留，失败的在下方逐条记录",
]


def main():
    ROOT.mkdir(parents=True, exist_ok=True)
    todo = []
    seen = set()
    excluded = []
    for code, fn, url in ENTRIES:
        try:
            links = fn(url, code)
        except Exception as e:
            print(f"[list FAIL] {url} {e}")
            excluded.append(f"# 不可达: {url} 列表页抓取失败 {e}")
            continue
        print(f"[list] {url} -> {len(links)} links")
        for key, art in links:
            if key in seen:
                continue
            seen.add(key)
            todo.append((code, key, art))
    got = 0
    for code, key, art in todo:
        num = key.split(":", 1)[1].rsplit("/", 1)[-1]
        path = ROOT / f"{code}_{num}.txt"
        if path.exists():
            print(f"[skip existing] {path.name}")
            continue
        try:
            html = fetch(art)
        except Exception as e:
            print(f"[fetch FAIL] {art} {e}")
            excluded.append(f"# 已排除: {art} 页面抓取失败 {e}")
            time.sleep(DELAY)
            continue
        soup = BeautifulSoup(html, "lxml")
        body = body_of(code, html, soup)
        if not body:
            print(f"[drop] {art} no body container")
            excluded.append(f"# 已排除: {art} 正文容器为空(图片版/视频页)")
            time.sleep(DELAY)
            continue
        nz = zh_count(body)
        if nz < MIN_ZH:
            print(f"[drop zh={nz}] {art}")
            excluded.append(f"# 已排除: {art} 正文仅{nz}中文字(<{MIN_ZH})")
            time.sleep(DELAY)
            continue
        title = find_title(html, BeautifulSoup(html, "lxml"), art, code)
        date = find_date(html, art, code)
        text = f"# URL: {art}\n# 标题: {title}\n# 发布方/日期: {PUBLISHER[code]} {date}\n\n{body}\n"
        tmp = path.with_suffix(".part")
        tmp.write_text(text, encoding="utf-8")
        tmp.rename(path)
        got += 1
        print(f"[saved] {path.name} zh={nz} | {title[:40]}")
        time.sleep(DELAY)

    # rebuild manifest from files on disk (idempotent)
    lines = []
    for p in sorted(ROOT.glob("*.txt")):
        if p.name == "MANIFEST.txt":
            continue
        head = p.read_text(encoding="utf-8").splitlines()
        try:
            url = head[0].split("URL: ", 1)[1]
            title = head[1].split("标题: ", 1)[1]
            pubdate = head[2].split("发布方/日期: ", 1)[1]
            pub, _, d = pubdate.partition(" ")
        except IndexError:
            continue
        lines.append(f"{p.name} | {title} | {url} | {pub} | {d}")
    manifest = "\n".join(lines)
    notes = STANDING_NOTES + excluded
    seen_note = set()
    uniq = []
    for n in notes:
        if n not in seen_note:
            seen_note.add(n)
            uniq.append(n)
    if uniq:
        manifest += "\n" + "\n".join(uniq)
    (ROOT / "MANIFEST.txt").write_text(manifest + "\n", encoding="utf-8")
    total = len(list(ROOT.glob("*.txt"))) - 1
    print(f"DONE this run: +{got}; files on disk: {total}")


if __name__ == "__main__":
    main()
