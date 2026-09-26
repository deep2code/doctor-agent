#!/usr/bin/env python3
"""Download WHO (世卫组织) Chinese health content NOT already covered by the
fact-sheet pipeline (external/fetch_who_factsheets_zh.py fetches
/zh/news-room/fact-sheets/item/* — those are already in the KB; this script
deliberately skips them).

Three categories, by priority:
  1. Health-topic pages   https://www.who.int/zh/health-topics/<slug>
     (slugs enumerated from the /zh/health-topics landing page HTML)
  2. Questions & answers  https://www.who.int/zh/news-room/questions-and-answers/item/<slug>
     (slugs from Sitefinity API /api/hubs/qandagroups, paged with $skip)
  3. News items           https://www.who.int/zh/news/item/<slug>
     (from Sitefinity API /api/news/newsitems, paged with $skip;
      purely administrative / meeting / appointment items are skipped)

Output: external/who_zh_health/raw/<file>.txt + raw/MANIFEST.txt
Each txt starts with 3 meta lines (# URL / # 标题 / # 发布方/日期), then a
blank line, then verbatim body text (no rewriting, no translation).
Idempotent: existing files are skipped. Requests are throttled >=0.6s.
"""
import json
import re
import sys
import time
import urllib.request
from html.parser import HTMLParser
from pathlib import Path

OUT = Path(__file__).parent / "who_zh_health" / "raw"
UA = {"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
      "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"}
SLEEP = 0.65
MIN_CJK = 300

BASE = "https://www.who.int"
TOPICS_LIST = BASE + "/zh/health-topics"
QA_API = (BASE + "/api/hubs/qandagroups?sf_site=15210d59-ad60-47ff-a542-"
          "7ed76645f0c7&sf_provider=OpenAccessProvider&sf_culture=zh"
          "&$orderby=PublicationDateAndTime%20desc"
          "&$select=Title,ItemDefaultUrl,FormatedDate&$top=100&$skip={skip}")
NEWS_API = (BASE + "/api/news/newsitems?sf_provider=OpenAccessDataProvider"
            "&sf_culture=zh&$orderby=PublicationDateAndTime%20desc"
            "&$select=Title,ItemDefaultUrl,FormatedDate,NewsType"
            "&$top=100&$skip={skip}")
QA_ITEM = BASE + "/zh/news-room/questions-and-answers/item{slug}"
TOPIC_ITEM = BASE + "/zh/health-topics/{slug}"
NEWS_ITEM = BASE + "/zh/news/item{slug}"

# news titles matching this are pure administration/meetings — excluded
ADMIN_RE = re.compile(
    r"任命|执行委员会|世界卫生大会|理事机构|预算|捐款|认捐|就职|讣告|悼念|"
    r"办事处开幕|伙伴关系|续签|奖项|周年纪念活动|工作人员招聘|"
    r"谈判|会议召开前夕|缔结|执行理事会|区域委员会|高级别会议签署|仪式")


class BaseParser(HTMLParser):
    """Collect text between start/end of a tracked region."""
    def __init__(self):
        super().__init__()
        self.cur = []
        self.active = False
        self.depth = 0
        self.skip = 0
        self.blocks = []

    def _track_noise(self, tag):
        if self.active and tag in ("script", "style", "nav"):
            self.skip += 1

    def handle_data(self, data):
        if self.active and not self.skip:
            t = data.strip()
            if t:
                self.cur.append(t)

    def _flush(self):
        t = "\n".join(self.cur).strip()
        if t:
            self.blocks.append(t)
        self.cur = []


class DivText(BaseParser):
    """Full text (incl. nested divs) of the first div whose class token list
    contains `cls`."""
    def __init__(self, cls):
        super().__init__()
        self.cls = cls
        self.done = False

    def handle_starttag(self, tag, attrs):
        if tag == "div" and not self.done:
            a = dict(attrs)
            toks = (a.get("class") or "").split()
            if self.cls in toks:
                self.active = True
                self.depth = 1
                self.cur = []
                self._track_noise(tag)
                return
        if self.active:
            if tag == "div":
                self.depth += 1
            self._track_noise(tag)

    def handle_endtag(self, tag):
        if not self.active:
            return
        if tag in ("script", "style", "nav") and self.skip:
            self.skip -= 1
            return
        if tag == "div":
            self.depth -= 1
            if self.depth == 0:
                self.active = False
                self.done = True
                self._flush()


