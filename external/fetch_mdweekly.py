#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Harvest 医学科普/临床科普 article full text from 《医师报》官网 (www.mdweekly.com.cn).

医师报 is hosted by 《健康报》社 under 国家卫生健康委员会 (national health-industry
newspaper, authoritative; no WAF — plain GET works with a Chrome UA).

Enumeration path (verified 2026-09-30):
  GET /index/index/search?keyword=<kw>&page=<N>   (~5 items/page, carries 分类 + 发布日期)
  GET /index/index/search2?keyword=<kw>&page=<N>  (~10 items/page, same item markup)
Both paginate properly (unlike /index/index/kepu / newlist which ignore ?page).
Only items whose 分类 contains 科普 (科普 / 科普在线) are kept as candidates.

Output: external/mdweekly/raw/mdw_<id>.txt (3-line header + body, one paragraph per line)
        external/mdweekly/MANIFEST.txt
State (resume): external/mdweekly/state.json  {"candidates": {...}, "done": {...}}

Usage:
  python3 external/fetch_mdweekly.py            # enumerate (once, cached) + fetch all pending
  python3 external/fetch_mdweekly.py --enumerate-only
  python3 external/fetch_mdweekly.py --fetch-only
Politeness: single-threaded, 1.3s between requests, 15s timeout, one retry.
"""
import html
import json
import os
import re
import sys
import time
import urllib.parse
import urllib.request

BASE = "https://www.mdweekly.com.cn"
UA = ("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 "
      "(KHTML, like Gecko) Chrome/120.0 Safari/537.36")
DELAY = 1.3
OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "mdweekly")
RAW = os.path.join(OUT, "raw")
STATE = os.path.join(OUT, "state.json")
MANIFEST = os.path.join(OUT, "MANIFEST.txt")

KEYWORDS = [
    "高血压", "糖尿病", "用药", "儿童", "癌症", "肿瘤", "疼痛", "呼吸", "心脏", "卒中",
    "过敏", "疫苗", "胃肠", "肾", "肥胖", "睡眠", "抑郁", "焦虑", "哮喘", "体检",
    "幽门螺杆菌", "甲状腺", "脂肪肝", "痛风", "营养", "急救", "筛查", "血脂", "尿酸",
    "口腔", "眼科", "皮肤", "孕期", "老年", "咳嗽", "发烧",
]

# non-popular-science editorial content patterns (医院管理/人物/会议/公文…)
DROP_TITLE = re.compile(
    r"访谈|对话|专访|风采|致敬|庆祝|表彰|开幕|闭幕|预告|招募|招标|中标|公示|通告|"
    r"培训|研讨|巡讲|巡回|签约|捐赠|院长|书记|医师节|中国医师|图片直播|视频直播|直播|"
    r"大会|论坛|峰会|展览|比赛|竞赛|颁奖|摄影|书画|诗歌|散文|随笔|寄语|贺信|走访|调研|"
    r"揭牌|成立|年会|白皮书发布|榜单|评选|回忆|纪念|纪事|人物|追光|践行者|侧记|手记")

SEARCH_ITEM = re.compile(
    r'<li>\s*<h1><a href="/index/article/detail\?id=(\d+)">(.*?)</a></h1>(.*?)</li>', re.S)
CAT_RE = re.compile(r"分类：<strong>(.*?)</strong>")
DATE_RE = re.compile(r"发布日期：(\d{4}-\d{2}-\d{2})")

BODY_DROP = re.compile(r"转载请注明出处|版权归|扫码关注|关注我们| ad|广告|医师报官微|更多新闻请")


def fetch(url, tries=2):
    for t in range(tries):
        try:
            req = urllib.request.Request(url, headers={"User-Agent": UA})
            with urllib.request.urlopen(req, timeout=15) as r:
                data = r.read()
            return data.decode("utf-8", "replace")
        except Exception:
            if t + 1 < tries:
                time.sleep(2.0)
    return None


def load_state():
    if os.path.exists(STATE):
        with open(STATE, encoding="utf-8") as f:
            return json.load(f)
    return {"candidates": {}, "enumerated": []}


def save_state(st):
    tmp = STATE + ".tmp"
    with open(tmp, "w", encoding="utf-8") as f:
        json.dump(st, f, ensure_ascii=False, indent=1)
    os.replace(tmp, STATE)


def enumerate_all(st):
    """Fill st['candidates'] {id: {title, cat, date}} via paginated site search."""
    done_marks = set(st.get("enumerated", []))
    for kw in KEYWORDS:
        for ep, maxp in (("search", 30),):  # search2 omitted: server-side 20~80+s/request, unreliable
            mark = f"{ep}:{kw}"
            if mark in done_marks:
                continue
            prev_ids = None
            added = 0
            for p in range(1, maxp + 1):
                url = f"{BASE}/index/index/{ep}?keyword={urllib.parse.quote(kw)}&page={p}"
                h = fetch(url)
                time.sleep(DELAY)
                if not h:
                    break
                blocks = SEARCH_ITEM.findall(h)
                ids = {m[0] for m in blocks}
                if not ids:
                    break
                for aid, title, meta in blocks:
                    cm = CAT_RE.search(meta)
                    cat = html.unescape(cm.group(1)).strip() if cm else ""
                    if "科普" not in cat:
                        continue
                    title = html.unescape(re.sub(r"<[^>]+>", "", title)).strip()
                    if DROP_TITLE.search(title) or DROP_TITLE.search(cat):
                        continue
                    dm = DATE_RE.search(meta)
                    cand = st["candidates"].setdefault(
                        aid, {"title": title, "cat": cat, "date": dm.group(1) if dm else ""})
                    if not cand.get("date") and dm:
                        cand["date"] = dm.group(1)
                    added += 1
                if ids == prev_ids:
                    break
                prev_ids = ids
            done_marks.add(mark)
            st["enumerated"] = sorted(done_marks)
            save_state(st)
            print(f"[enum] {mark}: +{added} hits, candidates total {len(st['candidates'])}",
                  flush=True)


def strip_tags(s):
    s = re.sub(r"<[^>]+>", "", s)
    s = html.unescape(s).replace("\u00a0", " ").replace("\u3000", " ")
    return re.sub(r"[ \t]+", " ", s).strip()


def parse_article(h):
    """Return (title, date, category, [paragraphs]) from an article detail page.

    Two layouts: /index/article/detail uses <h1 class="glob-h1"> + class="detail-box";
    ids that 302 to /index/paper/detail use a bare <h1> + class="bm-detail-box".
    The body container is what matters — title falls back to the search-list caption.
    """
    dm = re.search(r"时间：(\d{4}-\d{2}-\d{2})", h)
    date = dm.group(1) if dm else ""
    cm = re.search(r'<div class="lanmu">.*?class="text">([^<]+)<', h, re.S)
    cat = cm.group(1).strip() if cm else ""
    t = re.search(r'<h1 class="glob-h1">(.*?)</h1>', h, re.S)
    title = strip_tags(t.group(1)) if t else ""
    i = h.find('class="detail-box"')
    if i < 0:
        i = h.find('class="bm-detail-box"')
    j = h.find("detail-editor", i)
    if i < 0:
        return None
    seg = h[h.find(">", i) + 1: j if j > i else len(h)]
    seg = re.sub(r"<!--.*?-->", "", seg, flags=re.S)  # drop commented dupes
    seg = re.sub(r"<(?:script|style)\b.*?</(?:script|style)>", "", seg, flags=re.S)

    from html.parser import HTMLParser

    class ParaParser(HTMLParser):
        BLOCK_END = {"p", "div", "li", "section", "h1", "h2", "h3", "h4",
                     "h5", "h6", "tr", "article", "blockquote"}

        def __init__(self):
            super().__init__(convert_charrefs=True)
            self.chunks = []
            self.buf = []

        def flush(self):
            t = "".join(self.buf).replace("\u00a0", " ").replace("\u3000", " ")
            t = re.sub(r"\s+", " ", t).strip()
            if t:
                self.chunks.append(t)
            self.buf = []

        def handle_starttag(self, tag, attrs):
            if tag == "br":
                self.flush()

        def handle_endtag(self, tag):
            if tag in self.BLOCK_END:
                self.flush()

        def handle_data(self, data):
            self.buf.append(data)

    pp = ParaParser()
    pp.feed("<div>" + seg)  # neutral wrapper so stray closing tags are harmless
    pp.flush()
    paras = []
    for t in pp.chunks:
        if len(t) < 2:
            continue
        if BODY_DROP.search(t):
            continue
        # drop reporter/editor byline duplications & widget lines
        if re.fullmatch(r"(责任编辑|编辑|校对|作者)[：:].{0,30}", t):
            continue
        if re.fullmatch(r"(医师报|《医师报》|健康报).{0,12}", t):
            continue
        t = re.sub(r"^医师报讯[：:]?", "", t)
        t = re.sub(r"（?融媒体记者[：:]?[^）\n]{1,25}）?[，,]?", "", t, count=1)
        t = t.strip()
        if t:
            paras.append(t)
    # de-dup consecutive identical paragraphs
    out = []
    for t in paras:
        if out and out[-1] == t:
            continue
        out.append(t)
    return title, date, cat, out


def parse_paper(h):
    """Extract (title, date, category, paras) from /index/paper/detail page (print edition)."""
    from html.parser import HTMLParser
    bi = h.find("bm-detail-info")
    if bi < 0:
        return None
    m = None
    for mm in re.finditer(r'<h1[^>]*>(.*?)</h1>', h[:bi], re.S):
        m = mm
    if not m:
        return None
    title = strip_tags(m.group(1))
    # category: text between the title h1 and 标题导航 (print-edition header)
    cm = re.search(r"</h1>\s*(?:<!--.*?-->\s*)*([^<>{}\s][^<>{}]{1,28})\s*<",
                   h[m.end():m.end() + 600], re.S)
    cat = re.sub(r"<[^>]+>", "", cm.group(1)).strip() if cm else ""
    dm = re.search(r"发布时间：(\d{4}-\d{2}-\d{2})", h)
    date = dm.group(1) if dm else ""

    i = h.find('id="content"')
    if i < 0:
        return None
    seg = h[h.find(">", i) + 1:]
    k = seg.find('<div class="modal')
    seg = seg[:k] if k > 0 else seg
    seg = re.sub(r"<!--.*?-->", "", seg, flags=re.S)
    seg = re.sub(r"<(?:script|style)\b.*?</(?:script|style)>", "", seg, flags=re.S)

    class ParaParser(HTMLParser):
        BLOCK_END = {"p", "div", "li", "br", "h1", "h2", "h3", "h4", "tr"}

        def __init__(self):
            super().__init__(convert_charrefs=True)
            self.chunks, self.buf = [], []

        def flush(self):
            t = "".join(self.buf).replace("\u00a0", " ").replace("\u3000", " ")
            t = re.sub(r"\s+", " ", t).strip()
            if t:
                self.chunks.append(t)
            self.buf = []

        def handle_starttag(self, tag, attrs):
            if tag == "br":
                self.flush()

        def handle_endtag(self, tag):
            if tag in self.BLOCK_END:
                self.flush()

        def handle_data(self, data):
            self.buf.append(data)

    pp = ParaParser()
    pp.feed("<div>" + seg)
    pp.flush()
    SKIP = re.compile(r"^(发布时间|来源：|字体尺寸|标题导航|总第\d+期|【发表证书】|ICP|京ICP|粤ICP|吉ICP|京公网安备)"
                      r"|备案编号|医师报 - 医师网|欢迎订阅医师报|Copyright|版权所有|微信直播"
                      r"|^(上一篇|下一篇|直播|微信)|^.{0,6}微信$|加入我们|联系我们|关于我们|网站声明|广告及服务"
                      r"|^\.(pagination|bi|zoom)|^\}")
    raw = [t for t in pp.chunks if t and not BODY_DROP.search(t)
           and not SKIP.search(t) and t != title and not re.fullmatch(r"[A-Z]\d+(-\d+)?:.+", t)
           and "科普" != t]
    # print layout wraps mid-sentence across <p>: merge unless prev ends sentence-final
    merged = []
    SENT_END = "。！？：；…”』】》.!?;"
    for t in raw:
        if merged and merged[-1] and merged[-1][-1] not in SENT_END and len(t) < 400 \
                and not re.match(r"^(一|二|三|四|五|六|七|八|九|十|[0-9]+)[、.．：:]|^[（(]\d|^第|^[A-Za-z]?\d+[\.、]", t):
            merged[-1] += t
        else:
            merged.append(t)
    paras = []
    for t in merged:
        if re.fullmatch(r"(责任编辑|编辑|校对|作者)[：:].{0,40}", t):
            continue
        t = re.sub(r"^医师报讯[：:]?", "", t)
        t = t.strip()
        if t:
            paras.append(t)
    return title, date, cat, paras


def cjk_count(s):
    return sum(1 for ch in s if "\u4e00" <= ch <= "\u9fff")


def write_manifest(st, done):
    lines = ["医师报（www.mdweekly.com.cn）医学科普原文采集清单",
             "格式: 文件名 | 标题 | 直链 | 来源 | 日期", ""]
    for aid in sorted(done, key=int):
        d = done[aid]
        lines.append(f"raw/{d['file']} | {d['title']} | "
                     f"{BASE}/index/article/detail?id={aid} | 医师报 | {d['date'] or '-'}")
    lines += ["", "== 枚举方式说明 ==",
              "有效: GET /index/index/search?keyword=<词>&page=<N>（约5条/页），真实分页，"
              "条目带 分类 与 发布日期；仅收录 分类 含「科普」(科普/科普在线) 的条目。"
              "正文两种形态: 新条目在 article/detail 的 detail-box；旧期刊条目 article/detail 会 302 到 "
              "paper/detail?id=N，正文在 div#content (bm-detail-editor)，由 parse_paper 抽取。",
              "未用: /index/index/search2 名义上也分页，但服务端单请求 20~80+ 秒、频繁超时，已放弃。",
              "无效: /index/index/kepu、/index/index/newlist?id=N 的 ?page= 参数被忽略（每页恒为同一批）；"
              "无 sitemap.xml；robots.txt 允许全部。未按 id 空间暴力扫描（32k+ 请求会触发限封）。",
              "过滤: 标题/分类含 访谈|人物|会议|表彰|直播|招标 等编辑类关键词的条目丢弃；"
              "正文汉字数 <400 的丢弃。"]
    with open(MANIFEST, "w", encoding="utf-8") as f:
        f.write("\n".join(lines) + "\n")


def fetch_all(st):
    done = {}
    if os.path.exists(STATE):
        done = st.get("done", {})
    cands = st["candidates"]
    todo = [aid for aid in cands if aid not in done]
    print(f"[fetch] {len(todo)} pending of {len(cands)} candidates", flush=True)
    st.setdefault("done", done)
    kept = dropped = 0
    for n, aid in enumerate(todo, 1):
        url = f"{BASE}/index/article/detail?id={aid}"
        h = fetch(url)
        time.sleep(DELAY)
        cand = cands[aid]
        if not h:
            done[aid] = {"file": "", "title": cand["title"], "date": cand["date"],
                         "status": "fetch_fail"}
            save_state(st)
            continue
        if 'id="content"' in h and 'class="detail-box"' not in h:
            parsed = parse_paper(h)
        else:
            parsed = parse_article(h)
        if not parsed:
            done[aid] = {"file": "", "title": cand["title"], "date": cand["date"],
                         "status": "no_body"}
            save_state(st)
            continue
        title, date, cat, paras = parsed
        body = "\n".join(paras)
        if "标题导航" in body and "总第" in body[:12]:
            done[aid] = {"file": "", "title": title, "date": date, "status": "nav_only"}
            save_state(st)
            continue
        disp_title = title or cand["title"]
        if DROP_TITLE.search(disp_title) or DROP_TITLE.search(cat):
            done[aid] = {"file": "", "title": title, "date": date, "status": "drop_topic"}
            save_state(st)
            continue
        if cjk_count(body) < 400:
            done[aid] = {"file": "", "title": title, "date": date, "status": "too_short"}
            dropped += 1
            save_state(st)
            continue
        fname = f"mdw_{aid}.txt"
        head = [f"# URL: {url}", f"# 标题: {title or cand['title']}", ""]
        src_line = f"# 发布方/日期: 医师报（健康报社），{date or cand['date']}" if (date or cand["date"]) \
            else "# 发布方/日期: 医师报（健康报社）"
        head.insert(2, src_line)
        with open(os.path.join(RAW, fname), "w", encoding="utf-8") as f:
            f.write("\n".join(head) + "\n" + body + "\n")
        done[aid] = {"file": fname, "title": title or cand["title"],
                     "date": date or cand["date"], "status": "ok"}
        kept += 1
        save_state(st)
        write_manifest(st, {k: v for k, v in done.items() if v["status"] == "ok"})
        if n % 25 == 0:
            print(f"[fetch] {n}/{len(todo)} kept={kept} dropped={dropped}", flush=True)
    print(f"[fetch] done. kept={kept} dropped={dropped} fail={len(todo)-kept-dropped}",
          flush=True)


def main():
    os.makedirs(RAW, exist_ok=True)
    st = load_state()
    args = sys.argv[1:]
    if "--fetch-only" not in args:
        enumerate_all(st)
    if "--enumerate-only" not in args:
        fetch_all(st)
    write_manifest(st, {k: v for k, v in st.get("done", {}).items()
                        if v.get("status") == "ok"})


if __name__ == "__main__":
    main()
