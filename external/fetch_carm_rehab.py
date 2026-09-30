#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Fetch clinical rehabilitation content from 中国康复医学会官网 (www.carm.org.cn).

TRS/JPaaS CMS. Channel index pages render their list via
/api-gateway/jpaas-publish-server/front/page/build/unit (see unitbuild.js):
we extract each page's `queryData` script attrs and re-call that endpoint with
paramJson={"pageNo":1,"pageSize":99999} to enumerate the FULL channel archive.

Output: external/carm_rehab/raw/*.txt (3-line header + body, one para per line)
        external/carm_rehab/MANIFEST.txt (incl. 已排除 section)
Resume: re-run the same command; URLs already in MANIFEST (kept or excluded)
are skipped.

Usage: python3 external/fetch_carm_rehab.py [--channels-only] [--no-fetch]
"""
import html as htmlmod
import json
import os
import re
import sys
import threading
import time
import urllib.parse
import urllib.request
from concurrent.futures import ThreadPoolExecutor

BASE = "https://www.carm.org.cn"
UA = {"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
      "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"}
OUT_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "carm_rehab")
RAW_DIR = os.path.join(OUT_DIR, "raw")
MANIFEST = os.path.join(OUT_DIR, "MANIFEST.txt")
ISSUER = "中国康复医学会"

# ---- rate limiter: >=1.0s between request starts, <=2 concurrent ----
_gate = threading.Semaphore(2)
_last = [0.0]
_last_lock = threading.Lock()


def _throttle():
    with _last_lock:
        now = time.time()
        wait = 1.0 - (now - _last[0])
        if wait > 0:
            time.sleep(wait)
        _last[0] = time.time()


def fetch(url, tries=3, timeout=20):
    """GET with retry; returns text or raises."""
    err = None
    for _ in range(tries):
        with _gate:
            _throttle()
            try:
                req = urllib.request.Request(url, headers=UA)
                with urllib.request.urlopen(req, timeout=timeout) as r:
                    return r.read().decode("utf-8", "ignore")
            except Exception as e:  # noqa: BLE001
                err = e
        time.sleep(1.5)
    raise err


# ---------------- content filters ----------------
# channels skipped entirely (org boilerplate / member portal / pure media / noise)
SKIP_CHANNEL_PREFIXES = (
    "/gywm/", "/hyzj/", "/yqlj", "/sjzx", "/jypx/zscx",
    "/kpgy/kpcxzpzb/", "/jypx/tzgg", "/tzgg",
)
# only these channels are enumerated+harvested (clinical-value survey, see MANIFEST notes)
KEEP_CHANNELS = (
    "/kpgy/",            # 科普公益: 科普动态/培训/专题/健康科普/平台资源
    "/bzzd/qtfgzd/",
    "/kyxs/ttbz/", "/kyxs/xsdt/", "/kyxs/zgkfyxnj/",   # 学术: 团体标准/学术动态/年鉴
    "/jypx/kfzlsgfhpx/",
)

# title screening: meeting/admin noise unless a clinical-education signal present
DROP_TITLE = re.compile(
    r"(通知|公告|公示|报名|缴费|议程|日程|证书|换届|理事会|常务|办公会|党支部|党建|廉政"
    r"|表彰|先进|慰问|走访|调研|座谈|拜访|接待|捐赠|招聘|录用|人事|贺信|致辞|开幕|闭幕"
    r"|倒计时|评选|评审|名单|考试|考核|培训班|师资|地址|乘车|志愿者征集|作品征集|参赛作品"
    r"|作品展示|展示活动|成功举办|圆满举办|圆满召开|纪念|周年|学习贯|主题团日|民主生活"
    r"|服务行|走进|回访|侧记|巡诊|帮扶)"
)
KEEP_TITLE = re.compile(
    r"(指南|共识|规范|标准|科普|讲座|问答|解读|知识|训练|康复|预防|识别|居家|护理"
    r"|疼痛|卒中|中风|脊髓|老年|失能|跌倒|吞咽|言语|认知|脑瘫|孤独症|发育|假肢|矫形"
    r"|心肺|癌症|安宁|姑息|健康|义诊|宣传日|残疾)"
)

# body-level meeting-report markers (>=3 occurrences => drop)
MEETING_MARKERS = ["召开", "举办", "出席", "致辞", "参会", "座谈", "调研", "走访",
                   "慰问", "表彰", "颁奖", "签约", "启动仪式", "理事会", "圆满落幕",
                   "圆满结束", "顺利举行", "评选", "评审", "评委", "会长", "理事长",
                   "秘书长", "来自", "走进", "帮扶", "捐赠", "义诊"]
CLINICAL_BODY = re.compile(
    r"(康复|训练|卒中|中风|脊髓|疼痛|老年|失能|跌倒|吞咽|言语|认知|脑瘫|孤独症"
    r"|发育迟缓|假肢|矫形|心肺|癌症|安宁|姑息|肌力|平衡|步行|偏瘫|截瘫|术后|骨折"
    r"|关节|腰椎|颈椎|指南|共识|规范)"
)


def cjk_count(text):
    return sum(1 for ch in text if "\u4e00" <= ch <= "\u9fff")


def extract_zoom(htm):
    """Inner HTML of <div id=\"zoom\"> accounting for nested divs."""
    m = re.search(r'<div[^>]*id="zoom"[^>]*>', htm)
    if not m:
        return None
    i = m.end()
    depth = 1
    tok = re.compile(r"<div\b[^>]*>|</div>", re.I)
    pos = i
    for t in tok.finditer(htm, i):
        if t.group(0).lower().startswith("</"):
            depth -= 1
            if depth == 0:
                pos = t.start()
                break
        else:
            depth += 1
        pos = t.end()
    return htm[i:pos] if depth == 0 else htm[i:]


def html_to_text(seg):
    seg = re.sub(r"<(script|style)\b.*?</\1>", "", seg, flags=re.S | re.I)
    seg = re.sub(r"<br\s*/?>|</p>|</div>|</h[1-6]>|</li>|</tr>", "\n", seg, flags=re.I)
    seg = re.sub(r"<[^>]+>", "", seg)
    seg = htmlmod.unescape(seg)
    lines = []
    for ln in seg.splitlines():
        ln = re.sub(r"\s+", " ", ln).strip()
        if not ln:
            continue
        if re.search(r"(发布时间|【字体|字体[:：]|打印|关闭|来源[:：]|分享到|扫码|扫一扫"
                     r"|版权所有|版权|责任编辑|原创文章|转载请注明出处|关注我们|关注我们"
                     r"| Advertisement|广告)", ln):
            continue
        if cjk_count(ln) < 2 and not re.search(r"[A-Za-z0-9]{2,}", ln):
            continue
        lines.append(ln)
    return "\n".join(lines)


