#!/usr/bin/env python3
"""Bulk-download State Council / NHC policy documents (gov.cn 政策文件库).

Pure download step: writes text extracts under external/gov_health/raw/ plus
index.tsv. Touches nothing under internal/knowledge/.

  python3 external/fetch_gov_health.py              # full run
  python3 external/fetch_gov_health.py -limit 5     # smoke test
"""
import html
import json
import os
import re
import subprocess
import sys
import time
from concurrent.futures import ThreadPoolExecutor
from urllib.parse import urljoin

import requests

API = "https://sousuo.www.gov.cn/search-gov/data"
OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "gov_health")
RAW = os.path.join(OUT, "raw")
PDFDIR = os.path.join(OUT, "pdf")
PAGE_SIZE = int(os.environ.get("GOV_PAGE_SIZE", "50"))
ENUM_MAX_PAGES = int(os.environ.get("GOV_ENUM_MAX_PAGES", "300"))
MAX_DOCS = int(os.environ.get("GOV_MAX_DOCS", "6000"))
PDF_PER_DOC = 2  # cap attachment PDFs per document

UA = {
    "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
    "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
}

# Catalogue categories walked in full (gongbao is the 公报 mirror, mostly duplicates).
CATS = ("gongwen", "bumenfile", "otherfile")
# Documents published by these bodies are the ones we treat as authoritative.
OK_ORG = re.compile(r"卫生|健康|疾控|药监|医保|中医药|国务院|应急|市场监管|教育|民政|体育|人力社会保障|农业农村")
BAD_TITLE = re.compile(
    r"会议纪要|征求意见|答复|建议的函|政协|人大|表彰|先进|党建|巡视|预算|决算"
    r"|解读|答记者问|访谈|新闻稿|通稿"
    r"|预案|演练|联席会议|领导小组|机构编制|人员编制|职责内设|招标|中标|政府采购"
    r"|培训班|论坛|展会|博览会|通报表扬|行政处罚|行政复议|外事|接待|车辆购置"
    r"|煤矿|矿山|化工|烟花爆竹|尾矿|旅游|文物|林业|草原|地震台网|网络安全"
)
BAD_URL = re.compile(r"/lianbo|/yaowen|解读|答记者问|访谈")
HEALTH = re.compile(
    r"卫生|健康|医疗|医药|医用|药品|药物|药师|处方|疫苗|接种|免疫|传染病|防控|防疫"
    r"|疾病|大病|慢病|慢性病|高血压|糖尿病|肿瘤|癌症|结核|肝炎|艾滋|罕见病|地方病"
    r"|职业病|尘肺|疫情|诊疗|诊治|临床|医院|基层卫生|社区卫生|乡镇卫生院|村卫生室"
    r"|家庭医生|分级诊疗|转诊|护理|康复|安宁疗护|姑息|医疗服务|医疗质量|医院感染"
    r"|病历|体检|筛查|医保|医疗救助|药品目录|集中带量"
    r"|营养|膳食|食品安全|食品标准|保健食品|母乳|喂养|辅食"
    r"|妇幼|孕产妇|出生缺陷|托育|婴幼儿|儿童|青少年|学生|校园|近视|口腔|牙齿|视力|眼科"
    r"|老年|老龄|养老|失能|跌倒|心理|精神卫生|睡眠|戒烟|控烟|烟草|体重|肥胖"
    r"|健身|运动促进|急救|救护|突发公共卫生|中医|中药|治未病|针灸"
    r"|爱国卫生|健康素养|健康教育|健康促进|健康中国|全民健康|生育|血液|器官捐献|残疾"
    r"|医养|医疗卫生|医疗机构|医用|医疗器械|公共卫生|卫生健康|卫生事业|卫生防疫"
    r"|基本药物|卫生人才|卫生创建|慢病|专病|共病|影像|检验检查|合理用药|安全用药"
)
# Industrial / trade policy that merely mentions a medical word is not public
# health knowledge.
DENY = re.compile(
    r"产业|园区|开发区|企业|公司|上市|股权|融资|进出口|关税|出口|反垄断|经营者集中"
    r"|知识产权|商标|专利|认证认可|检验检测机构|招商|博览会|展销|运动会|赛事|场馆"
    r"|标准地|用地|规划选址|环评|排污|采矿|许可审批|资质认定|示范区的设立"
)

