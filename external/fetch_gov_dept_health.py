#!/usr/bin/env python3
"""Crawl public health knowledge texts from four central-ministry sites.

Sources (entry points only, no other hosts):
  1. 生态环境部  mee.gov.cn      噪声/辐射/生态环境状况公报/全民行动文件
  2. 国家体育总局 sport.gov.cn    运动促进健康文件 + 全民健身科普资讯
  3. 教育部      moe.gov.cn      学校卫生/近视/心理健康/校园食品等文件库
  4. 上海市市场监管局 scjgj.sh.gov.cn  1072 消费提示栏目

Writes raw/*.txt + raw/MANIFEST.txt under external/gov_dept_health/.
No concurrency; >=0.9s between requests; max 2 tries per URL.

  python3 external/fetch_gov_dept_health.py            # full run
"""
import html as htmlmod
import io
import os
import re
import sys
import time
import urllib.request
from urllib.parse import urljoin

OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "gov_dept_health")
RAW = os.path.join(OUT, "raw")
UA = {
    "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
    "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
}
CJK = re.compile(r"[\u4e00-\u9fff]")
MIN_CJK = 400
SLEEP = 0.9

excluded = []   # (url, title, reason)
unreachable = []  # (url, reason)
manifest = []   # (fname, title, url, org, date)
seen_urls = set()


def fetch(url, tries=3, binary=False):
    hdr = dict(UA)
    m = re.match(r"(https?://[^/]+/)", url)
    if m:
        hdr["Referer"] = m.group(1)
    for i in range(tries):
        try:
            req = urllib.request.Request(url, headers=hdr)
            with urllib.request.urlopen(req, timeout=30) as r:
                data = r.read()
            if binary:
                return data
            for enc in ("utf-8", "gb18030"):
                try:
                    return data.decode(enc)
                except UnicodeDecodeError:
                    continue
            return data.decode("utf-8", "replace")
        except Exception as e:  # noqa: BLE001
            code = getattr(e, "code", None)
            if code in (403, 429) and i < tries - 1:
                time.sleep(8 * (i + 1))  # WAF cooldown, then retry
                continue
            if i == tries - 1:
                unreachable.append((url, repr(e)[:80]))
                return None
            time.sleep(1.5)
    return None


def polite_sleep():
    time.sleep(SLEEP)


def decode_entities(text):
    return htmlmod.unescape(text).replace("\u2002", " ").replace("\u00a0", " ")


def clean_fragment(fragment):
    """HTML fragment -> verbatim plain-text lines (no rewriting)."""
    fragment = re.sub(r"<(script|style)[^>]*>.*?</\1>", "", fragment, flags=re.S | re.I)
    fragment = re.sub(r"<br\s*/?>", "\n", fragment, flags=re.I)
    fragment = re.sub(r"</p>", "\n", fragment, flags=re.I)
    fragment = re.sub(r"<[^>]+>", "", fragment)
    fragment = decode_entities(fragment)
    lines = []
    for ln in fragment.splitlines():
        ln = ln.replace("\u3000", "　").strip()
        if not ln:
            continue
        if re.search(r"^\s*[【\[]?(打印|字号|分享|纠错|责任编辑|来源[:：]|发布时间[:：]|访问)", ln):
            continue
        if CJK.search(ln) or re.search(r"\d", ln):
            lines.append(ln)
    return "\n".join(lines)


CONTAINER_STARTS = [
    'class="TRS_Editor"',
    'id="zoom"',
    'id="UCAP-CONTENT"',
    'class="neiright_JPZ_GK_CP',
    'class="Custom_UnionStyle"',
]
TERMINATORS = ['版权', '上一篇', '相关信息', '<footer', '主办：', '主办:', '网站地图',
               '扫一扫', '分享', '纠错', 'document.write', 'ztrelev']


def extract_body(doc):
    best = ""
    for marker in CONTAINER_STARTS:
        i = doc.find(marker)
        if i < 0:
            continue
        seg = doc[i:i + 900000]
        end = len(seg)
        for t in TERMINATORS:
            j = seg.find(t, 300)
            if 0 < j < end:
                end = j
        cand = clean_fragment(seg[:end])
        if len(CJK.findall(cand)) > len(CJK.findall(best)):
            best = cand
    if len(CJK.findall(best)) >= MIN_CJK:
        return best
    # generic fallback: keep every <p> with CJK (scjgj/sport simple pages)
    ps = re.findall(r"<p[^>]*>(.*?)</p>", doc, re.S)
    body = "\n".join(ps)
    if len(CJK.findall(body)) > len(CJK.findall(best)):
        return clean_fragment(body)
    return best


