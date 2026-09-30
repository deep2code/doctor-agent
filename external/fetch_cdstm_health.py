#!/usr/bin/env python3
"""中国数字科技馆 (www.cdstm.cn, 中国科协) 健康科普正文抓取 → external/cdstm_health/raw/*.txt

站点为 TRS CMS, 文章 URL 形如 https://www.cdstm.cn/<section>/<sub>/art/<YYYY>/art_<32hex>.html
(静态正文, urllib 直连即可)。

枚举策略 (按实测收敛):
  1. sitemap.xml — 注意它实际是 Word 文档包 (robots.txt 声明), 正文是**转义的
     urlset XML**, 需 html.unescape('<w:t>' 拼接文本) 后再提 <loc>;
  2. 主题站 https://www.cdstm.cn/subjects/kxlyb/index.html (每月"科学"流言榜
     汇总页, 内含 2024-2026 全部流言榜 frontier/xw 链接);
  3. 频道静态索引 /knowledge/kpwk/<slug>/index.html (每频道只暴露最近 6 篇,
     无 index_N 分页 — 归档不可达, 已按缺口记录)。
  Wayback CDX 在本机网络不可达 (curl 000), Playwright 渲染 frontier 频道列表
  只回落到 资讯 最新条目 — 均作为缺口写入 MANIFEST 排除段。

健康过滤: ColumnName 元信息 (医药健康) 或 标题命中健康/辟谣正则保留;
命中泛科技/活动通知正则丢弃; 正文中文数 < 500 丢弃 (视频/海报页)。
每抓成一篇立即写盘并追加 MANIFEST。

用法: python3 external/fetch_cdstm_health.py            # 增量 (跳过 MANIFEST 已有 URL)
      python3 external/fetch_cdstm_health.py --rebuild  # 重跑枚举, 仍跳过已抓文件
"""
import html as htmlmod
import re
import sys
import time
import urllib.request
from pathlib import Path

BASE = Path(__file__).parent
OUT = BASE / "cdstm_health" / "raw"
UA = {"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
                    "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"}
ART_RE = re.compile(r"(?:https?://(?:www\.)?cdstm\.cn)?/([a-z]+/[a-z0-9_]+(?:/[a-z0-9_]+)*)/art/(\d{4})/art_([0-9a-f]{32})\.html")
MIN_CJK = 500

# 保留: 健康/医疗栏目 或 标题命中
KEEP_COL = re.compile(r"医药健康|健康")
KEEP_TITLE = re.compile(
    r"流言榜|健康|医|药|病|癌|疫苗|血压|血糖|血脂|肥胖|营养|膳食|食安|食品|母婴|孕|产妇"
    r"|儿童|小儿|疫苗|急救|心肺复苏|心理|抑郁|焦虑|睡眠|失眠|运动(损伤|康复)?|骨质疏松"
    r"|牙齿|口腔|近视|护眼|疫苗|慢病|体检|辐射|消毒|抗菌|口罩|流感|感冒|退烧")
# 丢弃: 泛科技/活动通知 (优先于 KEEP 判定, 防止"中医药大讲堂"式标题误伤由人工复核)
DROP_TITLE = re.compile(
    r"机器人|人工智能|AI\b|航|火箭|卫星|空间站的?|天文|黑洞|星座|考古|恐龙|化石|矿物"
    r"|展览|开幕|闭幕|倒计时|启幕|游园|研|大赛|竞赛|评选|颁奖|论坛|会议|讲座预告|直播预告"
    r"|VR|元宇宙|3D打印|芯片|量子|核武|武器|物种|灭绝|朱鹮|白鲟|长江生物|蝠|鼠年")


def get(url: str, timeout: int = 15) -> str:
    for i in range(2):
        try:
            req = urllib.request.Request(url, headers=UA)
            with urllib.request.urlopen(req, timeout=timeout) as r:
                data = r.read()
            try:
                import gzip, io
                if data[:2] == b"\x1f\x8b":
                    data = gzip.decompress(data)
            except Exception:
                pass
            return data.decode("utf-8", "replace")
        except Exception:
            if i == 0:
                time.sleep(1.5)
            else:
                raise
    return ""


