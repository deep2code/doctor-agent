#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
fetch_commercial_pop.py — 商业/新媒体健康平台「原创署名科普」抓取器（对照层）

用途：只作为口语关键词与选题覆盖的**对照层**语料，**不是**权威数字来源。
任何数值都必须回到官方指南 raw 文本逐条核对。

抓取对象（2026-09-26 实测只有这三家能拿到静态正文 + 真实署名）：
  1. guokr.com          果壳网（科学人/健康频道）
  2. jiankang.163.com   网易健康（医师署名供稿）
  3. lifetimes.cn       生命时报（健康类报纸，本报记者署名 + 受访专家）

已放弃（详见 raw/MANIFEST.txt 的 已排除/不可达 段）：
  dxy.com（robots.txt 明确禁止大语言模型及其爬虫访问使用）、
  thepaper.cn（连续 403 WAF拦截页面）、
  cdstm.cn（文章页只有 ~280 汉字，正文客户端渲染）、
  familydoctor.com.cn（SEO 医院导流页，非原创署名）。

设计约束：
  * 幂等：已存在的 txt 直接跳过，可反复重跑续抓。
  * 逐条落盘：每抓到一篇立刻写文件并 flush，中断也保留已抓部分。
  * 礼貌：同域串行、请求间隔 >=1.0s、真实浏览器 UA；
    同一 host 连续 3 次失败或出现 429/403/5xx 即熔断该 host。
  * 正文一律原样摘录，不改写、不摘要、不翻译。

用法：
    python3 external/fetch_commercial_pop.py            # 抓取（默认每站上限 14 篇）
    python3 external/fetch_commercial_pop.py --limit 20 # 调整每站上限
    python3 external/fetch_commercial_pop.py --dry-run   # 只发现候选，不下载正文
