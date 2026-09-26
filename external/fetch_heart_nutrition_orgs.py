#!/usr/bin/env python3
"""Fetch public Chinese health-education texts from three authoritative sources
into external/heart_nutrition_orgs/raw/*.txt — source harvesting ONLY (no
knowledge entries are written here).

Sources (verified reachable 2026-09):
  * 国家心血管病中心  https://www.nccd.org.cn/   (JSON column API, no WAF)
  * 中国医学科学院阜外医院 https://www.fuwaihospital.org/ (in-site /News/ pages only;
    the many mp.weixin.qq.com entries on its 健康科普 list are skipped by design)
  * 中国营养学会 https://www.cnsoc.org/ and its 膳食指南 official subsite
    http://dg.cnsoc.org/

Each saved file has the mandated traceability header:
    # URL: ...
    # 标题: ...
    # 发布方/日期: ...
followed by the verbatim page text (numbers/units untouched).

Idempotent: existing files are skipped. MANIFEST.txt is regenerated from the
raw/*.txt headers plus accumulated skip notes.

Usage: python3 fetch_heart_nutrition_orgs.py [nccd|fuwai|cnsoc|dg|pdfs|manifest]
Request interval >= 0.9s, strictly serial.
"""
import html as htmllib
import json
import re
import sys
import time
import urllib.parse
import urllib.request
from html.parser import HTMLParser
from pathlib import Path

BASE = Path(__file__).parent
RAW = BASE / "heart_nutrition_orgs" / "raw"
MANIFEST = RAW / "MANIFEST.txt"
NOTES = BASE / "heart_nutrition_orgs" / ".skips.log"  # transient skip log, merged into MANIFEST at `manifest`
UA = {"User-Agent": "Mozilla/5.0 (research; contact: self)"}
INTERVAL = 0.9
_last = [0.0]


def polite():
    dt = time.time() - _last[0]
    if dt < INTERVAL:
        time.sleep(INTERVAL - dt)
    _last[0] = time.time()


def get(url, timeout=45, retries=2):
    for i in range(retries + 1):
        try:
            polite()
            req = urllib.request.Request(url, headers=UA)
            with urllib.request.urlopen(req, timeout=timeout) as r:
                return r.read().decode("utf-8", "replace")
        except Exception as e:  # noqa: BLE001
            if i == retries:
                raise
            time.sleep(2)


def post_json(url, data, retries=2):
    for i in range(retries + 1):
        try:
            polite()
            req = urllib.request.Request(
                url, urllib.parse.urlencode(data).encode(), UA
            )
            with urllib.request.urlopen(req, timeout=45) as r:
                return json.loads(r.read().decode("utf-8", "replace"))
        except Exception:
            if i == retries:
                raise
            time.sleep(2)


def cjk_count(text):
    return len(re.findall(r"[\u4e00-\u9fff]", text))


# ---------------------------------------------------------------- extraction

class BlockParser(HTMLParser):
    """Extract text of ONE container element (matched by predicate). Block
    boundaries become newlines; table cells are separated by ' | '. Script and
    style subtrees are skipped."""

    def __init__(self, pred):
        super().__init__(convert_charrefs=True)
        self.pred = pred
        self.found = False
        self.depth = 0
        self.skip = 0
        self.parts = []

    def handle_starttag(self, tag, attrs):
        a = dict(attrs)
        if not self.found:
            if self.pred(tag, a):
                self.found = True
                self.depth = 1
        elif tag in ("script", "style", "textarea"):
            self.skip += 1
        else:
            self.depth += 1
            if tag in ("p", "div", "br", "li", "tr", "h1", "h2", "h3", "h4",
                       "h5", "table", "blockquote", "dl", "dt", "dd", "ul", "ol"):
                self.parts.append("\n")
            if tag in ("td", "th"):
                self.parts.append(" | ")

    def handle_endtag(self, tag):
        if not self.found:
            return
        if tag in ("script", "style", "textarea") and self.skip:
            self.skip -= 1
            return
        self.depth -= 1
        if tag in ("p", "div", "li", "tr", "h1", "h2", "h3", "h4", "table"):
            self.parts.append("\n")
        if self.depth <= 0:
            self.found = "closed"

    def handle_data(self, data):
        if self.found is True and not self.skip:
            self.parts.append(data)

    def text(self):
        t = "".join(self.parts)
        t = re.sub(r"[ \t\u3000]+", " ", t)
        t = re.sub(r" ?\n ?", "\n", t)
        t = re.sub(r"\n{2,}", "\n\n", t)
        return t.strip()