class ArticleText(BaseParser):
    """Text of the first <article> element."""
    def __init__(self):
        super().__init__()
        self.done = False

    def handle_starttag(self, tag, attrs):
        if tag == "article":
            if not self.done and not self.active:
                self.active = True
                self.depth = 1
                self.cur = []
                return
            if self.active:
                self.depth += 1
        if self.active and tag in ("script", "style", "nav"):
            self.skip += 1

    def handle_endtag(self, tag):
        if not self.active:
            return
        if tag in ("script", "style", "nav") and self.skip:
            self.skip -= 1
            return
        if tag == "article":
            self.depth -= 1
            if self.depth == 0:
                self.active = False
                self.done = True
                self._flush()


class Panels(BaseParser):
    """Each div.sf-accordion__panel as its own block (question + answer)."""
    def handle_starttag(self, tag, attrs):
        if tag == "div":
            a = dict(attrs)
            if not self.active and "sf-accordion__panel" in (a.get("class") or "").split():
                self.active = True
                self.depth = 1
                self.cur = []
                return
            if self.active:
                self.depth += 1
        if self.active and tag in ("script", "style", "nav"):
            self.skip += 1

    def handle_endtag(self, tag):
        if not self.active:
            return
        if tag in ("script", "style", "nav") and self.skip:
            self.skip -= 1
            return
        if tag == "div":
            self.depth -= 1
            if self.depth == 0:
                self.active = False
                self._flush()


def cjk_count(text):
    return len(re.findall(r"[\u4e00-\u9fff]", text))


def get(url, attempts=2):
    last = None
    for i in range(attempts):
        try:
            req = urllib.request.Request(url, headers=UA)
            with urllib.request.urlopen(req, timeout=30) as r:
                return r.read().decode("utf-8", "replace")
        except Exception as e:  # noqa: BLE001
            last = e
            if i < attempts - 1:
                time.sleep(1.5)
    raise last  # noqa: RSE01


def page_title(html):
    m = re.search(r"<h1[^>]*>(.*?)</h1>", html, re.S)
    if not m:
        return ""
    return re.sub(r"\s+", " ", re.sub(r"<[^>]+>", "", m.group(1))).strip()


def clean_body(text):
    # collapse >2 blank lines, strip leftover whitespace noise
    text = re.sub(r"\n{3,}", "\n\n", text)
    return text.strip() + "\n"


class Manifest:
    def __init__(self, path):
        self.path = path
        self.fh = open(path, "a", encoding="utf-8")

    def line(self, s):
        self.fh.write(s + "\n")
        self.fh.flush()

    def close(self):
        self.fh.close()


def save(fname, url, title, publisher_date, body, man):
    if cjk_count(body) < MIN_CJK:
        man.line(f"# 已排除: {fname} 正文中文字数不足300 ({cjk_count(body)})")
        return False
    out = OUT / fname
    out.write_text(f"# URL: {url}\n# 标题: {title}\n"
                   f"# 发布方/日期: {publisher_date}\n\n{clean_body(body)}",
                   encoding="utf-8")
    man.line(f"{fname} | {title} | {url} | 世界卫生组织 | {publisher_date.split('（', 1)[-1].rstrip('）') if '（' in publisher_date else publisher_date}")
    return True


def fetch_topics(man, limit=None):
    ok = fail = 0
    try:
        html = get(TOPICS_LIST)
    except Exception as e:  # noqa: BLE001
        man.line(f"# 不可达: www.who.int /zh/health-topics 列表页 {e}")
        return 0, 0
    slugs = sorted(set(re.findall(r'href="https://www\.who\.int/zh/health-topics/([a-z0-9\-]+)"', html)))
    slugs = [s for s in slugs if s not in ("a-z-list",)]
    print(f"[topics] {len(slugs)} slugs", file=sys.stderr)
    for slug in slugs[:limit or len(slugs)]:
        url = TOPIC_ITEM.format(slug=slug)
        try:
            html = get(url)
        except Exception as e:  # noqa: BLE001
            man.line(f"# 已排除: topic {slug} 不可达 {e}")
            fail += 1
            time.sleep(SLEEP)
            continue
        time.sleep(SLEEP)
        ex = DivText("dynamic-content__section-content")
        ex.feed(html)
        body = ex.blocks[0] if ex.blocks else ""
        if cjk_count(body) < 50:
            ex2 = ArticleText()
            ex2.feed(html)
            body = ex2.blocks[0] if ex2.blocks else ""
        title = page_title(html) or slug
        # date: WHO topic pages carry no reliable visible date
        d = re.search(r"(?:最近审阅|更新日期)[：: ]*((?:19|20)\d{2}[^<\n ]{0,12})", html)
        pd = f"世界卫生组织（{d.group(1).strip()}）" if d else "世界卫生组织（无）"
        fname = f"topic_{slug}.txt"
        if (OUT / fname).exists():
            ok += 1
            continue
        if save(fname, url, title, pd, body, man):
            ok += 1
        else:
            fail += 1
    return ok, fail