"""

import html
import json
import re
import sys
import time
import urllib.request

REPO = __import__("pathlib").Path(__file__).resolve().parent.parent
OUT_DIR = REPO / "external" / "commercial_pop_health" / "raw"

UA = ("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 "
      "(KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
DELAY = 1.2          # 同一 host 请求间隔（秒）
MIN_CJK = 500        # 正文最少汉字数
PER_HOST_LIMIT = 14  # 每站目标篇数上限
MAX_ATTEMPTS_PER_ARTICLE = 2

LEVEL_LINE = ("# 来源级别: 商业平台原创科普（对照层，非权威数字来源；"
              "数字须回官方指南核对）")

# ---------------------------------------------------------------- HTTP

_last_hit = {}        # host -> timestamp
_fail_streak = {}     # host -> consecutive failures
_dead = {}            # host -> reason (熔断后不再访问)


def _sleep_politeness(host):
    gap = time.time() - _last_hit.get(host, 0.0)
    if gap < DELAY:
        time.sleep(DELAY - gap)


def fetch(url, *, host=None, allow_status=()):
    """Return (status, text) or (None, reason)."""
    host = host or re.sub(r"^.*?://", "", url).split("/")[0]
    if _dead.get(host):
        return None, f"circuit-open({_dead[host]})"
    _sleep_politeness(host)
    req = urllib.request.Request(url, headers={
        "User-Agent": UA,
        "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
        "Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
        "Connection": "close",
    })
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            _last_hit[host] = time.time()
            raw = r.read()
            status = r.status
    except urllib.error.HTTPError as e:
        _last_hit[host] = time.time()
        status, raw = e.code, b""
    except Exception as e:  # noqa: BLE001
        _fail_streak[host] = _fail_streak.get(host, 0) + 1
        if _fail_streak[host] >= 3:
            _dead[host] = f"3 consecutive errors ({type(e).__name__})"
        return None, f"error {type(e).__name__}"

    if status in allow_status:
        return status, ""
    if status != 200:
        _fail_streak[host] = _fail_streak.get(host, 0) + 1
        if status in (403, 429) or status >= 500:
            _dead[host] = f"HTTP {status}"
        elif _fail_streak[host] >= 3:
            _dead[host] = "3 consecutive failures"
        return None, f"HTTP {status}"

    _fail_streak[host] = 0
    # 中文站一律按 utf-8 解，否则出乱码（早期抓取器踩过这个坑）
    return 200, raw.decode("utf-8", "replace")


# ---------------------------------------------------------------- utils

def strip_tags(s):
    s = re.sub(r"(?is)<(script|style)[^>]*>.*?</\1>", " ", s)
    s = re.sub(r"(?i)<br\s*/?>|</p>|</div>|</li>|</h[1-6]>", "\n", s)
    s = re.sub(r"<[^>]+>", " ", s)
    s = html.unescape(s)
    s = s.replace("\u00a0", " ").replace("\u3000", " ")
    s = re.sub(r"[ \t]+", " ", s)
    s = re.sub(r"\n{3,}", "\n\n", s)
    return s.strip()


def cjk_count(s):
    return len(re.findall(r"[\u4e00-\u9fff]", s))


def slugify(text, fallback):
    keep = re.sub(r"[^\u4e00-\u9fffA-Za-z0-9]+", "-", text).strip("-")
    return (keep[:28] or fallback)


def write_article(fname, url, title, author, publisher, date, body, notes=None):
    """Verbatim body + provenance header. Flush per file."""
    OUT_DIR.mkdir(parents=True, exist_ok=True)
    path = OUT_DIR / fname
    if path.exists() and path.stat().st_size > 0:
        return None  # idempotent
    lines = [
        LEVEL_LINE,
        f"# URL: {url}",
        f"# 标题: {title}",
        f"# 作者/审核: {author or '未标注'}",
        f"# 发布方/日期: {publisher} / {date or '未标注'}",
    ]
    for k, v in (notes or {}).items():
        lines.append(f"# {k}: {v}")
    lines += ["", body, ""]
    with open(path, "w", encoding="utf-8") as fh:
        fh.write("\n".join(lines))
        fh.flush()
    return path.name


# --------------------------------------------------------------- 署名行
# 三个站的作者/审核信息都埋在正文里，格式各异。单独抽成函数，
# 这样 --reheader（离线修表头）和抓取走的是同一套规则。


def _author_guokr(body):
    """果壳署名：文：某某 / 作者：某某 / 编译：某某。
    名字后常紧跟「封面图来源：」，必须同段截断。返回 (署名行, 审核人)。"""
    author = ""
    ma = re.search(r"(?:^|\n)\s*(文|作者|编译|撰文|图文|文字|策划)"
                   r"\s*[：|｜]\s*([^\n]{2,60}?)(?=\n|$)", body)
    if ma:
        nm = re.split(r"[。；;]", ma.group(2))[0]
        nm = re.sub(r"(封面图|图片来源|题图|插画|设计|视觉|编辑|新媒体).*$", "", nm)
        nm = nm.strip(" 、，,｜|")
        if 2 <= len(nm) <= 40:
            author = f"{ma.group(1)}：{nm}"
    if not author:
        ma = re.search(r"(文|作者|编译|撰文)\s*[：|｜]\s*"
                       r"([\u4e00-\u9fffA-Za-z][\u4e00-\u9fffA-Za-z、·\s]{1,28})", body)
        if ma:
            nm = re.sub(r"(封面图|图片来源|题图|设计|视觉).*$", "", ma.group(2)).strip(" 、，")
            if 2 <= len(nm) <= 40:
                author = f"{ma.group(1)}：{nm}"
    mr = re.search(r"(?:科学性|医学|专业)?审核\s*[：|｜]\s*"
                   r"([\u4e00-\u9fffA-Za-z][\u4e00-\u9fffA-Za-z（）()、\s]{1,26})", body)
    reviewer = re.sub(r"\s+", " ", mr.group(1)).strip()[:26] if mr else ""
    return author, reviewer


def _author_netease(body):
    """网易健康署名：正文首段「作者：某某医院某某科医师 XXX」+「审核：某某」。"""
    ma = re.search(r"作者\s*[：:]\s*([^\n]{4,80})", body)
    author = "作者：" + re.sub(r"\s+", " ", ma.group(1)).strip()[:80] if ma else ""
    mrev = re.search(r"(?:审核|指导专家?|受访专家)\s*[：:]\s*([^\n]{3,60})", body)
    reviewer = re.sub(r"\s+", " ", mrev.group(1)).strip()[:60] if mrev else ""
    return author, reviewer


def _author_lifetimes(body, meta_authors=()):
    """生命时报：报纸署名在正文（本报记者 X / 通讯员 Y），专家栏为「受访专家：…」。"""
    inline = re.search(r"(?:^|[\n\s])(本报记者|健康时报(?:记者|特约记者)?|通讯员)"
                       r"\s*([\u4e00-\u9fff][\u4e00-\u9fff 　]{1,3}[\u4e00-\u9fff])"
                       r"(?=[\s，。、；：0-9日年]|$)", body)
    byline = ""
    if inline:
        byline = (f"{inline.group(1)} "
                  f"{re.sub('[ 　]+', ' ', inline.group(2)).strip()}")
    expert = re.search(r"受访专家\s*[：:]\s*([^\n]{4,80})", body)
    experts = ""
    if expert:
        experts = re.split(r"[。]", re.sub(r"\s+", " ", expert.group(1)))[0].strip()[:70]
    parts = []
    if meta_authors:
        parts.append("作者：" + "、".join(meta_authors))
    elif byline:
        parts.append(byline)
    if experts:
        parts.append("受访专家：" + experts)
    return "；".join(parts)


def _full_author(hostkey, body):
    """'# 作者/审核:' 这一行的最终文案（抓取和离线修表头共用）。"""
    if hostkey == "guokr":
        a, r = _author_guokr(body)
        return "；".join(p for p in (a, f"审核：{r}" if r else "") if p)
    if hostkey == "163jiankang":
        a, r = _author_netease(body)
        return "；".join(p for p in (a, f"审核/受访：{r}" if r else "") if p)
    return _author_lifetimes(body)


# ---------------------------------------------------------------- guokr

GUOKR_LISTS = [
    "https://www.guokr.com/science/category/health",
    "https://www.guokr.com/science/category/life",
    "https://www.guokr.com/science/category/food",
    "https://www.guokr.com/science/category/psy",
    "https://www.guokr.com/",
    "https://www.guokr.com/list",
]
GUOKR_RE = re.compile(r"/article/(\d{6})")

# 果壳/生命时报都是泛生活站，只有「身体·疾病·就医·吃喝·运动·孕幼·老人」这类题材
# 才是本轮要的口语问法语料。先按标题判定，标题不够再退到正文首段。
GUOKR_TOPICS = (
    "健康", "就医", "医院", "门诊", "急诊", "挂号", "复查", "筛查", "体检", "化验", "指标",
    "医", "病", "症", "候", "痛", "疼", "发烧", "发热", "咳嗽", "哮喘", "过敏", "拉肚子",
    "腹泻", "便秘", "胃", "肠", "肝", "肾", "肺", "心", "脑", "血", "糖", "压", "脂",
    "癌", "瘤", "结节", "息肉", "结石", "炎", "感染", "病毒", "细菌", "疫苗", "传染",
    "药", "处方", "用药", "副作用", "耐药", "抗生素", "止痛", "激素", "医保",
    "怀孕", "备孕", "孕期", "产", "哺乳", "月经", "妇科", "产科", "儿科", "新生",
    " child".strip(), "儿童", "孩子", "宝宝", "婴儿", "幼儿", "学龄", "青少年", "发育",
    "身高", "体重", "肥胖", "减肥", "长胖", "营养", "膳食", "饮食", "吃", "喝", "奶",
    "维生素", "钙", "铁", "蛋白", "益生", "乳糖", "酒精", "酒", "烟", "戒烟", "咖啡",
    "睡眠", "失眠", "熬夜", "打呼", "心理", "情绪", "抑郁", "焦虑", "压力", "认知",
    "痴呆", "记忆", "运动", "锻炼", "健身", "跑步", "拉伸", "久坐", "肌肉", "骨骼",
    "关节", "腰", "颈", "椎", "腿", "脚", "牙", "口腔", "眼", "视力", "近视", "耳",
    "鼻", "咽", "喉", "皮肤", "湿疹", "痘", "脱发", "癣", "疹", "痒", "烫伤", "烧伤",
    "急救", "复苏", "中暑", "溺水", "中毒", "防护", "消毒", "口罩", "卫生", "公卫",
    "老人", "养老", "失能", "照护", "护理", "更年期", "血压计", "血糖仪", "健康素养",
)

# 明确的非健康题材（消费维权/文娱/数码/出行等），命中标题即弃
OFF_TOPIC_TITLE = (
    "文具", "研学", "防诈", "诈骗", "颜值", "暖气", "马桶", "衣服", "穿搭", "手机",
    "冰箱", "芯片", "汽车", "房价", "门票", "维权", "投诉", "快递", "旅游", "景点",
    "综艺", "明星", "剧集", "游戏", "股价", "基金", "保险理赔", "营销", "直播带货",
    "咖啡品牌", "奶茶新品", "美妆品牌", "环保", "宠物食品", "猫粮", "狗粮",
    "主子", "猫主子", "狗子", "毛孩子",
)


def _healthish(title, body):
    """True only when the piece is genuinely about body/health/care-seeking."""
    t = re.sub(r"\s", "", title or "")
    if any(k in t for k in OFF_TOPIC_TITLE):
        return False
    if any(k in t for k in GUOKR_TOPICS):
        return True
    first = re.sub(r"\s", "", body[:400])
    return sum(1 for k in GUOKR_TOPICS if k in first) >= 4


def guokr_candidates():
    """Article ids from static links + the channel pages' embedded JSON."""
    out, seen = [], []
    for lp in GUOKR_LISTS:
        st, page = fetch(lp)
        if st != 200:
            continue
        for aid in GUOKR_RE.findall(page):
            if aid not in seen:
                seen.append(aid)
                out.append(aid)
        for aid in re.findall(r'"date_published":"[^"]+","id":(\d{6})', page):
            if aid not in seen:
                seen.append(aid)
                out.append(aid)
    return out


