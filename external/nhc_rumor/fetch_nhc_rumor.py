#!/usr/bin/env python3
"""Fetch all entries of the NHC 健康科普辟谣平台 (rumor-debunking platform).

Pipeline:
  1. List      — POST https://www.nhc.gov.cn/openapi/commonSearchWeb, page size 10,
                 until an empty page; dedupe by url then title.
                 The endpoint sits behind a RuiShu WAF (plain HTTP gets HTTP 412),
                 so the list is normally maintained in list.json (cached JSON array
                 of {title,url,pubDate,second_column,codeName,id}). If list.json is
                 missing this script tries the API directly and fails with a clear
                 message when the WAF blocks it — refresh list.json from a real
                 browser session (same-origin fetch) and re-run.
  2. Content   — mp.weixin.qq.com article pages are fetched verbatim with curl;
                 body = <div id="js_content"> plain text (tags stripped, no rewrite).
  3. Site pages— www.nhc.gov.cn/kppypt/*.shtml are WAF-blocked for curl: they are
                 only recorded in the 「站内待抓」 section of MANIFEST.txt.
  4. Dirty urls— unparseable entries (filehelper.weixin.qq.com, nhc.gov.cn URLs
                 missing a slash etc.) go to the 「坏链」 section.

Files written (nothing else):
  raw/<slug>_<标题前10字>.txt   header: 4 '#' lines, blank line, verbatim body
  MANIFEST.txt                  入库 / 丢弃 / 坏链 / 站内待抓 sections
  state.json                    url -> outcome, makes re-runs idempotent

Guards: >=1.5s between network hits, sequential (concurrency <= 2), WeChat blocks
trigger back-off (max 2 retries) instead of hammering. Bodies with <300 Chinese
characters are dropped (image/poster pages).

Usage:  python3 fetch_nhc_rumor.py [--force]   (--force re-fetches known urls)
"""

import html
import json
import re
import subprocess
import sys
import time
from pathlib import Path
from urllib.parse import urlparse, parse_qs

BASE = Path(__file__).resolve().parent
RAW = BASE / "raw"
LIST = BASE / "list.json"
STATE = BASE / "state.json"
MANIFEST = BASE / "MANIFEST.txt"

MIN_INTERVAL = 1.6          # seconds between network requests (>=1.5 required)
MIN_CN_CHARS = 300          # drop shorter bodies (poster/image pages)
UA = ("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 "
      "(KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")

API = "https://www.nhc.gov.cn/openapi/commonSearchWeb"
API_BODY = {
    "num": 1, "size": 10, "userName": "apicommonuser",
    "userPwd": "d1ec9d42ac73f8960ad13e24776e7970", "siteCode": "N000001835",
    "searchWord": "", "searchRange": 1, "sort": 0, "label": "",
    "startTime": "", "endTime": "",
}

_last_hit = [0.0]


def throttle():
    wait = MIN_INTERVAL - (time.monotonic() - _last_hit[0])
    if wait > 0:
        time.sleep(wait)
    _last_hit[0] = time.monotonic()


# ---------------------------------------------------------------- list

def try_api_list():
    """Best-effort direct API paging; the WAF usually returns 412 to non-browser
    clients, in which case we abort and ask for a refreshed list.json."""
    import urllib.request
    items = []
    num = 1
    while True:
        throttle()
        body = dict(API_BODY, num=num)
        req = urllib.request.Request(
            API, data=json.dumps(body).encode(), method="POST",
            headers={"Content-Type": "application/json", "User-Agent": UA})
        try:
            with urllib.request.urlopen(req, timeout=30) as r:
                data = json.loads(r.read().decode("utf-8", "replace"))
        except Exception as e:                       # noqa: BLE001
            sys.exit(f"❌ 列表接口不可直连（WAF 412 或网络错误: {e}）。\n"
                     f"   请用真浏览器同源 fetch 刷新 {LIST.name} 后重跑。")
        arr = (data or {}).get("searchTotal") or []
        if not arr:
            break
        for d in arr:
            items.append({"title": d.get("title", ""), "url": d.get("url", ""),
                          "pubDate": d.get("pubDate", ""),
                          "second_column": d.get("second_column", ""),
                          "codeName": d.get("codeName", ""), "id": d.get("id", "")})
        num += 1
    return items