def extract_container(html, pred):
    p = BlockParser(pred)
    p.feed(html)
    return p.text()


def by_class(cls):
    return lambda tag, a: cls in (a.get("class") or "")


def flatten_fragment(fragment):
    """Full-document-agnostic text of an HTML snippet (NCCD API Content)."""
    p = BlockParser(lambda tag, a: tag == "body" or tag == "html")
    # snippets have no body/html wrapper; fall back: wrap them
    p.feed("<body>" + fragment + "</body>")
    return p.text()


def plain_lines(html):
    body = re.sub(r"<script.*?</script>|<style.*?</style>", "", html, flags=re.S)
    body = body[body.find("<body"):] if "<body" in body else body
    txt = re.sub(r"<[^>]+>", "\n", body)
    txt = htmllib.unescape(txt).replace("\u3000", " ").replace("\xa0", " ")
    return [ln.strip() for ln in txt.splitlines() if ln.strip()]


def clean_cells(t):
    t = re.sub(r"^ *(?:\| *)+", "", t)
    t = re.sub(r"(?: *\| *)+$", "", t)
    return t.strip()


# ---------------------------------------------------------------- saving

def slugify(s):
    return re.sub(r"[^0-9A-Za-z.\-]+", "-", s)[:60]


def save(rel, url, title, orgdate, body):
    """Quality gate + write. Returns True on save, 'dup' if exists, else False."""
    out = RAW / rel
    if out.exists():
        return "dup"
    body = body.strip()
    if cjk_count(body) < 400:
        note("U", url, title, f"正文仅{cjk_count(body)}中文字(<400)")
        return False
    RAW.mkdir(parents=True, exist_ok=True)
    out.write_text(
        f"# URL: {url}\n# 标题: {title}\n# 发布方/日期: {orgdate}\n\n{body}\n",
        encoding="utf-8",
    )
    print(f"SAVED {rel} ({cjk_count(body)} cjk)")
    return True


def note(kind, url, title, why):
    with open(NOTES, "a", encoding="utf-8") as f:
        f.write(f"{kind}\t{url}\t{title}\t{why}\n")
    print(f"SKIP[{kind}] {url} | {title[:30]} | {why}")


# ---------------------------------------------------------------- NCCD

NCCD_COLS = {1367: "kp", 1086: "news"}


def phase_nccd():
    for colid, tag in NCCD_COLS.items():
        for page in (1, 2, 3):
            try:
                d = post_json(
                    "https://www.nccd.org.cn/Common/GetColumnImport",
                    {"id": colid, "page": page, "isEnglish": "false"},
                )
            except Exception as e:  # noqa: BLE001
                note("U", f"nccd column {colid} p{page}", "-", str(e))
                break
            try:
                arts = json.loads(d[1]) if isinstance(d[1], str) else d[1]
            except Exception:  # noqa: BLE001
                break
            if not arts:
                break
            for a in arts:
                aid = a["ArticleId"]
                title = (a.get("Title") or "").strip()
                if a.get("IsExternalLink"):
                    ext = a.get("ExternalLink") or ""
                    if "mp.weixin" in ext:
                        note("E", f"https://www.nccd.org.cn/News/Articles/Index/{aid}",
                             title, "外链微信公众号，本机取不到正文，按规则跳过")
                    else:
                        note("E", ext or "?", title, "外部链接(非站内正文页)，跳过")
                    continue
                frag = a.get("Content") or ""
                body = flatten_fragment(frag)
                date = (a.get("ReleaseDate") or "")[:10]
                src = a.get("Source") or ""
                url = f"https://www.nccd.org.cn/News/Articles/Index/{aid}"
                save(
                    f"nccd-{tag}-{aid}.txt", url, title,
                    f"国家心血管病中心（www.nccd.org.cn，栏目:{src or colid}）{date}",
                    body,
                )
            if page >= 2:  # column lists beyond page2 are legacy; keep probe count small
                break


