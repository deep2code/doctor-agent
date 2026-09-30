#!/usr/bin/env python3
"""中华医学会官网 (www.cma.org.cn) 健康科普正文抓取 → external/cma_health/raw/*.txt

枚举路径 (关键发现, 无需 playwright): 站点是 TRS 老模板, 栏目列表页内嵌
<datastore> 的 nextgroup 指向 /module/web/jpage/dataproxy.jsp?page=N&columnid=NNN
&unitid=325&... — 直接按 page 递增请求即可拿到该栏目**全部**文章链接
(响应含 <totalpage>)。静态 index_1.html 分页是 404, 不要用。

文章页静态: 正文在 <td class="bt_content"><div id="zoom"> ... 由
<meta name="ContentStart"/> / ContentEnd 注释包裹; 标题取
<meta name="ArticleTitle">, 日期取「发布日期：YYYY-MM-DD」, 来源取
<!--<$[信息来源]>begin-->...end 之间文本。正文中文数 < 400 的丢弃。
每抓成一篇立即写盘并追加 MANIFEST。

栏目: 4584 科普图文(主力,~1095条) / 68 健康常识 / 66 科普活动 / 12 科普与健康主页
(982 科普视频为视频海报页, 按设计排除)

用法: python3 external/fetch_cma_health.py            # 全量增量
      python3 external/fetch_cma_health.py --max-ok 110
"""
import html as htmlmod
import re
import sys
import time
import urllib.request
from pathlib import Path

BASE = Path(__file__).parent
OUT = BASE / "cma_health" / "raw"
UA = {"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
                    "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
      "Accept-Language": "zh-CN,zh;q=0.9", "Accept": "text/html"}
HOST = "https://www.cma.org.cn"
MIN_CJK = 400
SLEEP = 0.85

# (columnid, slug, name) — 按患者价值排序; dataproxy unitid 全站恒为 325
COLUMNS = [
    (4584, "kepu_tuwen",  "科普图文"),
    (68,   "jiankang",    "健康常识"),
    (66,   "kepu_huodong", "科普活动"),
]
# 每栏目最多抓多少篇 OK (总量再受 --max-ok 约束)
COL_CAP = {4584: 120, 68: 9, 66: 15}

ART_RE = re.compile(r'(/art/\d{4}-\d{1,2}-\d{1,2}/art_(\d+)_(\d+)\.html)|'
                    r'(/art/\d{4}/\d{1,2}/\d{1,2}/art_(\d+)_(\d+)\.html)')


def http_get(url: str, timeout: int = 15, retries: int = 1) -> str:
    for i in range(retries + 1):
        try:
            req = urllib.request.Request(url, headers=UA)
            with urllib.request.urlopen(req, timeout=timeout) as r:
                return r.read().decode("utf-8", "replace")
        except Exception:
            if i == retries:
                raise
            time.sleep(1.5)
    return ""


def cjk_count(s: str) -> int:
    return sum(1 for c in s if "\u4e00" <= c <= "\u9fff")


def clean_block(seg: str) -> str:
    """HTML 片段 → 纯文本: 块级/表格单元换行, 抹平行内标签。"""
    s = re.sub(r"<!--.*?-->", "", seg, flags=re.S)
    s = re.sub(r"<(script|style)[^>]*>.*?</\1>", "", s, flags=re.S)
    s = re.sub(r'<meta name="Content(?:Start|End)"/?>', "", s)
    s = re.sub(r"<br\s*/?>|</(p|div|h\d|li|tr|blockquote)>", "\x00", s)
    s = re.sub(r"</td>", "\x01", s)
    s = re.sub(r"<img[^>]*>", "", s)
    s = re.sub(r"<[^>]+>", "", s)
    s = htmlmod.unescape(s)
    lines = []
    for ln in s.split("\x00"):
        ln = re.sub(r"\s*\n\s*", "", ln).replace("\x01", " ")
        ln = re.sub(r"[ \t\u3000]+", " ", ln).strip()
        if ln:
            lines.append(ln)
    return "\n".join(lines)


def list_column(col_id: int, max_pages: int = 15):
    """经 dataproxy.jsp 翻页取栏目全部文章 (abs_url, title)。"""
    out, seen = [], set()
    url = ("%s/module/web/jpage/dataproxy.jsp?page=1&webid=1&path=/&columnid=%d"
           "&unitid=325&permissiontype=0" % (HOST, col_id))
    for page in range(1, max_pages + 1):
        u = url.replace("page=1", "page=%d" % page) if page > 1 else url
        try:
            d = http_get(u)
        except Exception as e:
            print("  [col%d] page%d 列表失败: %s" % (col_id, page, str(e)[:60]), file=sys.stderr)
            break
        tp = re.search(r"<totalpage>(\d+)</totalpage>", d)
        total_page = int(tp.group(1)) if tp else 1
        for rec in re.findall(r"<record><!\[CDATA\[(.*?)\]\]></record>", d, re.S):
            m = re.search(r'href="(/art/[^"]+\.html)"[^>]*>([^<]+)', rec)
            if not m:
                continue
            href = m.group(1)
            if href in seen:
                continue
            seen.add(href)
            out.append((HOST + href, htmlmod.unescape(m.group(2)).strip()))
        print("  [col%d] page %d/%d → 累计 %d 候选" % (col_id, page, total_page, len(out)),
              file=sys.stderr)
        if page >= total_page:
            break
        time.sleep(SLEEP)
    return out