def load_list():
    if LIST.exists():
        items = json.loads(LIST.read_text(encoding="utf-8"))
        print(f"列表: 使用缓存 {LIST.name}（{len(items)} 条去重前）")
    else:
        print("列表: list.json 缺失，尝试直连接口…")
        items = try_api_list()
        print(f"列表: 接口取到 {len(items)} 条")
    seen_u, seen_t, uniq = set(), set(), []
    for it in items:
        u, t = it.get("url", ""), it.get("title", "")
        if not u or u in seen_u or t in seen_t:
            continue
        seen_u.add(u)
        seen_t.add(t)
        uniq.append(it)
    print(f"列表: 去重后 {len(uniq)} 条")
    return uniq


# ---------------------------------------------------------------- classify

WEIXIN_RE = re.compile(r"https://mp\.weixin\.qq\.com/s[?/][^\s\"']+")

def classify(url):
    """-> (kind, real_url)  kind in {wechat, nhc_site, bad}"""
    m = WEIXIN_RE.search(url)          # also rescues 'nhc.gov.cn <weixin url>' dirt
    if m:
        return "wechat", m.group(0)
    if re.match(r"https://www\.nhc\.gov\.cn/kppypt/", url):
        return "nhc_site", url
    return "bad", url


def slug_of(url):
    p = urlparse(url)
    if p.path.startswith("/s/"):
        h = p.path[3:].split("?")[0]
        return re.sub(r"[^0-9A-Za-z_-]", "", h)[:10] or "wx"
    sn = parse_qs(p.query).get("sn", [""])[0]
    if sn:
        return re.sub(r"[^0-9A-Za-z_-]", "", sn)[:10]
    return re.sub(r"[^0-9A-Za-z]", "", url)[-10:] or "wx"


def stem_title(title):
    t = re.sub(r"[\\/:*?\"<>|\s【】《》（）()\[\]…]+", "", title)[:10]
    return t or "untitled"


# ---------------------------------------------------------------- fetch weixin

BLOCK_MARKERS = ("当前环境异常", "请求异常", "去验证", "该内容已被发布者删除",
                 "此内容因无法验证而不能展示", "参数错误")


def curl(url, timeout=40):
    throttle()
    r = subprocess.run(
        ["curl", "-sL", "--compressed", "-A", UA, "--max-time", str(timeout), url],
        capture_output=True, text=False)
    if r.returncode != 0:
        return None, f"curl exit {r.returncode}"
    body = r.stdout.decode("utf-8", "replace")
    if not body.strip():
        return None, "empty response"
    return body, None


def extract_js_content(page):
    """Plain text of <div id="js_content"> without bs4 dependency."""
    i = page.find('id="js_content"')
    if i < 0:
        return None
    start = page.find(">", i)
    if start < 0:
        return None
    # walk to the matching close tag of this div
    depth, pos, end = 1, start + 1, None
    while pos < len(page):
        nxt_open = page.find("<div", pos)
        nxt_close = page.find("</div>", pos)
        if nxt_close < 0:
            return None
        if 0 <= nxt_open < nxt_close:
            depth += 1
            pos = nxt_open + 4
        else:
            depth -= 1
            pos = nxt_close + 6
            if depth == 0:
                end = nxt_close
                break
    if end is None:
        return None
    frag = page[start + 1:end]
    frag = re.sub(r"<(script|style)[^>]*>.*?</\1>", "", frag,
                  flags=re.S | re.I)
    frag = re.sub(r"<br[^>]*>", "\n", frag, flags=re.I)
    frag = re.sub(r"</(p|div|section|li|h\d)>", "\n", frag, flags=re.I)
    frag = re.sub(r"<[^>]+>", "", frag)
    frag = html.unescape(frag)
    lines = [ln.strip().replace("\u200b", "").replace("\xa0", " ")
             for ln in frag.splitlines()]
    out, blank = [], 0
    for ln in lines:
        if ln:
            out.append(ln)
            blank = 0
        else:
            blank += 1
            if blank == 1:
                out.append("")
    return "\n".join(out).strip()


def cn_count(text):
    return len(re.findall(r"[\u4e00-\u9fff]", text))


def fetch_weixin(url):
    """-> (body_text, fail_reason). Backs off on blocks; never hammers."""
    delay = 8.0
    for attempt in range(3):
        page, err = curl(url)
        if page is None:
            if attempt < 2:
                print(f"    网络失败({err})，{delay:.0f}s 后重试")
                time.sleep(delay)
                delay *= 2
                continue
            return None, f"抓取失败: {err}"
        body = extract_js_content(page)
        if body and cn_count(body) > 0:
            return body, None
        if any(m in page for m in BLOCK_MARKERS):
            mnext = next((m for m in BLOCK_MARKERS if m in page), "?")
            if attempt < 2:
                print(f"    微信拦截({mnext})，退避 {delay:.0f}s")
                time.sleep(delay)
                delay *= 2
                continue
            return None, f"微信拦截: {mnext}"
        if body is None:
            return None, "无 js_content"
        return None, f"正文过短({cn_count(body)}汉字)" if cn_count(body) else "js_content 为空"
    return None, "unreachable"