def guokr_extract(aid):
    url = f"https://www.guokr.com/article/{aid}"
    st, page = fetch(url)
    if st != 200:
        return None, st or "fail"
    m = re.search(r"<title[^>]*>([^<]*?)\| 果壳", page)
    title = html.unescape(m.group(1)).strip() if m else f"guokr-{aid}"

    # 正文容器用 styled-components 类名，但它同时出现在 <style> 的
    # sc-component-id 注释里——必须只匹配真正的 <div class=...> 开标签，
    # 否则会把整页（含导航）当成正文。
    hits = list(re.finditer(r'<div class="[^"]*styled__ArticleContent[^"]*"[^>]*>', page))
    body = ""
    for m in hits:
        end = page.find('<div class="styled__BlockWrap', m.end())
        if end < 0:
            end = page.find('id="adArticleBottom"', m.end())
        if end < 0:
            continue
        chunk = page[m.end():end]
        if cjk_count(chunk) > cjk_count(body):
            body = chunk
    body = strip_tags(body)
    # 少数转载稿正文前面粘了公众号名片（账号名/微信号/功能介绍），属站务样板不是正文
    body = re.sub(r"^\s*(?:.{0,30}\n)?\s*原创\s*\n[\s\S]{0,160}?"
                  r"(?:微信号\s*\n\s*Guokr42[\s\S]{0,60}?功能介绍[\s\S]{0,80}?)?\n\s*\n",
                  "", body)
    if "功能介绍" in body[:600] and "微信号" in body[:600]:
        cut = body.find("本文来自")
        m2 = re.search(r"(科学和技术，是我们和这个世界对话所用的语言。|[^。\n]{0,60}科普[^。\n]{0,20}。)",
                       body[:900])
        if m2:
            body = body[m2.end():].strip()
        elif cut > 0:
            body = body[:cut].strip()
    # 正文里混入的站务尾巴
    body = re.sub(r"(?m)^\s*(?:点个[“\"']?小爱心[”\"']?吧|扫码打开|手机客户端|打开微信|"
                  r"本文来自果壳，未经授权不得转载\.?|如有需要请联系sns@guokr\.com|"
                  r"我的同事竟是人工智障！|一个AI)\s*$", "", body).strip()
    if cjk_count(body) < MIN_CJK:
        return None, "short-body"

    date = ""
    md = re.search(r"发布于(?:<!-- -->)?\s*([0-9]{4}-[0-9]{2}-[0-9]{2})", page)
    if md:
        date = md.group(1)

    # 果壳的署名在正文里，见 _author_guokr
    author, reviewer = _author_guokr(body)

    if not author and not reviewer:
        return None, "no-signed-author"
    # 号外/每日新闻速览类聚合改写，正文里没有原创署名结构
    if re.search(r"每日.{0,6}(新闻|速览)|新闻快递|科学照妖镜|以下内容为聚合", title + body[:400]):
        return None, "aggregator-rewrite"
    # 果壳「科学人」有相当比例是公众号转载稿（她刊/南风窗…）：署名是自媒体写手、
    # 题材多为泛生活八卦，对医学口语对照层没用，一律不收。
    if re.search(r"经授权转载自|本文转自|原标题：", title + body[:600]):
        return None, "repost(公众号转载稿)"
    if not _healthish(title, body):
        return None, "off-topic(非健康题材)"

    tag = ""
    mt = re.search(r'styled__TagsWrap[^"]*"[^>]*><li>([^<]{2,10})</li>', page)
    if mt:
        tag = mt.group(1)
    words = ""
    mw = re.search(r'<span class="word-count">(\d+)', page)
    if mw:
        words = mw.group(1)

    notes = {}
    if tag:
        notes["栏目"] = tag
    if words:
        notes["原文字数"] = words
    if reviewer:
        notes["审核"] = reviewer

    full_author = _full_author("guokr", body)
    fname = f"guokr-{aid}-{slugify(title, aid)}.txt"
    return dict(fname=fname, url=url, title=title, author=full_author,
                publisher="果壳网（科学人）", date=date, body=body, notes=notes), None