# ---------------- enumeration ----------------
def channel_units(page_html):
    """Yield (api_url, params) for each unitbuild script on a page."""
    out = []
    for tag in re.finditer(r"<script\b[^>]*queryData=.*?>", page_html, re.S):
        t = tag.group(0)
        mu = re.search(r'url="([^"]*build/unit[^"]*)"', t)
        mq = re.search(r'queryData="(.*?)"', t, re.S)
        if not (mu and mq):
            continue
        try:
            qd = json.loads(mq.group(1).replace("'", '"'))
        except Exception:  # noqa: BLE001
            continue
        qd["paramJson"] = json.dumps({"pageNo": 1, "pageSize": 99999})
        out.append((mu.group(1), qd))
    return out


def list_unit(url, qd):
    if url.startswith("/"):
        url = BASE + url
    full = url + "?" + urllib.parse.urlencode(qd)
    body = fetch(full)
    d = json.loads(body)
    return d.get("data", {}).get("html", "") if d.get("success") else ""


LINK_RE = re.compile(r'<a\b[^>]*href="([^"]+)"[^>]*title="([^"]*)"')


def parse_list(list_html):
    """Return list of (abs_url, title) for on-site article pages."""
    items = {}
    for href, title in LINK_RE.findall(list_html):
        title = htmlmod.unescape(title).strip()
        if "/art/" not in href and not href.endswith(".html"):
            continue
        if href.startswith("http"):
            if urllib.parse.urlparse(href).netloc.replace("www.", "") != "carm.org.cn":
                continue
            href = urllib.parse.urlparse(href).path
        if not href.startswith("/"):
            continue
        if not re.search(r"/art_[0-9a-f]{32}\.html$", href):
            continue
        if href not in items or (title and not items[href]):
            items[href] = title
    return sorted(items.items())