# ---------------------------------------------------------------- Fuwai

FUWAI_FOOTER_IDS = {"3", "4", "5", "201808", "208361", "201760", "210510"}


def phase_fuwai():
    h = get("https://www.fuwaihospital.org/News/Main?siteId=107")
    ids = []
    for m in re.finditer(r'href="(/News/Articles/Index/(\d+))"', h):
        if m.group(2) not in FUWAI_FOOTER_IDS and m.group(2) not in ids:
            ids.append(m.group(2))
    for aid in ids:
        url = f"https://www.fuwaihospital.org/News/Articles/Index/{aid}"
        try:
            page = get(url)
        except Exception as e:  # noqa: BLE001
            note("U", url, "-", str(e))
            continue
        if "信息不存在" in page:
            note("U", url, "-", "页面已失效(信息不存在)")
            continue
        t = re.search(r"<h1>\s*(.*?)</h1>", page, re.S)
        title = re.sub(r"\s+", " ", t.group(1)).strip() if t else f"fuwai-{aid}"
        mdate = re.search(r"发布时间[：:]\s*([\d\-]+)[\d: ]*", page)
        msrc = re.search(r"来源[：:]\s*</span>|来源[：:]\s*([^<\n]{2,30})", page)
        msrc2 = re.search(r"<span>来源：([^<]*)</span>", page)
        src = (msrc2.group(1).strip() if msrc2 else "") or "阜外医院"
        cat = re.search(r'class="act">([^<]+)<', page)
        body = extract_container(page, lambda tag, a: a.get("id") == "zoom")
        body = clean_cells(body)
        date = mdate.group(1) if mdate else "?"
        save(
            f"fuwai-{aid}.txt", url, title,
            f"中国医学科学院阜外医院（栏目:{cat.group(1).strip() if cat else '健康科普'}；页面来源:{src}）{date}",
            body,
        )


# ---------------------------------------------------------------- CNSOC

CNSOC_LISTS = [
    "https://www.cnsoc.org/scienpopuln/",
    "https://www.cnsoc.org/publicac/",
    "https://www.cnsoc.org/learnnews/",
    "https://www.cnsoc.org/acadconfn/",
    "https://www.cnsoc.org/othernews/",
]
CNSOC_LINK_RE = re.compile(r'https://www\.cnsoc\.org/([a-z0-9]+)/(\d+)\.html')
CNSOC_SKIP_CATS = {"about", "politicalnews", "cparty", "workplan", "bg"}


def phase_cnsoc():
    urls = {}
    for lst in CNSOC_LISTS:
        try:
            h = get(lst)
        except Exception as e:  # noqa: BLE001
            note("U", lst, "-", str(e))
            continue
        for cat, num in CNSOC_LINK_RE.findall(h):
            if cat in CNSOC_SKIP_CATS or cat.endswith("video"):
                continue
            urls.setdefault(f"https://www.cnsoc.org/{cat}/{num}.html", None)
    for url in sorted(urls):
        try:
            page = get(url)
        except Exception as e:  # noqa: BLE001
            note("U", url, "-", str(e))
            continue
        t = re.search(r"<title>(.*?)_[^<]*_中国营养学会官网</title>", page, re.S)
        title = t.group(1).strip() if t else url
        if "会员" in title or "点击查看" in title:
            note("E", url, title, "会员登录墙，无公开正文")
            continue
        mdate = re.search(r"发布时间[：:]\s*([\d\-]+)", page)
        body = extract_container(page, by_class("content_main"))
        body = re.sub(r"阅读次数[：:]?\s*\d*", "", body)
        body = clean_cells(body)
        save(
            f"cnsoc-{url.split('/')[-2]}-{url.split('/')[-1][:-5]}.txt", url, title,
            f"中国营养学会（www.cnsoc.org）{mdate.group(1) if mdate else '?'}",
            body,
        )