# ---------------------------------------------------------------- 163 健康

NETEASE_LISTS = [
    "https://jiankang.163.com/",
    "https://jiankang.163.com/special/gongkaike/",
    "https://jiankang.163.com/special/health_health/",
    "https://jiankang.163.com/special/yangsheng_n/",
    "https://jiankang.163.com/special/jkgz/",
]
NETEASE_RE = re.compile(r"https?://www\.163\.com/jiankang/article/([A-Z0-9]+)\.html")


def netease_candidates():
    out, seen = [], []
    for lp in NETEASE_LISTS:
        st, page = fetch(lp)
        if st != 200:
            continue
        for code in NETEASE_RE.findall(page):
            if code not in seen:
                seen.append(code)
                out.append(code)
    return out


def netease_extract(code):
    url = f"https://www.163.com/jiankang/article/{code}.html"
    st, page = fetch(url)
    if st != 200:
        return None, st or "fail"
    mt = re.search(r'<h1[^>]*>([^<]+)</h1>', page)
    title = html.unescape(mt.group(1)).strip() if mt else f"163jiankang-{code}"

    mb = (re.search(r'<div class="post_body"[^>]*>([\s\S]*?)'
                    r'(?:<div class="post_author"|<div class="end_title|</article>)', page)
          or re.search(r'id="artibody"[^>]*>([\s\S]*?)'
                       r'(?:<div class="end_title|<div class="post_author")', page))
    body = strip_tags(mb.group(1)) if mb else ""
    body = re.sub(r"(?m)^\s*[#.][\w-]+\s*\{[^}]*\}\s*$", "", body)  # 残留内联样式
    body = re.sub(r"(?m)^\s*(?:with\(document\)|window\.).*$", "", body)
    body = body.strip()
    if cjk_count(body) < MIN_CJK:
        return None, "short-body"

    date = ""
    md = re.search(r"(\d{4}-\d{2}-\d{2})\s+(?:\d{2}:\d{2})", page)
    if md:
        date = md.group(1)

    # 网易健康的署名在正文首段，见 _author_netease
    author, reviewer = _author_netease(body)

    led = re.search(r"责任编辑\s*[：:]\s*([^\s<]{2,24})", page)
    editor = led.group(1) if led else ""

    if not author and not reviewer:
        return None, "no-signed-author"
    if "独家策划" in body[:600] and not author:
        return None, "aggregator-rewrite"
    if not _healthish(title, body):
        return None, "off-topic(非健康题材)"

    full_author = "；".join(p for p in
                            (author, f"审核/受访：{reviewer}" if reviewer else "") if p)
    notes = {}
    if editor:
        notes["责任编辑"] = editor
    fname = f"163jiankang-{code}-{slugify(title, code)}.txt"
    return dict(fname=fname, url=url, title=title, author=full_author,
                publisher="网易健康", date=date, body=body, notes=notes), None