def parse_article(url: str, anchor_title: str = ""):
    """返回 (title, date, source_org, body) 或 None (太短/无正文/视频海报页)。"""
    raw = http_get(url)
    # 正文容器: <td class="bt_content"> 内 div#zoom, 用 ContentStart/End 注释定界
    m = re.search(r'<div[^>]*id="zoom"[^>]*>(.*?)(?=<!--ZJEG_RSS\.content\.end-->|'
                  r'<meta name="ContentEnd"|<div class="bt_link|<td class="bt_link|$)', raw, re.S)
    if not m:
        m2 = re.search(r'<td class="bt_content">(.*?)(?=<td class="bt_link|<div class="clear|$)',
                       raw, re.S)
        if not m2:
            return None
        m = type("M", (), {"group": lambda self, n, s=m2: s.group(1)})()
    seg = m.group(1)
    body = clean_block(seg)
    # 去尾部残留 widget 文本
    for stop in ("我要评论", "分享给朋友", "【打印文章】", "打印", "版权页", "相关文章"):
        i = body.find(stop)
        if i > 300:
            body = body[:i].rstrip()
    n = cjk_count(body)
    if n < MIN_CJK:
        return None
    if "<img" in seg and n < MIN_CJK * 3:
        pass  # 图多文少也保留, 只要汉字够
    t = re.search(r'<meta name="ArticleTitle" content="([^"]*)"', raw)
    title = t.group(1).strip() if t else ""
    if not title:
        t2 = re.search(r"<title>([^<]*)</title>", raw)
        title = (t2.group(1) if t2 else anchor_title).strip()
        title = re.sub(r"^中华医学会\s*\S{2,8}\s*", "", title).strip() or anchor_title
    dm = re.search(r"发布日期[：:]\s*(\d{4}-\d{2}-\d{2})", raw)
    date = dm.group(1) if dm else ""
    if not date:
        um = re.search(r"/art/(\d{4})-(\d{1,2})-(\d{1,2})/", url)
        if um:
            date = "%s-%02d-%02d" % (int(um.group(1)), int(um.group(2)), int(um.group(3)))
    sm = re.search(r"信息来源\]>begin-->(.*?)<", raw, re.S)
    org = clean_block(sm.group(1)).split("\n")[0].strip() if sm else ""
    if not org:
        sm2 = re.search(r"来源[：:]\s*([^<\n]{2,30})", raw)
        org = (sm2.group(1).strip() if sm2 else "")
    org = org.strip("－-— 　")
    if "中华医学会" not in org:
        org = ("中华医学会" + ("·" + org if org and org != "中华医学会" else ""))
    return title or anchor_title, date, org or "中华医学会", body


def main() -> int:
    OUT.mkdir(parents=True, exist_ok=True)
    manifest = OUT / "MANIFEST.txt"
    if not manifest.exists():
        manifest.write_text("# cma.org.cn 科普抓取清单\n", encoding="utf-8")
    max_ok = 120
    if "--max-ok" in sys.argv:
        max_ok = int(sys.argv[sys.argv.index("--max-ok") + 1])

    done_ids = set()
    for ln in manifest.read_text(encoding="utf-8").splitlines():
        done_ids.update(re.findall(r"/art/\S+/art_\d+_(\d+)\.html", ln))

    ok = dropped = 0
    for col_id, slug, colname in COLUMNS:
        if ok >= max_ok:
            break
        print("[col%d %s] 枚举列表..." % (col_id, colname), file=sys.stderr)
        arts = list_column(col_id)
        # 每栏目上限按磁盘已有文件累计 (增量重跑时不会被单一栏目吃满)
        col_ok = len(list(OUT.glob("%s_*.txt" % slug)))
        for url, anchor in arts:
            aid = url.rsplit("_", 1)[-1].split(".")[0]
            if aid in done_ids or col_ok >= COL_CAP.get(col_id, 10 ** 9) or ok >= max_ok:
                continue
            try:
                parsed = parse_article(url, anchor)
            except Exception as e:
                with manifest.open("a", encoding="utf-8") as fh:
                    fh.write("# 不可达: %s %s\n" % (url, str(e)[:60]))
                dropped += 1
                done_ids.add(aid)
                time.sleep(SLEEP)
                continue
            if not parsed:
                with manifest.open("a", encoding="utf-8") as fh:
                    fh.write("# 已排除: %s(%s) 正文<%d汉字或为图片/视频页\n" % (url, anchor, MIN_CJK))
                dropped += 1
                done_ids.add(aid)
                time.sleep(SLEEP)
                continue
            title, date, org, body = parsed
            date_part = ("，" + date) if date else ""
            text = "# URL: %s\n# 标题: %s\n# 发布方/日期: %s%s\n\n%s\n" % (
                url, title, org, date_part, body)
            fname = "%s_%s.txt" % (slug, aid)
            (OUT / fname).write_text(text, encoding="utf-8")
            with manifest.open("a", encoding="utf-8") as fh:
                fh.write("%s | %s | %s | %s | %s\n" %
                         (fname, title, url, org, date or "-"))
            ok += 1
            col_ok += 1
            done_ids.add(aid)
            print("OK %s cjk=%d" % (fname, cjk_count(body)), file=sys.stderr)
            time.sleep(SLEEP)
        print("[col%d] 本轮 OK %d" % (col_id, col_ok), file=sys.stderr)
    print("完成: 新抓 %d, 排除/不可达 %d" % (ok, dropped), file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