# ---------------------------------------------------------------- dg.cnsoc.org

def phase_dg():
    urls = {}
    dates = {}
    for lst in [
        "http://dg.cnsoc.org/",
        "http://dg.cnsoc.org/gzdtnewslist_0403_2_1.htm",
        "http://dg.cnsoc.org/gzdtnewslist_0403_2_2.htm",
        "http://dg.cnsoc.org/newslist_0506_1.htm",
        "http://dg.cnsoc.org/newslist_0503_1.htm",
    ]:
        try:
            h = get(lst)
        except Exception as e:  # noqa: BLE001
            note("U", lst, "-", str(e))
            continue
        for m in re.finditer(
            r'href="((?:\.\./\.\./|http://dg\.cnsoc\.org/|)?article/0[45]/[^"]+\.html)"', h
        ):
            u = m.group(1)
            if u.startswith("http://"):
                u = u.split("dg.cnsoc.org")[-1].lstrip("/")
            u = "http://dg.cnsoc.org/" + u.replace("../", "").lstrip("/")
            if u.startswith("http://dg.cnsoc.org/article/"):
                urls[u] = True
        for m in re.finditer(
            r'<dd><span>\[?([\d-]+)\]?</span><a href="([^"]*article/0[45]/[^"]+\.html)"', h
        ):
            u = m.group(2)
            if u.startswith("http://"):
                u = u.split("dg.cnsoc.org")[-1]
            u = "http://dg.cnsoc.org/" + u.replace("../", "").lstrip("/")
            dates[u] = m.group(1)
    for url in sorted(urls):
        try:
            page = get(url)
        except Exception as e:  # noqa: BLE001
            note("U", url, "-", str(e))
            continue
        t = re.search(r"<title>(.*?)_[^<]*</title>", page, re.S)
        title = t.group(1).strip() if t else url
        lines = plain_lines(page)
        try:
            s = next(i for i, ln in enumerate(lines) if ln.startswith("发布时间"))
        except StopIteration:
            note("U", url, title, "无发布时间锚点，结构未识别")
            continue
        body = lines[s + 1:]
        # footer: 首页 / 专家委员会 …  or 联系电话/京ICP
        cut = len(body)
        for i, ln in enumerate(body):
            if ln in ("首页", "专家委员会", "联系电话：010-83554781、83554782、83554783"):
                cut = i
                break
        body = [ln for ln in body[:cut] if ln not in ("首页", "指南动态", "新指南解读")]
        text = "\n".join(body)
        save(
            "dg-" + slugify(url.split("/")[-1][:-5]) + ".txt", url, title,
            f"中国营养学会·中国居民膳食指南官网（dg.cnsoc.org）{dates.get(url, '2022')}",
            text,
        )


# ---------------------------------------------------------------- NCCD PDFs

def extract_pdf_text(path):
    import pypdf

    rd = pypdf.PdfReader(str(path))
    chunks = []
    for pg in rd.pages:
        try:
            chunks.append(pg.extract_text() or "")
        except Exception:  # noqa: BLE001
            chunks.append("")
    return "\n".join(chunks)


PDFS = [
    ("nccd-report-2024", "中国心血管健康与疾病报告2024",
     "https://www.nccd.org.cn/Sites/Uploaded/File/2025/12/%E4%B8%AD%E5%9B%BD%E5%BF%83%E8%A1%80%E7%AE%A1%E5%81%A5%E5%BA%B7%E4%B8%8E%E7%96%BE%E7%97%85%E6%8A%A5%E5%91%8A2024.pdf",
     "国家心血管病中心 2025-11-19 发布"),
    ("nccd-report-2023", "中国心血管健康与疾病报告2023",
     "https://www.nccd.org.cn/Sites/Uploaded/File/2024/12/%E4%B8%AD%E5%9B%BD%E5%BF%83%E8%A1%80%E7%AE%A1%E5%81%A5%E5%BA%B7%E4%B8%8E%E7%96%BE%E7%97%85%E6%8A%A5%E5%91%8A2023.pdf",
     "国家心血管病中心 2024-11-25 发布"),
]