s = requests.Session()
s.headers.update(UA)


def fix_surrogates(text):
    """Recombine valid UTF-16 surrogate pairs, drop unpaired ones.

    Some government PDFs carry broken character maps, so pypdf hands back lone
    surrogates that cannot be written as UTF-8 (this crashed one crawl).
    """
    buf = bytearray()
    i, n = 0, len(text)
    while i < n:
        o = ord(text[i])
        if 0xD800 <= o <= 0xDBFF and i + 1 < n and 0xDC00 <= ord(text[i + 1]) <= 0xDFFF:
            cp = 0x10000 + ((o - 0xD800) << 10) + (ord(text[i + 1]) - 0xDC00)
            buf += chr(cp).encode("utf-8")
            i += 2
            continue
        if 0xD800 <= o <= 0xDFFF:
            buf += b"?"
            i += 1
            continue
        buf += text[i].encode("utf-8")
        i += 1
    return buf.decode("utf-8")


def strip_tags(fragment):
    text = re.sub(r"<[^>]+>", "", fragment)
    return fix_surrogates(html.unescape(text).replace("\u2002", " ")).strip()


def get(url, tries=3, binary=False):
    for i in range(tries):
        try:
            r = s.get(url, timeout=30)
            r.encoding = "utf-8"
            if r.status_code == 200 and r.content:
                return r.content if binary else r.text
            if r.status_code in (404, 410):
                return None
        except Exception as e:  # noqa: BLE001
            print(f"  ! {url} {e}", file=sys.stderr)
        time.sleep(1.2 * (i + 1))
    return None


def enumerate_cat(cat):
    """Walk a whole catalogue (empty query = every document) page by page.

    The policy library has ~12.8k 部门文件; keyword search cannot reach them all,
    so we scroll the listing and filter on the publisher + title instead.
    """
    rows, total, page = [], None, 1
    while page <= ENUM_MAX_PAGES and (total is None or len(rows) < total):
        url = (
            f"{API}?t=zhengcelibrary&q=&timetype=timeqb"
            f"&sort=pubtime&sortType=1&searchfield=&p={page}&n={PAGE_SIZE}"
        )
        doc = get(url)
        try:
            node = json.loads(doc)["searchVO"]["catMap"].get(cat) or {}
            items = node.get("listVO") or []
            if total is None:
                total = int(node.get("totalCount") or 0)
        except Exception:  # noqa: BLE001
            break
        if not items:
            break
        rows.extend(items)
        if page % 10 == 0:
            print(f"  [{cat}] scrolled {len(rows)}/{total}", file=sys.stderr)
        page += 1
        time.sleep(0.12)
    print(f"  [{cat}] listed {len(rows)} docs (total={total})", file=sys.stderr)
    return rows


CONTENT_RE = re.compile(
    r'id="UCAP-CONTENT"(.*?)(?:<div[^>]+id="fileDown|</div>\s*<div)', re.S)
ATTACH_RE = re.compile(r'href="([^"]+?\.pdf)"', re.I)
CJK = re.compile(r"[\u4e00-\u9fff]")


def slug(title):
    t = re.sub(r"[^\w\u4e00-\u9fff]+", "_", title)[:46].strip("_")
    return t or "untitled"


def pdf_text(path):
    try:
        from pypdf import PdfReader
        rd = PdfReader(path)
        return fix_surrogates("\n".join((pg.extract_text() or "") for pg in rd.pages))
    except Exception as e:  # noqa: BLE001
        print(f"  ! pdf {path}: {e}", file=sys.stderr)
        return ""


def build_header(title, item, url, extra=""):
    return (
        f"来源: {item.get('puborg') or '中国政府网'}《{title}》"
        + (f"（{item['pcode']}）" if item.get("pcode") else "")
        + "\n"
        f"发布时间: {item.get('pubtimeStr') or ''}\n"
        f"URL: {url}\n"
        + (extra or "")
        + "抓取方式: 直连 gov.cn 政策文件库 + 正文提取，未改写\n"
        + "=" * 60 + "\n"
    )


