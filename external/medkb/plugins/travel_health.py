"""WHO《国际旅行与健康》各国黄热病/疟疾疫苗接种要求列表（中文，2019-07 版）。

raw: https://www.who.int/docs/default-source/documents/emergencies/
     travel-advice/yellow-fever-vaccination-requirements-country-list-2019-zh.pdf
convert: 按 "国家名行(短行、无冒号) → 黄热病/疟疾/其他要求段" 切分，每国一条
CorpusDoc；"X，见Y" 指针行跳过。要求随年份变化，正文保留 WHO 标注年份。
"""

from __future__ import annotations

import re

from .. import http
from ..schema import CorpusDoc, corpus_set
from . import base

PDF_URL = ("https://www.who.int/docs/default-source/documents/emergencies/"
           "travel-advice/yellow-fever-vaccination-requirements-country-list-2019-zh.pdf")
RAW_PDF = "yfv_country_list_zh.pdf"

PAGE_NO = re.compile(r"^\d{1,3}$")
YF_RE = re.compile(r"^黄热病（\d{4}）$")
FOOTNOTE_RE = re.compile(r"^\d{1,2}\s")
COUNTRY_MAXLEN = 22


def fetch(ctx):
    dest = ctx.raw_dir / RAW_PDF
    if dest.exists() and dest.stat().st_size > 100_000:
        ctx.log(f"raw 已存在，跳过: {dest.name}")
        return
    dest.parent.mkdir(parents=True, exist_ok=True)
    http.download(PDF_URL, dest)
    ctx.log(f"下载完成 {dest.stat().st_size >> 10}KiB")


def _is_country(s: str) -> bool:
    if not s or len(s) > COUNTRY_MAXLEN:
        return False
    if "，见" in s or "：" in s or ":" in s or "。" in s:
        return False
    if PAGE_NO.match(s) or FOOTNOTE_RE.match(s) or s.startswith("国家列表"):
        return False
    return True


def convert(ctx):
    import pymupdf

    doc = pymupdf.open(ctx.raw_dir / RAW_PDF)
    lines: list[str] = []
    for pno in range(len(doc)):
        for raw in doc[pno].get_text().splitlines():
            s = raw.strip()
            if s and not PAGE_NO.match(s):
                lines.append(s)

    docs: list[CorpusDoc] = []
    i = 0
    while i < len(lines):
        # anchor: a 黄热病（YYYY） line whose predecessor is a bare country name
        if YF_RE.match(lines[i]) and i > 0 and _is_country(lines[i - 1]):
            name = lines[i - 1]
            buf = [lines[i]]
            j = i + 1
            while j < len(lines):
                if FOOTNOTE_RE.match(lines[j]) or (j + 1 < len(lines)
                                                    and YF_RE.match(lines[j + 1])
                                                    and _is_country(lines[j])):
                    break
                if "，见" in lines[j]:
                    j += 1
                    continue
                buf.append(lines[j])
                j += 1
            body = "\n".join(buf).strip()
            yf = [l for l in buf if l.startswith("国家入境要求")]
            docs.append(CorpusDoc(
                source="travel_health", id=f"travel-{len(docs) + 1:03d}",
                lang="zh", title=f"前往{name}的国际旅行健康要求",
                summary=(yf[0] if yf else body.splitlines()[0])[:600],
                kind="", url=PDF_URL,
                keywords=[name, "旅行", "黄热病", "疟疾", "疫苗", "入境"],
                body=body,
            ))
            i = j
            continue
        i += 1
    ctx.log(f"countries: {len(docs)}")
    out = base.out_path("travel_health")
    base.write_json(out, corpus_set("travel_health", "2019-07-01", docs))
    ctx.log(f"写出 {out} ({out.stat().st_size >> 10}KiB)")
