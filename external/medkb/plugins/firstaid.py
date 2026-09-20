"""中国红十字会《应急救护手册》（IFRC 中文版小册子，56 页 PDF）。

raw: https://img.gmw.cn/rendao/yingjiejiuhushouce.pdf
convert: 内容页首行为场景标题（其后为无编号步骤行），插图页只有散落的
步骤编号数字（1/2/3）→ 跳过。约 21 个急救场景，每场景一条 CorpusDoc。
"""

from __future__ import annotations

import re

from .. import http
from ..schema import CorpusDoc, corpus_set
from . import base

PDF_URL = "https://img.gmw.cn/rendao/yingjiejiuhushouce.pdf"
RAW_PDF = "hongshizhi_jiuhushouce.pdf"

STEP_NO = re.compile(r"^\d{1,2}$")
# front matter (运动介绍/基本原则) 不是急救内容，从"意识丧失"页开始收
SKIP_TITLES = ("国际红十字", "红十字会与红新月会", "基本原则", "人道",
               "每个组成部分", "红十字运动和应急救护", "基本的日常急救技巧",
               "急救")


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
    pages = []
    for pno in range(len(doc)):
        lines = [l.strip() for l in doc[pno].get_text().splitlines() if l.strip()]
        content = [l for l in lines if not STEP_NO.match(l)]
        if len(content) >= 3:
            pages.append(content)

    docs: list[CorpusDoc] = []
    n = 0
    for content in pages:
        title = content[0]
        if title in SKIP_TITLES or title.endswith("，见"):
            continue
        if not (title.endswith("急救") or title in ("意识丧失", "气道梗阻", "昆虫蛰伤和叮咬")):
            continue
        body = "\n".join(content[1:])
        if len(body) < 20:
            continue
        n += 1
        sub = content[1] if len(content) > 1 and len(content[1]) < 16 else ""
        disp = f"{title}（{sub}）" if sub and title == "意识丧失" else title
        docs.append(CorpusDoc(
            source="firstaid", id=f"firstaid-{n:03d}", lang="zh",
            title=disp, summary=body[:600], kind="",
            url="https://img.gmw.cn/rendao/yingjiejiuhushouce.pdf",
            keywords=[disp.replace("的急救", ""), "急救", "现场处理", "拨打120"],
            body=body,
        ))
    ctx.log(f"scenarios: {len(docs)}")
    out = base.out_path("firstaid")
    base.write_json(out, corpus_set("firstaid", "2013-02-01", docs))
    ctx.log(f"写出 {out} ({out.stat().st_size >> 10}KiB)")