# ---------------- main pipeline ----------------
def load_manifest_state():
    done = {}
    if os.path.exists(MANIFEST):
        with open(MANIFEST, encoding="utf-8") as f:
            section = "keep"
            for ln in f:
                if ln.strip().startswith("# 已排除"):
                    section = "drop"
                    continue
                parts = [p.strip() for p in ln.strip().split("|")]
                if section == "keep" and len(parts) >= 3 and parts[2].startswith("http"):
                    done[parts[2]] = ("keep", parts[0])
                elif section == "drop" and len(parts) >= 1 and parts[0].startswith("http"):
                    done[parts[0]] = ("drop", parts[-1] if len(parts) > 1 else "")
    return done


def discover_channels():
    h = fetch(BASE + "/")
    chans = sorted(set(re.findall(r'href="(/[^"]*?/index\.html)"', h)))
    return chans, h


def enumerate_channel(path):
    """Fetch a channel index page and pull all its units' full lists."""
    page = fetch(BASE + path)
    found = {}
    for url, qd in channel_units(page):
        try:
            lh = list_unit(url, qd)
        except Exception as e:  # noqa: BLE001
            print(f"  unit fail {path}: {e}", flush=True)
            continue
        for u, t in parse_list(lh):
            if t and (u not in found or not found[u]):
                found[u] = t
        time.sleep(0.2)
    # fallback: static links inside the raw page too
    for u, t in parse_list(page):
        found.setdefault(u, t)
    return found


def process_article(chan, art_path, seed_title):
    url = BASE + art_path
    aid = re.search(r"art_([0-9a-f]{32})\.html", art_path).group(1)
    fname = f"carm_{aid[:12]}.txt"
    fpath = os.path.join(RAW_DIR, fname)
    try:
        page = fetch(url)
    except Exception as e:  # noqa: BLE001
        return ("drop", url, seed_title, f"fetch fail: {e}", None)
    mt = re.search(r"<title>([^<]+)</title>", page)
    title = htmlmod.unescape(mt.group(1)).strip() if mt else seed_title
    md = re.search(r"发布时间[：:]\s*(\d{4}-\d{2}-\d{2})", page)
    date = md.group(1) if md else ""
    zoom = extract_zoom(page)
    if not zoom:
        return ("drop", url, title, "无正文容器", None)
    body = html_to_text(zoom)
    n = cjk_count(body)
    if n < 400:
        return ("drop", url, title, f"正文过短({n}汉字,多为图片/视频页)", None)
    mc = sum(body.count(mk) for mk in MEETING_MARKERS)
    if mc >= 3 and not re.search(r"(指南|共识|规范)", title or ""):
        return ("drop", url, title, f"会议/活动报道(标记{mc}次)", None)
    if title and DROP_TITLE.search(title) and not re.search(r"(指南|共识|规范|知识|问答)", title):
        return ("drop", url, title, "标题会务/通知类", None)
    if not CLINICAL_BODY.search((title or "") + body[:3000]):
        return ("drop", url, title, "无康复临床内容关键词", None)
    with open(fpath, "w", encoding="utf-8") as f:
        f.write(f"# URL: {url}\n# 标题: {title}\n")
        f.write(f"# 发布方/日期: {ISSUER}，{date}\n" if date
                else f"# 发布方: {ISSUER}\n")
        f.write("\n" + body + "\n")
    return ("keep", url, title, "", (fname, date))


