"""国家卫健委《精神障碍诊疗规范（2020年版）》（官方 PDF，492 页）。

raw: https://www.nhc.gov.cn/cms-search/downFiles/9944cdd142574ea59c541d552fe345a9.pdf
convert: 正文按 "第N章/第N节" 标题切分，每节一条 CorpusDoc（约 90 条），
kind=condition，sections.chapter 记录所属章名。确定性抽取，不依赖 LLM。
"""

from __future__ import annotations

import re

from .. import http
from ..schema import CorpusDoc, corpus_set
from . import base

PDF_URL = ("https://www.nhc.gov.cn/cms-search/downFiles/"
           "9944cdd142574ea59c541d552fe345a9.pdf")
RAW_PDF = "jingshen_zhiliao.pdf"

CN = "[一二三四五六七八九十百]+"
CH_RE = re.compile(rf"^第{CN}章\s*(\S.*)$")
SEC_RE = re.compile(rf"^第{CN}节\s*(\S.*)$")
PAGE_NO = re.compile(r"^\d{1,3}$")


def fetch(ctx):
    dest = ctx.raw_dir / RAW_PDF
    if dest.exists() and dest.stat().st_size > 1_000_000:
        ctx.log(f"raw 已存在，跳过: {dest.name}")
        return
    dest.parent.mkdir(parents=True, exist_ok=True)
    http.download(PDF_URL, dest)
    ctx.log(f"下载完成 {dest.stat().st_size >> 10}KiB")


def convert(ctx):
    import pymupdf

    doc = pymupdf.open(ctx.raw_dir / RAW_PDF)
    # TOC is pages 1-3 (dotted leaders); body starts with 第一章 on a later page.
    cur_title = None
    cur_chapter = ""
    buf: list[str] = []
    docs: list[CorpusDoc] = []

    def flush():
        nonlocal cur_title, buf
        if not cur_title:
            return
        body = "\n".join(buf).strip()
        if len(body) < 40:
            cur_title, buf = None, []
            return
        title = cur_title
        if title in ("概述", "绪论") and cur_chapter:
            title = f"{cur_chapter}·{title}"
        enc = body.encode()
        if len(enc) > 24576:
            cut = body[: len(body) * 24000 // len(enc)]
            while len(cut.encode()) > 24576:
                cut = cut[: len(cut) - 200]
            body = cut.rsplit("\n", 1)[0] + "\n\n[截断：全文见来源 URL]"
        summary = body.split("\n\n", 1)[0][:600]
        docs.append(CorpusDoc(
            source="nhc_mental", id=f"nhc-mental-{len(docs) + 1:03d}", lang="zh",
            title=title, summary=summary, kind="condition",
            url="https://www.nhc.gov.cn/yzygj/c100068/202012/"
                "b4305ace9e14440792eb76d29602c88a.shtml",
            sections={"chapter": cur_chapter},
            keywords=[title] + ([cur_chapter] if cur_chapter and cur_chapter != title else []),
            body=body,
        ))
        cur_title, buf = None, []

    for pno in range(4, len(doc)):
        for raw in doc[pno].get_text().splitlines():
            s = raw.strip()
            if not s or PAGE_NO.match(s) or "...." in s:
                continue
            m = CH_RE.match(s)
            if m:
                flush()
                cur_chapter = m.group(1).strip()
                # chapters like 第四章 双相障碍 / 第五章 抑郁障碍 have no 节 —
                # the chapter itself is the document; a following 节 flushes
                # the (usually tiny, then dropped) chapter intro.
                cur_title = cur_chapter
                continue
            m = SEC_RE.match(s)
            if m:
                flush()
                cur_title = m.group(1).strip()
                continue
            if cur_title:
                buf.append(s)
    flush()
    ctx.log(f"sections: {len(docs)}")
    out = base.out_path("nhc_mental")
    base.write_json(out, corpus_set("nhc_mental", "2020-12-07", docs))
    ctx.log(f"写出 {out} ({out.stat().st_size >> 10}KiB)")