# ---------------------------------------------------------------- 生命时报

LIFETIMES_LISTS = [
    "https://www.lifetimes.cn/life",
    "https://www.lifetimes.cn/medicine",
    "https://www.lifetimes.cn/news",
    "https://www.lifetimes.cn/hotspot",
    "https://www.lifetimes.cn/longevity",
    "https://www.lifetimes.cn/mothers",
]
LT_RE = re.compile(r"/article/([0-9A-Za-z]{8,14})(?:[\"'#?]|$)")


def lifetimes_candidates():
    out, seen = [], []
    for lp in LIFETIMES_LISTS:
        st, page = fetch(lp)
        if st != 200:
            continue
        for code in LT_RE.findall(page):
            if code not in seen:
                seen.append(code)
                out.append(code)
    return out


def _lt_json(page):
    m = re.search(r'var cs_article = "((?:[^"\\]|\\.)*)"', page, re.S)
    if not m:
        return None
    try:
        return json.loads(json.loads('"' + m.group(1) + '"'))
    except Exception:  # noqa: BLE001
        try:
            return json.loads(m.group(1).encode().decode("unicode_escape"))
        except Exception:  # noqa: BLE001
            return None


def lifetimes_extract(code):
    url = f"https://www.lifetimes.cn/article/{code}"
    st, page = fetch(url)
    if st != 200:
        return None, st or "fail"
    d = _lt_json(page)
    if not d:
        return None, "no-json-payload"
    title = (d.get("title") or "").strip()
    body = strip_tags(d.get("content") or "")
    if cjk_count(body) < MIN_CJK:
        return None, "short-body"

    date = ""
    ct = d.get("ctime")
    if isinstance(ct, (int, float)) and ct > 10 ** 11:
        date = time.strftime("%Y-%m-%d", time.localtime(ct / 1000.0))

    authors = [a.get("name", "").strip() for a in (d.get("author") or [])
               if isinstance(a, dict) and a.get("name")]
    # 报纸署名/专家栏规则见 _author_lifetimes
    full_author = _author_lifetimes(body, authors)
    if not full_author:
        return None, "no-signed-author"
    if not _healthish(title, body):
        return None, "off-topic(非健康题材)"

    src = (d.get("source") or {}).get("name") or "生命时报"
    editor = (d.get("editor") or {}).get("name") or ""
    notes = {}
    if editor:
        notes["编辑"] = editor
    fname = f"lifetimes-{code}-{slugify(title, code)}.txt"
    return dict(fname=fname, url=url, title=title, author=full_author,
                publisher=src, date=date, body=body, notes=notes), None


