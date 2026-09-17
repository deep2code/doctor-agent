"""MedlinePlus Genetics（公有领域，美国 NLM）。

raw: 官方单文件汇总 https://medlineplus.gov/download/ghr-summaries.xml（~8MB）
convert: health-condition-summary→kind=condition（body=description），
gene-summary→kind=gene（body=function），chromosome/mtdna→kind=chromosome。
"""

from __future__ import annotations

import re
import xml.etree.ElementTree as ET

from .. import http
from ..schema import CorpusDoc, corpus_set
from . import base

SUMMARIES_URL = "https://medlineplus.gov/download/ghr-summaries.xml"


def fetch(ctx):
    dest = ctx.raw_dir / "ghr-summaries.xml"
    if dest.exists() and dest.stat().st_size > 0:
        ctx.log(f"raw 已存在，跳过: {dest.name}")
        return
    http.download(SUMMARIES_URL, dest)
    ctx.log(f"下载完成 {dest.stat().st_size >> 10}KiB")


def _local(tag: str) -> str:
    return tag.rsplit("}", 1)[-1]


def _html_text(elem) -> str:
    """<html> 容器 → 纯文本段落（itertext，段间空行）。"""
    paras = []
    for child in elem:
        paras.append(" ".join("".join(child.itertext()).split()))
    return "\n\n".join(p for p in paras if p)


def _cap(text: str, max_bytes: int) -> str:
    if len(text.encode()) <= max_bytes:
        return text
    cut = text[:max_bytes]
    if "\n\n" in cut[max_bytes // 2:]:
        cut = cut[:cut.rfind("\n\n", max_bytes // 2)]
    return cut.rstrip() + "\n\n[truncated]"


def _docs_of(ctx, elem_name: str, kind: str, body_role: str, root):
    """按 summary 元素类型产出 CorpusDoc（elem_name=XML 元素名，kind=CorpusDoc.kind）。"""
    max_body = ctx.args.get("max_body_bytes") or 24576
    max_chapters = ctx.args.get("max_chapters") or 0
    out = []
    for elem in root:
        lt = _local(elem.tag)
        if lt != f"{elem_name}-summary" and not (
            elem_name == "chromosome" and lt.endswith("dna-summary")
        ):
            continue
        name_el = page_el = None
        body = ""
        for child in elem:
            t = _local(child.tag)
            if t == "name":
                name_el = child
            elif t == "ghr-page":
                page_el = child
            elif t == "text-list":
                for text in child:
                    role = ""
                    for sub in text:
                        if _local(sub.tag) == "text-role":
                            role = (sub.text or "").strip()
                        elif _local(sub.tag) == "html" and role == body_role:
                            body = _html_text(sub)
        title = (name_el.text or "").strip() if name_el is not None else ""
        if not title or not body:
            continue
        eid = elem.get("id") or re.sub(r"[^a-z0-9-]+", "-", title.lower())
        summary = body.split("\n\n", 1)[0][:600]
        out.append(CorpusDoc(
            source="medgen", id=f"medgen-{eid}", lang="en", title=title,
            summary=summary, kind=kind,
            url=(page_el.text or "").strip() if page_el is not None else "",
            body=_cap(body, max_body), keywords=[title.lower()],
        ))
        if max_chapters and len(out) >= max_chapters:
            break
    return out


def convert(ctx):
    src = ctx.raw_dir / "ghr-summaries.xml"
    root = ET.parse(src).getroot()
    docs = _docs_of(ctx, "health-condition", "condition", "description", root)
    ctx.log(f"conditions: {len(docs)}")
    docs += _docs_of(ctx, "gene", "gene", "function", root)
    ctx.log(f"+genes: {len(docs)} 总计")
    docs += _docs_of(ctx, "chromosome", "chromosome", "description", root)
    ctx.log(f"+chromosomes/mtdna: {len(docs)} 总计")
    out = base.out_path("medgen")
    base.write_json(out, corpus_set("medgen", base.today(), docs))
    ctx.log(f"写出 {out} ({out.stat().st_size >> 10}KiB)")