def main():
    os.makedirs(RAW_DIR, exist_ok=True)
    only_channels = "--no-fetch" in sys.argv
    state = load_manifest_state()
    print(f"resume: {len(state)} urls already in manifest", flush=True)

    mlock = threading.Lock()

    def manifest_init():
        if not os.path.exists(MANIFEST):
            with open(MANIFEST, "w", encoding="utf-8") as f:
                f.write("# 中国康复医学会 www.carm.org.cn 全文抓取清单\n")
                f.write("# 格式: 文件名 | 标题 | 直链 | 中国康复医学会 | 日期\n")

    def manifest_line(line, drops=False):
        with mlock:
            manifest_init()
            with open(MANIFEST, "a", encoding="utf-8") as f:
                if drops:
                    need = "# 已排除" not in open(MANIFEST, encoding="utf-8").read()
                    if need:
                        f.write("\n# 已排除\n")
                f.write(line + "\n")

    chans, home_html = discover_channels()
    print(f"channels found: {len(chans)}", flush=True)
    allow = [c for c in chans if any(c.startswith(p) for p in KEEP_CHANNELS)]

    pending = {}   # art_path -> (channel, title)
    ch_fail = []
    cache_path = os.path.join(os.path.dirname(os.path.abspath(__file__)),
                              os.pardir, ".cache", "carm", "candidates.json")
    cache_path = os.path.normpath(cache_path)
    if "--refresh" not in sys.argv and os.path.exists(cache_path):
        with open(cache_path, encoding="utf-8") as f:
            for path, val in json.load(f).items():
                pending[path] = tuple(val)
        print(f"candidates cache: {len(pending)} articles", flush=True)
    else:
        for ch in allow:
            try:
                items = enumerate_channel(ch)
            except Exception as e:  # noqa: BLE001
                ch_fail.append((ch, f"枚举失败: {e}"))
                print(f"FAIL {ch}: {e}", flush=True)
                continue
            print(f"{ch}: {len(items)} articles", flush=True)
            for path, title in items.items():
                pending.setdefault(path, (ch, title))
        os.makedirs(os.path.dirname(cache_path), exist_ok=True)
        with open(cache_path, "w", encoding="utf-8") as f:
            json.dump({k: list(v) for k, v in pending.items()}, f, ensure_ascii=False)
    if only_channels:
        print(f"total candidate articles: {len(pending)}")
        return

    # title pre-screen to cut downloads
    todo, prescreened = [], 0
    manifest_init()
    for path, (ch, title) in pending.items():
        url = BASE + path
        if url in state:
            continue
        if title and DROP_TITLE.search(title) \
                and not re.search(r"(指南|共识|规范|知识|问答)", title):
            state[url] = ("drop", "")
            prescreened += 1
            manifest_line(f"{url} | {title} | 标题预筛:通知/会务类", drops=True)
            continue
        todo.append((ch, path, title))
    print(f"to fetch: {len(todo)} (title-prescreened: {prescreened})", flush=True)

    nkeep = [0]

    def work(item):
        ch, path, title = item
        kind, url, t2, reason, kept = process_article(ch, path, title)
        if kind == "keep":
            fname, date = kept
            manifest_line(f"{fname} | {t2} | {url} | {ISSUER} | {date}")
            nkeep[0] += 1
        else:
            manifest_line(f"{url} | {t2} | {reason}", drops=True)
        print(f"[{kind}] {t2[:40]} — {reason or 'ok'}", flush=True)

    with ThreadPoolExecutor(max_workers=2) as ex:
        list(ex.map(work, todo))

    for ch, why in ch_fail:
        manifest_line(f"{BASE + ch} | 频道列表 | 频道{why}", drops=True)
    print(f"done: {nkeep[0]} new kept files this run", flush=True)


if __name__ == "__main__":
    main()