def fetch_doc(item):
    url = item.get("url") or ""
    if "gov.cn" not in url:
        return None
    doc = get(url)
    if doc is None:
        return None
    title = strip_tags(item.get("title") or "")
    m = CONTENT_RE.search(doc)
    body = ""
    if m:
        body = re.sub(r"<[^>]+>", "\n", m.group(1))
        body = fix_surrogates(html.unescape(body))
        body = "\n".join(
            ln.strip() for ln in body.splitlines()
            if CJK.search(ln) or re.search(r"\d", ln)
        )
    cid = re.search(r"content_(\d+)", url)
    cid = cid.group(1) if cid else str(abs(hash(url)))[:10]
    rows = []
    if len(body) >= 400:
        fname = f"gov_{cid}_{slug(title)}.txt"
        with open(os.path.join(RAW, fname), "w", encoding="utf-8") as fh:
            fh.write(build_header(title, item, url) + body + "\n")
        rows.append("\t".join([fname, item.get("puborg") or "", title, url]))
    # attachments: gov.cn ships the annexes (指南/方案 正文常在 PDF 里)
    for au in list(dict.fromkeys(ATTACH_RE.findall(doc)))[:PDF_PER_DOC]:
        au = urljoin(url, au)
        if "/P0" not in au and ".pdf" not in au.lower():
            continue
        base = au.rsplit("/", 1)[-1]
        os.makedirs(PDFDIR, exist_ok=True)
        pth = os.path.join(PDFDIR, base)
        if not os.path.exists(pth):
            raw = get(au, binary=True)
            if not raw or len(raw) < 8000:
                continue
            with open(pth, "wb") as fh:
                fh.write(raw)
        txt = pdf_text(pth)
        if len(re.sub(r"\s", "", txt)) < 500:
            continue
        tfname = f"gov_{cid}_att_{base[:-4]}.txt"
        with open(os.path.join(RAW, tfname), "w", encoding="utf-8") as fh:
            fh.write(
                build_header(title, item, url,
                             extra=f"附件原件: {au}\n") + txt.strip() + "\n")
        rows.append("\t".join([tfname, item.get("puborg") or "", f"{title} (附件 {base})", au]))
    return rows


def main():
    os.makedirs(RAW, exist_ok=True)
    limit = 0
    if "-limit" in sys.argv:
        limit = int(sys.argv[sys.argv.index("-limit") + 1])
    index_path = os.path.join(OUT, "index.tsv")
    index_rows = []
    if os.path.exists(index_path):
        with open(index_path, encoding="utf-8") as fh:
            index_rows = [ln.rstrip("\n") for ln in fh if ln.strip()]
    seen = {r.split("\t")[-1] for r in index_rows if r.count("\t") >= 3}
    seen_urls = set(seen)

    picked = {}
    for cat in CATS:
        for it in enumerate_cat(cat):
            u = it.get("url") or ""
            title = strip_tags(it.get("title") or "")
            org = it.get("puborg") or ""
            if not u or u in seen_urls or BAD_TITLE.search(title):
                continue
            if BAD_URL.search(u) or DENY.search(title) or not HEALTH.search(title):
                continue
            if not OK_ORG.search(org):
                continue
            seen_urls.add(u)
            picked[u] = it
        if len(picked) >= MAX_DOCS:
            break
    print(f"candidate docs: {len(picked)}", file=sys.stderr)
    if "-dry" in sys.argv:
        for it in list(picked.values())[:60]:
            print(f"  {it.get('pubtimeStr','')}  {it.get('puborg','')}  {strip_tags(it['title'])[:52]}")
        return 0

    jobs = list(picked.values())
    if limit:
        jobs = jobs[:limit]
    written = 0
    with ThreadPoolExecutor(max_workers=6) as pool:
        for rows in pool.map(fetch_doc, jobs):
            if not rows:
                continue
            for row in rows:
                index_rows.append(row)
                written += 1
            with open(index_path, "w", encoding="utf-8") as fh:
                fh.write("\n".join(index_rows) + "\n")
            if written % 25 == 0:
                print(f"  .. {written} files", file=sys.stderr)
    print(f"DONE gov_health: +{written} files, {len(index_rows)} indexed", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