# ---------------------------------------------------------------- state / manifest

def load_state():
    if STATE.exists():
        return json.loads(STATE.read_text(encoding="utf-8"))
    return {}


def save_state(state):
    STATE.write_text(json.dumps(state, ensure_ascii=False, indent=1),
                     encoding="utf-8")


def main():
    force = "--force" in sys.argv
    RAW.mkdir(parents=True, exist_ok=True)
    items = load_list()
    state = load_state()

    stats = {"入库": 0, "丢弃": 0, "坏链": 0, "站内待抓": 0, "跳过已有": 0}
    accepted, dropped, bad, pending = [], [], [], []
    counts = []

    for n, it in enumerate(items, 1):
        url, title = it["url"], it["title"]
        kind, real = classify(url)
        date = (it.get("pubDate") or "")[:10]
        domain = it.get("second_column", "")

        if kind == "bad":
            bad.append((title, url))
            state[url] = {"status": "bad"}
            continue
        if kind == "nhc_site":
            pending.append((title, url))
            state[url] = {"status": "pending"}
            continue

        rec = state.get(real) or state.get(url)
        if rec and rec.get("status") == "ok" and not force:
            fname = rec.get("file", "")
            if fname and (RAW / fname).exists():
                accepted.append((title, real, domain, date, rec.get("chars", 0), fname))
                stats["入库"] += 1
                counts.append(rec.get("chars", 0))
                stats["跳过已有"] += 1
                continue

        print(f"[{n}/{len(items)}] {title[:38]}")
        body, fail = fetch_weixin(real)
        if body is None:
            dropped.append((title, real, fail or "未知"))
            state[real] = {"status": "drop", "reason": fail or "未知"}
            stats["丢弃"] += 1
            continue
        cn = cn_count(body)
        if cn < MIN_CN_CHARS:
            dropped.append((title, real, f"正文仅{cn}汉字(<{MIN_CN_CHARS})"))
            state[real] = {"status": "drop", "reason": f"正文仅{cn}汉字"}
            stats["丢弃"] += 1
            continue

        fname = f"{slug_of(real)}_{stem_title(title)}.txt"
        path = RAW / fname
        header = (f"# 标题: {title}\n# URL: {real}\n# 领域: {domain}\n"
                  f"# 发布日期: {date}\n\n")
        path.write_text(header + body + "\n", encoding="utf-8")
        accepted.append((title, real, domain, date, cn, fname))
        state[real] = {"status": "ok", "file": fname, "chars": cn}
        if url != real:
            state[url] = {"status": "ok", "file": fname, "chars": cn}
        stats["入库"] += 1
        counts.append(cn)
        save_state(state)

    save_state(state)

    stats["丢弃"] = len(dropped)
    stats["坏链"] = len(bad)
    stats["站内待抓"] = len(pending)
    med = (sorted(counts)[len(counts) // 2] if counts else 0)
    lines = ["# 国家卫健委 健康科普辟谣平台 抓取清单",
             f"# 运行时间: {time.strftime('%Y-%m-%d %H:%M:%S')}",
             f"# 接口去重后条目: {len(items)} | 入库: {len(accepted)}"
             f" | 丢弃: {len(dropped)} | 坏链: {len(bad)} | 站内待抓: {len(pending)}",
             "", "## 入库", "# 标题|URL|领域|发布日期|正文字数"]
    for t, u, dm, d, c, _f in accepted:
        lines.append(f"{t}|{u}|{dm}|{d}|{c}")
    lines += ["", "## 丢弃", "# 标题|URL|原因"]
    for t, u, r in dropped:
        lines.append(f"{t}|{u}|{r}")
    lines += ["", "## 坏链", "# 标题|URL"]
    for t, u in bad:
        lines.append(f"{t}|{u}")
    lines += ["", "## 站内待抓", "# 标题|URL"]
    for t, u in pending:
        lines.append(f"{t}|{u}")
    MANIFEST.write_text("\n".join(lines) + "\n", encoding="utf-8")

    print(f"\n入库 {len(accepted)} / 丢弃 {len(dropped)} / 坏链 {len(bad)}"
          f" / 站内待抓 {len(pending)}；正文字数中位数 {med}")


if __name__ == "__main__":
    main()
