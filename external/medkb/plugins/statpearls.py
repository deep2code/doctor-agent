"""StatPearls（英文专业医学全书，CC BY-NC-ND 4.0，自用/非商用）。

raw: LitArch FTP 整包 tar.gz（~1.9GB）。下载地址发现方式同 lactmed：
书页 https://www.ncbi.nlm.nih.gov/books/NBK430685/ 内 grep litarch 目录。
convert: JATS XML → CorpusDoc{kind=condition}；body 截断（--max-body-bytes），
章节全量转换（--max-chapters 试跑用）。
"""

from __future__ import annotations

import re
import tarfile
import xml.etree.ElementTree as ET

from .. import http
from ..schema import CorpusDoc, corpus_set
from . import base

BOOK_PAGE = "https://www.ncbi.nlm.nih.gov/books/NBK430685/"

# 临床相关小节标题 → section key（正文全部保留，sections 只存这几类）
SECTION_KEYS = [
    (re.compile(r"^etiology", re.I), "etiology"),
    (re.compile(r"^epidemiology", re.I), "epidemiology"),
    (re.compile(r"^pathophysiology", re.I), "pathophysiology"),
    (re.compile(r"^history and physical", re.I), "history_physical"),
    (re.compile(r"^(evaluation|assessment)", re.I), "evaluation"),
    (re.compile(r"^(treatment|management)", re.I), "treatment"),
    (re.compile(r"^differential diagnosis", re.I), "differential_diagnosis"),
    (re.compile(r"^prognosis", re.I), "prognosis"),
    (re.compile(r"^complications", re.I), "complications"),
    (re.compile(r"^deterrences", re.I), "prevention"),
    (re.compile(r"^pearls and other issues", re.I), "pearls"),
]

# 非临床内容（目录页/继续教育说明等）标题过滤
SKIP_TITLES = re.compile(
    r"^(ncbi bookshelf|continuing education activity|statpearls ?(reference|review)|"
    r"treasure island|appendix|disclaimer)", re.I)


def fetch(ctx):
    tar = ctx.raw_dir / "statpearls_NBK430685.tar.gz"
    if not tar.exists() or tar.stat().st_size == 0:
        page = http.get(BOOK_PAGE)
        m = re.search(r'href="(https://ftp\.ncbi\.nlm\.nih\.gov/pub/litarch/[0-9a-f]{2}/[0-9a-f]{2}/)"', page)
        if not m:
            raise RuntimeError("书页未找到 litarch 目录链接，FTP 布局可能已变")
        listing = http.get(m.group(1))
        m2 = re.search(r'href="([^"]+\.tar\.gz)"', listing)
        if not m2:
            raise RuntimeError("litarch 目录中未找到 tar.gz")
        url = m.group(1) + m2.group(1)
        ctx.log(f"发现下载地址: {url}")
        http.download(url, tar)
    xdir = ctx.raw_dir / "x"
    if not xdir.exists() or not any(xdir.iterdir()):
        ctx.log("解压 tar.gz（~数分钟）…")
        with tarfile.open(tar, "r:gz") as tf:
            tf.extractall(xdir, filter="data")
        ctx.log("解压完成")
    else:
        ctx.log("已解压，跳过")


def _local(tag: str) -> str:
    return tag.rsplit("}", 1)[-1]


def _text(el) -> str:
    return " ".join("".join(el.itertext()).split())


def _cap_bytes(text: str, max_bytes: int) -> str:
    """按 UTF-8 字节数截断（与 schema.MAX_BODY_BYTES 校验一致）。"""
    raw = text.encode("utf-8")
    if len(raw) <= max_bytes:
        return text
    return raw[:max_bytes - 16].decode("utf-8", "ignore").rstrip() + "\n\n[truncated]"


def _doc_from_article(path, max_body: int) -> CorpusDoc | None:
    try:
        root = ET.parse(path).getroot()
    except ET.ParseError:
        return None
    # BITS DTD：book-meta 里是 book-title（不同标签），首个 <title> 即章节标题
    title = ""
    for el in root.iter():
        if _local(el.tag) == "title":
            title = _text(el)
            break
    if not title or SKIP_TITLES.match(title):
        return None
    sections: dict[str, str] = {}
    body_parts: list[str] = []
    for sec in root.iter():
        if _local(sec.tag) != "sec":
            continue
        sec_title = ""
        paras: list[str] = []
        for child in sec:
            if _local(child.tag) == "title":
                sec_title = _text(child)
            elif _local(child.tag) == "p":
                paras.append(_text(child))
        if not paras:
            continue
        text = "\n\n".join(p for p in paras if p)
        if sec_title and SKIP_TITLES.match(sec_title):
            continue
        body_parts.append(f"{sec_title}\n{text}" if sec_title else text)
        for pat, key in SECTION_KEYS:
            if pat.match(sec_title):
                sections.setdefault(key, text[:4000])
                break
    if not body_parts:
        return None
    body = _cap_bytes("\n\n".join(body_parts), max_body)
    summary = body.split("\n\n", 1)[0][:600]
    return CorpusDoc(
        source="statpearls", id=f"statpearls-{path.stem}", lang="en", title=title,
        summary=summary, kind="condition",
        sections=sections or None, keywords=[title.lower()], body=body,
    )


def convert(ctx):
    max_body = ctx.args.get("max_body_bytes") or 24576
    max_chapters = ctx.args.get("max_chapters") or 0
    docs = []
    for xml_path in sorted((ctx.raw_dir / "x").rglob("*.nxml")):
        d = _doc_from_article(xml_path, max_body)
        if d:
            docs.append(d)
            if max_chapters and len(docs) >= max_chapters:
                break
    ctx.log(f"{len(docs)} 章节")
    out = base.out_path("statpearls")
    base.write_json(out, corpus_set("statpearls", base.today(), docs))
    ctx.log(f"写出 {out} ({out.stat().st_size >> 10}KiB)")