def page_title(doc, fallback=""):
    m = re.search(r"<title>(.*?)</title>", doc, re.S)
    t = decode_entities(m.group(1)).strip() if m else fallback
    t = re.split(r"[_|]", t)[0].strip()
    return t or fallback


def pub_date(url, doc=""):
    m = re.search(r"/(20\d{2})(\d{2})(\d{2})/", url) or re.search(r"t(20\d{2})(\d{2})(\d{2})_", url)
    if m:
        return f"{m.group(1)}-{m.group(2)}-{m.group(3)}"
    m = re.search(r'class="xqLyPc[^"]*">\s*(20\d\d-\d\d-\d\d)', doc)
    return m.group(1) if m else ""


def pdf_bytes_text(raw):
    try:
        from pypdf import PdfReader
        rd = PdfReader(io.BytesIO(raw))
        parts = []
        for pg in rd.pages:
            try:
                parts.append(pg.extract_text() or "")
            except Exception:  # noqa: BLE001
                continue
        txt = "\n".join(parts)
        # strip lone surrogates that cannot be encoded
        return txt.encode("utf-8", "replace").decode("utf-8")
    except Exception as e:  # noqa: BLE001
        print(f"  ! pdf parse fail: {e}", file=sys.stderr)
        return ""


def save(site, ident, title, url, org, date, body):
    body = body.strip()
    if len(CJK.findall(body)) < MIN_CJK:
        excluded.append((url, title, f"正文中文不足{MIN_CJK}字"))
        return False
    safe = re.sub(r"[^\w\u4e00-\u9fff]+", "_", title)[:40].strip("_")
    fname = f"{site}_{ident}_{safe}.txt"
    path = os.path.join(RAW, fname)
    with open(path, "w", encoding="utf-8") as fh:
        fh.write(f"# URL: {url}\n# 标题: {title}\n# 发布方/日期: {org} {date}\n\n{body}\n")
    manifest.append((fname, title, url, org, date))
    print(f"  + {fname} ({len(CJK.findall(body))}字)")
    return True


def parse_header(path):
    title = url = org = date = ""
    with open(path, encoding="utf-8") as fh:
        for ln in fh:
            if ln.startswith("# URL: "):
                url = ln[len("# URL: "):].strip()
            elif ln.startswith("# 标题: "):
                title = ln[len("# 标题: "):].strip()
            elif ln.startswith("# 发布方/日期: "):
                parts = ln[len("# 发布方/日期: "):].split()
                date = parts[-1] if parts and parts[-1][:4].isdigit() else ""
                org = " ".join(parts[:-1]) if date else " ".join(parts)
            else:
                break
    return title, url, org, date


def load_existing():
    """Register files already on disk (resume mode): their URLs are skipped
    forever after, so a rerun never stores a duplicate."""
    import glob
    for path in sorted(glob.glob(os.path.join(RAW, "*.txt"))):
        if path.endswith("MANIFEST.txt"):
            continue
        title, url, org, date = parse_header(path)
        manifest.append((os.path.basename(path), title, url, org, date))
        if url:
            seen_urls.add(url)


def handle_article(site, ident, url, title, org):
    """Fetch one HTML article page; PDF-attach aware (MEE reports)."""
    if url in seen_urls:
        return False
    seen_urls.add(url)
    doc = fetch(url)
    polite_sleep()
    if doc is None:
        return False
    title = title or page_title(doc)
    date = pub_date(url, doc)
    body = extract_body(doc)
    if len(CJK.findall(body)) < MIN_CJK and site == "mee":
        # reports ship as PDF attachments on the same page
        m = re.search(r'href="([^"]+\.pdf)"', doc, re.I)
        if m:
            pdf_url = urljoin(url, m.group(1))
            raw = fetch(pdf_url, binary=True)
            polite_sleep()
            if raw:
                body = pdf_bytes_text(raw)
                if len(CJK.findall(body)) >= MIN_CJK:
                    return save(site, ident, title, pdf_url, org, date, body)
    if len(CJK.findall(body)) >= MIN_CJK:
        return save(site, ident, title, url, org, date, body)
    excluded.append((url, title, "正文过短且无可用PDF附件"))
    return False


# ---------------------------------------------------------------- filters
BAD = re.compile(r"会议|座谈|会见|会见|研修|培训班|人事|任免|表彰|先进|巡视|党建|纪检|招标|中标|采购|决算|预算|公示名单|公务员|职称|论文征集|名单的公示|推荐信|来访|调研|慰问|外事")