def cjk(s: str) -> int:
    return sum(1 for c in s if "\u4e00" <= c <= "\u9fff")


def enumerate_urls() -> list:
    """返回去重后的候选文章 URL 列表 (art id 为键)。"""
    urls: dict[str, str] = {}

    def harvest(page: str):
        for m in ART_RE.finditer(page):
            urls.setdefault(m.group(3), "https://www.cdstm.cn" + m.group(0).split("cdstm.cn", 1)[-1])

    # 1. sitemap.xml (Word-doc wrapped escaped urlset)
    try:
        raw = get("https://www.cdstm.cn/sitemap.xml", timeout=30)
        txt = htmlmod.unescape(" ".join(re.findall(r"<w:t[^>]*>([^<]+)</w:t>", raw)))
        if "art_" not in txt:
            txt = htmlmod.unescape(raw)
        harvest(txt)
        print(f"[枚举] sitemap locs → {len(urls)} 候选", file=sys.stderr)
    except Exception as e:
        print(f"[枚举] sitemap 失败: {str(e)[:80]}", file=sys.stderr)
    # 2. 流言榜主题页
    for page_url in ("https://www.cdstm.cn/subjects/kxlyb/index.html",):
        try:
            time.sleep(0.9)
            harvest(get(page_url))
        except Exception as e:
            print(f"[枚举] {page_url} 失败: {str(e)[:60]}", file=sys.stderr)
    # 3. kpwk 静态频道索引 (仅健康相关 slug)
    for slug in ("yyjk", "smkx", "jtys", "hjkx", "aqkx"):
        try:
            time.sleep(0.9)
            harvest(get(f"https://www.cdstm.cn/knowledge/kpwk/{slug}/index.html"))
        except Exception as e:
            print(f"[枚举] kpwk/{slug} 失败: {str(e)[:60]}", file=sys.stderr)
    return list(urls.values())


# 只在这些频道里抓正文 (站点主体是泛科技, 其余频道必被标题过滤杀掉, 省流量)
PATH_ALLOW = re.compile(r"^https://www\.cdstm\.cn/(frontier/(yyjk|xw)/|subjects/kxlyb/|knowledge/kpwk/)")


def clean_block(seg: str) -> str:
    s = re.sub(r"<(script|style)[^>]*>.*?</\1>", "", seg, flags=re.S)
    s = re.sub(r"<br\s*/?>|</(p|div|h\d|li|tr|blockquote)>", "\x00", s)
    s = re.sub(r"<[^>]+>", "", s)
    s = htmlmod.unescape(s)
    lines = []
    for ln in s.split("\x00"):
        ln = re.sub(r"\s*\n\s*", "", ln).strip()
        ln = re.sub(r"[ \t\u3000]+", " ", ln)
        if ln:
            lines.append(ln)
    return "\n".join(lines)


def parse_article(url: str):
    """返回 (title, date, body) / None(太短或无正文) / 'offtopic'。"""
    raw = get(url)
    meta_t = re.search(r'<meta name="ArticleTitle" content="([^"]*)"', raw)
    meta_col = re.search(r'<meta name="ColumnName" content="([^"]*)"', raw)
    meta_pd = re.search(r'<meta name="PubDate" content="([^"]*)"', raw)
    title = htmlmod.unescape(meta_t.group(1)).strip() if meta_t else ""
    if not title:
        t = re.search(r"<title>([^<]*)</title>", raw)
        title = t.group(1).strip() if t else ""
    col = meta_col.group(1) if meta_col else ""
    date = (meta_pd.group(1)[:10] if meta_pd else "")
    if not date:
        m = re.search(r"/art/(\d{4})/", url)
        date = m.group(1) if m else ""

    # 标题/栏目过滤
    if DROP_TITLE.search(title) and not KEEP_TITLE.search(title):
        return "offtopic"
    if not (KEEP_COL.search(col) or KEEP_TITLE.search(title)):
        return "offtopic"

    m = re.search(r'<div[^>]*class="article-content"[^>]*>(.*?)(?=<div[^>]*class="[^"]*article-(?:nav|footer)|<div[^>]*class="[^"]*comment|<div[^>]*id="comment)', raw, re.S)
    if not m:
        m = re.search(r'<div[^>]*class="[^"]*TRS_Editor[^"]*"[^>]*>(.*?)(?=<div[^>]*class="[^"]*(?:article-nav|footer)|<script)', raw, re.S)
    if not m:
        return None
    body = clean_block(m.group(1))
    # 截掉页面挂回的元数据/版权尾巴
    cut = len(body)
    for stop in ("特别声明", "[责任编辑", "【责任编辑", "本文来自"):
        i = body.find(stop)
        if i > 200:
            cut = min(cut, i)
    body = body[:cut]
    body = re.sub(r"(?m)^(发布时间|来源[:：]|浏览|分享|收藏|扫码.*|上一篇.*|下一篇.*|相关文章.*)\n?", "", body)
    if cjk(body) < MIN_CJK:
        return None
    return title, date, body