def phase_pdfs():
    import pypdf  # noqa: F401  (fail fast if missing)

    for rel, title, url, orgdate in PDFS:
        if (RAW / (rel + ".txt")).exists():
            continue
        tmp = Path("/tmp") / (rel + ".pdf")
        try:
            polite()
            urllib.request.urlretrieve(url, tmp)  # noqa: S310
        except Exception as e:  # noqa: BLE001
            note("U", url, title, f"PDF下载失败 {e}")
            continue
        try:
            text = extract_pdf_text(tmp)
        except Exception as e:  # noqa: BLE001
            note("U", url, title, f"PDF解析失败 {e}")
            continue
        if cjk_count(text) < 2000:
            note("U", url, title, f"PDF无可读文字层(仅{cjk_count(text)}字)")
            continue
        (RAW / (rel + ".txt")).write_text(
            f"# URL: {url}\n# 标题: {title}\n# 发布方/日期: {orgdate}\n\n{text}\n",
            encoding="utf-8",
        )
        print(f"SAVED {rel}.txt ({cjk_count(text)} cjk)")
        tmp.unlink(missing_ok=True)


# ---------------------------------------------------------------- manifest

def phase_manifest():
    rows, skips = [], []
    for f in sorted(RAW.glob("*.txt")):
        if f.name == "MANIFEST.txt":
            continue
        head = f.read_text(encoding="utf-8").splitlines()
        url = title = orgdate = "?"
        for ln in head[:3]:
            if ln.startswith("# URL:"):
                url = ln[7:].strip()
            elif ln.startswith("# 标题:"):
                title = ln[6:].strip()
            elif ln.startswith("# 发布方/日期:"):
                orgdate = ln[9:].strip()
        dm = re.search(r"20\d{2}(?:-\d{2}){0,2}", orgdate)
        if dm:
            org, date = orgdate[:dm.start()].strip(), dm.group(0)
        else:
            org, date = orgdate, "?"
        rows.append((f.name, title, url, org, date))
    if NOTES.exists():
        skips = NOTES.read_text(encoding="utf-8").splitlines()
    with open(MANIFEST, "w", encoding="utf-8") as out:
        out.write("# external/heart_nutrition_orgs/raw/MANIFEST.txt\n")
        out.write("# 国家心血管病中心 / 阜外医院站内页 / 中国营养学会(cnsoc.org+dg.cnsoc.org) 公众科普正文清单\n")
        out.write("# 格式: 相对文件名 | 标题 | 直链 | 发布机构 | 日期\n")
        for r in rows:
            out.write(" | ".join(r) + "\n")
    if skips:
        with open(MANIFEST, "a", encoding="utf-8") as out:
            ex, un = [], []
            for ln in skips:
                p = ln.split("\t")
                if len(p) != 4:
                    continue
                (ex if p[0] == "E" else un).append(
                    f"{p[1]} | {p[2]} | {p[3]}"
                )
            out.write("# 已排除:\n")
            out.write("\n".join("# " + x for x in sorted(set(ex))) + "\n")
            out.write("# 不可达:\n")
            out.write("\n".join("# " + x for x in sorted(set(un))) + "\n")
    print(f"manifest: {len(rows)} files")


if __name__ == "__main__":
    phase = sys.argv[1] if len(sys.argv) > 1 else "all"
    for p in ([phase] if phase != "all" else ["nccd", "fuwai", "cnsoc", "dg", "pdfs", "manifest"]):
        {"nccd": phase_nccd, "fuwai": phase_fuwai, "cnsoc": phase_cnsoc,
         "dg": phase_dg, "pdfs": phase_pdfs, "manifest": phase_manifest}[p]()
    if phase != "manifest" and phase != "all":
        phase_manifest()