def fetch_qa(man, limit=None):
    ok = fail = 0
    items = []
    for skip in (0, 100, 200):
        try:
            data = json.loads(get(QA_API.format(skip=skip)))
        except Exception as e:  # noqa: BLE001
            man.line(f"# 不可达: www.who.int qandagroups API skip={skip} {e}")
            break
        batch = data.get("value", [])
        items.extend(batch)
        if len(batch) < 100:
            break
        time.sleep(SLEEP)
    print(f"[qa] {len(items)} groups", file=sys.stderr)
    for it in items[:limit or len(items)]:
        slug = it["ItemDefaultUrl"]
        url = QA_ITEM.format(slug=slug)
        fname = f"qa_{slug.strip('/')}.txt"
        if (OUT / fname).exists():
            ok += 1
            continue
        try:
            html = get(url)
        except Exception as e:  # noqa: BLE001
            man.line(f"# 已排除: qa {slug} 不可达 {e}")
            fail += 1
            time.sleep(SLEEP)
            continue
        time.sleep(SLEEP)
        p = Panels()
        p.feed(html)
        body = "\n\n".join(p.blocks)
        if cjk_count(body) < 50:
            ex = ArticleText()
            ex.feed(html)
            body = "\n".join(ex.blocks)
        title = it.get("Title") or page_title(html) or slug
        pd = f"世界卫生组织（{it.get('FormatedDate') or '无'}）"
        if save(fname, url, title, pd, body, man):
            ok += 1
        else:
            fail += 1
    return ok, fail


def fetch_news(man, limit=None):
    ok = fail = 0
    items = []
    for skip in (0, 100):
        try:
            data = json.loads(get(NEWS_API.format(skip=skip)))
        except Exception as e:  # noqa: BLE001
            man.line(f"# 不可达: www.who.int newsitems API skip={skip} {e}")
            break
        batch = data.get("value", [])
        items.extend(batch)
        if len(batch) < 100:
            break
        time.sleep(SLEEP)
    print(f"[news] {len(items)} items", file=sys.stderr)
    done = 0
    for it in items:
        slug = it["ItemDefaultUrl"]
        title = it.get("Title") or ""
        ntype = it.get("NewsType") or ""
        if ADMIN_RE.search(title) or ntype in ("演讲", "突发卫生事件", "情况报告"):
            man.line(f"# 已排除: news {slug} 纯行政/会议/演讲类 ({ntype})")
            continue
        url = NEWS_ITEM.format(slug=slug)
        fname = f"news_{slug.strip('/').replace('/', '_')}.txt"
        if (OUT / fname).exists():
            ok += 1
            done += 1
            if limit and done >= limit:
                break
            continue
        try:
            html = get(url)
        except Exception as e:  # noqa: BLE001
            man.line(f"# 已排除: news {slug} 不可达 {e}")
            fail += 1
            time.sleep(SLEEP)
            continue
        time.sleep(SLEEP)
        ex = ArticleText()
        ex.feed(html)
        body = "\n\n".join(ex.blocks)
        if cjk_count(body) < 50:
            d = DivText("news-content-item")
            d.feed(html)
            body = "\n\n".join(d.blocks)
        pd = f"世界卫生组织（{it.get('FormatedDate') or '无'}）"
        if save(fname, url, title or page_title(html), pd, body, man):
            ok += 1
            done += 1
        else:
            fail += 1
            continue
        if limit and done >= limit:
            break
    return ok, fail


def main():
    limit = None
    if len(sys.argv) > 1 and sys.argv[1].isdigit():
        limit = int(sys.argv[1])
    which = sys.argv[2] if len(sys.argv) > 2 else "all"
    OUT.mkdir(parents=True, exist_ok=True)
    man = Manifest(OUT / "MANIFEST.txt")
    tot = {}
    if which in ("all", "topics"):
        tot["topics"] = fetch_topics(man, limit)
    if which in ("all", "qa"):
        tot["qa"] = fetch_qa(man, limit)
    if which in ("all", "news"):
        tot["news"] = fetch_news(man, limit)
    man.close()
    print(tot, file=sys.stderr)


if __name__ == "__main__":
    main()