def main() -> int:
    OUT.mkdir(parents=True, exist_ok=True)
    manifest = OUT / "MANIFEST.txt"
    rebuild = "--rebuild" in sys.argv
    done_ids = set()
    if manifest.exists():
        for ln in manifest.read_text(encoding="utf-8").splitlines():
            if ln.startswith("#"):  # 排除/不可达记录 — 重跑时先重试
                continue
            done_ids.update(re.findall(r"art_([0-9a-f]{32})", ln))
    cands = [u for u in enumerate_urls() if PATH_ALLOW.search(u)]
    todo = [u for u in cands if ART_RE.search(u).group(3) not in done_ids]
    print(f"候选 {len(cands)}, 待抓 {len(todo)}", file=sys.stderr)
    ok = skip_len = skip_of = err = 0
    for url in todo:
        aid = ART_RE.search(url).group(3)
        try:
            parsed = parse_article(url)
        except Exception as e:
            with manifest.open("a", encoding="utf-8") as fh:
                fh.write("# 不可达: %s %s\n" % (url, str(e)[:60]))
            err += 1
            time.sleep(0.9)
            continue
        if parsed == "offtopic":
            with manifest.open("a", encoding="utf-8") as fh:
                fh.write("# 已排除: %s 非健康主题(标题/栏目过滤)\n" % url)
            skip_of += 1
            time.sleep(0.9)
            continue
        if not parsed:
            with manifest.open("a", encoding="utf-8") as fh:
                fh.write("# 已排除: %s 正文<%d中文字(视频/海报页)或无正文容器\n" % (url, MIN_CJK))
            skip_len += 1
            time.sleep(0.9)
            continue
        title, date, body = parsed
        stem = url.rsplit("/", 1)[-1][:-5]  # art_<hex>
        if "流言榜" in title:
            dm = re.search(r"(\d{4})年(\d{1,2})月", title)
            fname = "cdstm_lyb_%s%s.txt" % (dm.group(1), dm.group(2).zfill(2)) if dm \
                else "cdstm_%s.txt" % stem[4:16]
        else:
            fname = "cdstm_%s.txt" % stem[4:16]
        path = OUT / fname
        n = 2
        while path.exists() and not rebuild:
            path = OUT / (fname[:-4] + "_%d.txt" % n)
            n += 1
        text = "# URL: %s\n# 标题: %s\n# 发布方/日期: 中国数字科技馆，%s\n\n%s\n" % (url, title, date, body)
        path.write_text(text, encoding="utf-8")  # 立即落盘
        with manifest.open("a", encoding="utf-8") as fh:
            fh.write("%s | %s | %s | 中国数字科技馆 | %s\n" % (path.name, title, url, date))
        ok += 1
        print("OK", path.name, cjk(body), file=sys.stderr)
        time.sleep(0.9)
    # 枚举缺口一次性记录
    if not manifest.exists() or "枚举缺口" not in manifest.read_text(encoding="utf-8", errors="replace"):
        with manifest.open("a", encoding="utf-8") as fh:
            fh.write("# 枚举缺口: kpwk 频道静态索引仅暴露最近6篇, index_N 分页 404;"
                     " Wayback CDX 本机不可达(curl 000); frontier 频道列表 JS 渲染只回落资讯最新条目\n")
    print(f"新抓 {ok}, 非健康 {skip_of}, 正文过短 {skip_len}, 不可达 {err}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