# ---------------------------------------------------------------- drivers

SOURCES = [
    ("guokr.com", "guokr", guokr_candidates, guokr_extract),
    ("jiankang.163.com", "netease", netease_candidates, netease_extract),
    ("lifetimes.cn", "lifetimes", lifetimes_candidates, lifetimes_extract),
]


def build_manifest():
    """Rebuild the manifest from the txt files actually on disk (idempotent:
    a re-run that skips already-saved articles still lists them)."""
    rows = []
    for p in sorted(OUT_DIR.glob("*.txt")):
        if p.name == "MANIFEST.txt":
            continue
        head = {}
        with open(p, encoding="utf-8") as fh:
            for line in fh:
                if line.startswith("# "):
                    k, _, v = line[2:].partition(":")
                    head[k.strip()] = v.strip()
                elif line.strip() and not line.startswith("#"):
                    break
        rows.append((p.name, head.get("标题", ""), head.get("URL", ""),
                     head.get("发布方/日期", "").split(" / ")[0],
                     head.get("发布方/日期", "").split(" / ")[-1] or "未标注"))
    return rows


def run(limit, dry):
    OUT_DIR.mkdir(parents=True, exist_ok=True)
    stats, skipped = {}, []
    for host, key, cand_fn, ext_fn in SOURCES:
        cands = cand_fn() or []
        print(f"[{host}] discovered {len(cands)} candidates")
        got, tried = 0, 0
        for cid in cands:
            if got >= limit or _dead.get(host):
                break
            art = err = None
            for _ in range(MAX_ATTEMPTS_PER_ARTICLE):
                tried += 1
                art, err = ext_fn(cid)
                if art is not None or (err and not str(err).startswith("HTTP")):
                    break
                time.sleep(DELAY)
            if art:
                wrote = write_article(**art)
                got += 1
                print(f"  + {wrote or art['fname'] + '  (already on disk)'}")
            else:
                skipped.append(f"{host}/{cid} — {err}")
        stats[host] = (got, tried)
        if _dead.get(host):
            skipped.append(f"{host} — 熔断: {_dead[host]}")

    rows = build_manifest()
    if dry:
        for host, got, tried in stats.items():
            print(f"DRY {host}: {got}")
        print(f"on disk: {len(rows)}")
        return

    lines = ["MANIFEST — 商业/新媒体平台原创署名科普（对照层语料，非权威数字来源）",
             "format: 相对文件名 | 标题 | 直链 | 发布方 | 日期",
             "NOTE: 本目录只做「口语怎么问 / 大家关心什么」的对照，"
             "任何数字必须回官方指南 raw 文本逐条核对，禁止直接引用本目录。",
             ""]
    lines += [f"{f} | {t} | {u} | {p} | {d}" for f, t, u, p, d in rows]
    lines += ["", f"# 统计: 共 {len(rows)} 篇", ""]
    for host, (got, tried) in stats.items():
        lines.append(f"# {host}: saved-this-run={got} attempts={tried}")
    lines.append("")
    for s in skipped:
        lines.append(f"# 已排除: {s}")
    lines += ["", "# 已排除: dxy.com（丁香医生）— robots.txt 明确声明"
                  "「任何大语言模型及其网络爬虫均不得访问或使用本网站内容」，按站方条款放弃",
              "# 已排除: cdstm.cn（中国数字科技馆）— 文章页正文客户端渲染，实测仅 ~280 汉字",
              "# 已排除: familydoctor.com.cn（家庭医生在线）— SEO 医院导流/通讯员供稿，非原创署名科普",
              "# 已排除: youlai.cn（有来医生）— 正文是短视频口播稿（~780 汉字），非文章",
              "# 不可达: thepaper.cn — 首页/文章页起初静态可读，频道页连续 HTTP 403"
              "「WAF拦截页面」，按要求停止继续请求该 host",
              ""]
    with open(OUT_DIR / "MANIFEST.txt", "w", encoding="utf-8") as fh:
        fh.write("\n".join(lines))
        fh.flush()
    print(f"\nwrote {len(manifest)} articles -> {OUT_DIR}")


if __name__ == "__main__":
    lim, dry = PER_HOST_LIMIT, ("--dry-run" in sys.argv)
    for i, a in enumerate(sys.argv):
        if a == "--limit" and i + 1 < len(sys.argv):
            lim = int(sys.argv[i + 1])
    run(lim, dry)