def listing_pairs(doc, base, href_re, skip_re=None):
    out = []
    for l, t in re.findall(r'href="([^"]+)"[^>]*>\s*([^<]{6,90})\s*<', doc):
        t = decode_entities(t.strip())
        u = urljoin(base, l)
        if not re.search(href_re, u):
            continue
        if skip_re and re.search(skip_re, u):
            continue
        out.append((u, t))
    # dedupe preserving order
    seen = set()
    pairs = []
    for u, t in out:
        if u not in seen:
            seen.add(u)
            pairs.append((u, t))
    return pairs


# ---------------------------------------------------------------- main
def main():
    os.makedirs(RAW, exist_ok=True)
    load_existing()

    # ---- 1. 生态环境部 -------------------------------------------------
    mee_noise = re.compile(r"噪声污染防治报告")
    mee_rad = re.compile(r"辐射环境质量报告|辐射环境")
    print("### MEE 噪声报告")
    doc = fetch("https://www.mee.gov.cn/hjzl/sthjzk/hjzywr/")
    polite_sleep()
    n = 0
    if doc:
        for u, t in listing_pairs(doc, "https://www.mee.gov.cn/hjzl/sthjzk/hjzywr/", r"/hjzywr/20\d{4}/t.*\.shtml"):
            if mee_noise.search(t) and n < 3:
                if handle_article("mee", f"noise{n}", u, t, "生态环境部"):
                    n += 1
    print("### MEE 辐射环境质量报告")
    doc = fetch("https://www.mee.gov.cn/hjzl/hjzlqt/hyfshj/")
    polite_sleep()
    n = 0
    if doc:
        for u, t in listing_pairs(doc, "https://www.mee.gov.cn/hjzl/hjzlqt/hyfshj/", r"/hyfshj/20\d{4}/t.*\.shtml"):
            if n >= 2:
                break
            if not mee_rad.search(t):
                continue
            if handle_article("mee", f"rad{n}", u, t, "生态环境部"):
                n += 1
    # 辐射/公报 direct PDFs from the hjzl overview page
    doc = fetch("https://www.mee.gov.cn/hjzl/")
    polite_sleep()
    if doc:
        got = 0
        for u, t in re.findall(r'href="([^"]+\.pdf)"[^>]*>\s*([^<]{4,40})\s*<', doc):
            u = urljoin("https://www.mee.gov.cn/hjzl/", u)
            t = decode_entities(t.strip())
            if got >= 3:
                break
            if not re.search(r"辐射环境|生态环境状况公报", t):
                continue
            if u in seen_urls:
                continue
            seen_urls.add(u)
            raw = fetch(u, binary=True)
            polite_sleep()
            if not raw:
                continue
            body = pdf_bytes_text(raw)
            date = pub_date(u)
            if save("mee", f"pub{got}", t, u, "生态环境部", date, body):
                got += 1
    print("### MEE 全民行动文件（宣传教育→信息公开文稿）")
    doc = fetch("https://www.mee.gov.cn/ywgz/xcjy/shxc/")
    polite_sleep()
    if doc:
        got = 0
        for u, t in listing_pairs(doc, "https://www.mee.gov.cn/ywgz/xcjy/shxc/", r"xxgk2018/xxgk/xxgk0\d/.*\.html"):
            if got >= 2:
                break
            if BAD.search(t) or not re.search(r"全民行动|指导意见|行动计划|促进计划", t):
                continue
            if handle_article("mee", f"gov{got}", u, t, "生态环境部（等部委联合）"):
                got += 1

    # ---- 3. 教育部 ----------------------------------------------------
    moe_pick = re.compile(
        r"近视|肥胖|脊柱|龋齿|心理健康|营养|膳食|食品安全|卫生|健康|体育|体质|视力|眼|睡眠|流感|结核|艾滋|防溺|消防|急救|五健|校医|体检|生长|骨骼")
    moe_bad = re.compile(BAD.pattern + r"|财务|资助|经费|学历|学位|招生|考试|就业|招聘|职称|教师待遇|教材|语言|文字|学校设置|从业禁止|岗位设置|督导|评估|认定|试点名单|备案")
    print("### MOE 文件库 moe_1779 (8页)")
    got = 0
    for page in range(8):
        listu = "http://www.moe.gov.cn/jyb_xxgk/moe_1777/moe_1779/" + ("" if page == 0 else f"index_{page}.html")
        doc = fetch(listu)
        polite_sleep()
        if doc is None:
            break
        for u, t in listing_pairs(doc, "http://www.moe.gov.cn/jyb_xxgk/moe_1777/moe_1779/", r"/moe_1777/moe_1779/20\d{4}/t.*\.html"):
            if got >= 22:
                break
            if moe_bad.search(t) or not moe_pick.search(t):
                continue
            ident = re.search(r"_(\d+)\.html", u).group(1)
            if handle_article("moe", ident, u, t, "教育部等部委（见正文署名）"):
                got += 1
    print("  saved:", got)

    # ---- 4. 上海市市场监管局 ------------------------------------------
    print("### SH 1072 消费提示")
    doc = fetch("https://scjgj.sh.gov.cn/1072/")
    polite_sleep()
    if doc:
        got = 0
        sh_pick = re.compile(r"提示|指南|解读|消费|食品|安全")
        for u, t in listing_pairs(doc, "https://scjgj.sh.gov.cn/1072/", r"scjgj\.sh\.gov\.cn/1072/\d{8}/[0-9a-f]+\.html"):
            if got >= 10:
                break
            if not sh_pick.search(t):
                continue
            if handle_article("shscj", u.rsplit("/", 2)[-2], u, t, "上海市市场监督管理局"):
                got += 1

    # ---- 2. 国家体育总局（放最后：该站 WAF 对连发敏感，先冷却）----
    print("### SPORT 冷却 180 秒后抓取（避免 403 限流）")
    time.sleep(180)

    sport_pick = re.compile(
        r"运动促进健康|全民健身|科学健身|体质|运动处方|青少年体育|体育锻炼|体育强国|身体活动|运动技能|冬奥.*健身|健身标准")
    print("### SPORT 公文（n315/n20001395）")
    for i, (su, st) in enumerate([
        ("https://www.sport.gov.cn/n315/n20001395/c29071988/content.html",
         "体育总局关于印发《关于推动运动促进健康事业高质量发展的指导意见》的通知"),
        ("https://www.sport.gov.cn/n315/n20001395/c29789336/content.html",
         "体育总局关于印发《体育强国建设“十五五”规划》的通知"),
    ]):
        handle_article("sport", f"seed{i}", su, st, "国家体育总局")
    doc = fetch("https://www.sport.gov.cn/n315/n20001395/index.html")
    polite_sleep()
    if doc:
        got = 0
        for u, t in listing_pairs(doc, "https://www.sport.gov.cn/n315/n20001395/index.html", r"/n315/.*/c\d+/content\.html"):
            if got >= 4:
                break
            if not sport_pick.search(t):
                continue
            if handle_article("sport", re.search(r"/(c\d+)/", u).group(1), u, t, "国家体育总局"):
                got += 1
    print("### SPORT 资讯科普（n20001280）")
    doc = fetch("https://www.sport.gov.cn/n20001280/index.html")
    polite_sleep()
    if doc:
        got = 0
        pop = re.compile(r"健身|科学健身|锻炼|体质|运动促进|体卫|课间|指南|处方|如何|知识|长高|减脂|跑步|步行|睡眠|课后")
        neg = re.compile(r"赛|杯|锦标|夺|金牌|奖牌|战绩|小将|名将|开幕|闭幕|展演|展示|启动|论坛|峰会|颁奖|训练|交流|会见|公益|联赛|SUPER|冬运会|全运")
        for u, t in listing_pairs(doc, "https://www.sport.gov.cn/n20001280/index.html", r"/n20001280/.*/c\d+/content\.html"):
            if got >= 8:
                break
            if not pop.search(t) or BAD.search(t) or neg.search(t):
                continue
            if handle_article("sport", re.search(r"/(c\d+)/", u).group(1), u, t, "国家体育总局（中国体育报报道）"):
                got += 1

    # ---- manifest -------------------------------------------------------
    mp = os.path.join(RAW, "MANIFEST.txt")
    dedup = {}
    for f, t, u, org, d in manifest:
        dedup.setdefault(f, (f, t, u, org, d))
    final = list(dedup.values())
    with open(mp, "w", encoding="utf-8") as fh:
        for f, t, u, org, d in final:
            fh.write(f"{f} | {t} | {u} | {org} | {d}\n")
        if excluded:
            fh.write("# 已排除:\n")
            for u, t, why in excluded:
                fh.write(f"# 已排除: {t[:40]} {u} （{why}）\n")
        if unreachable:
            fh.write("# 不可达:\n")
            for u, why in unreachable:
                fh.write(f"# 不可达: {u} {why}\n")
    print(f"\nTOTAL saved: {len(final)}  excluded: {len(excluded)}  unreachable: {len(unreachable)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
