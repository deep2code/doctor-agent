#!/usr/bin/env python3
"""Parse 国家医保药品目录(2025年) official PDF into data/medins_drugs.json.

Input: external/medins/medins-2025-att1.pdf (NHSA official attachment;
downloaded from nhsa.gov.cn downfile.jsp with a browser UA). Uses PyMuPDF
find_tables() so merged multi-line name cells come out correctly
(e.g. '二甲双胍\\n二甲双胍Ⅱ' sharing one 编号/剂型) — pypdf flow text
interleaves those rows and is unusable (the 2024 conversion was garbage).

Sections walked by standalone header lines (not the TOC):
  西药部分(implicit start) → 中成药部分 → 协议期内谈判药品部分 → stop at 中药饮片部分

Output rows: one per (drug name, 甲/乙) pair; forms merged from all catalog
lines listing that drug (base entries + ★(n) cross-class references).

Validation: plain-编号 coverage must hit 1..1446 (西药) / 1..1335 (中成药)
and both negotiation subsections.
"""

import json
import re
import sys
from collections import OrderedDict
from pathlib import Path

import pymupdf

ROOT = Path(__file__).resolve().parent.parent
PDF = ROOT / "external" / "medins" / "medins-2025-att1.pdf"
OUT = ROOT / "internal" / "knowledge" / "data" / "medins_drugs.json"

NUM = re.compile(r"^(\d+|★\(\d+\))$")
CATS = {"甲", "乙"}

# 剂型 column values used in 西药部分 (closed vocabulary in the catalog).
WEST_FORMS = {
    "口服常释剂型", "口服液体剂", "缓释控释剂型", "注射剂", "注射液",
    "软膏剂", "乳膏剂", "颗粒剂", "滴眼剂", "眼膏剂", "眼用凝胶剂", "凝胶剂",
    "吸入剂", "吸入溶液剂", "吸入粉雾剂", "喷雾剂", "气雾剂", "粉雾剂",
    "栓剂", "阴道栓", "栓剂(含阴道栓)", "灌肠剂", "口服散剂", "散剂",
    "口服粉剂", "口服混悬剂", "口服乳剂", "口服溶液剂", "外用液体剂",
    "外用冻干制剂", "贴剂", "透皮贴剂", "贴膏剂", "橡胶膏剂", "骨凝胶",
    "放射密封籽源", "密封籽源", "植入剂", "涂剂", "滴鼻剂", "滴耳剂",
    "舌下含片", "舌下喷雾剂", "口溶膜", "含漱液", "乳剂", "硬膏剂",
    "缓释植入剂", "肠溶常释剂型", "滴剂", "口服滴剂", "混悬液",
    "胶浆剂", "敷贴", "贴片", "鼻吸剂", "口腔崩解片",
}


def standalone_line(text: str, title: str) -> bool:
    """True if `title` occurs as its own line (section header, not TOC dots)."""
    return any(line.strip() == title for line in text.splitlines())


def split_names(cell: str):
    """Reassemble one 药品名称 cell into complete drug names.

    Cells are multi-line for two distinct reasons (indistinguishable by the
    table model): a single name wrapped mid-word, or several sibling names
    sharing one 编号/剂型 cell. Rules (in order):
      1. join: next fragment starts a bracket/continuation ( ( [ 、 / 用,
         is ≤3 chars (液, 射液, 制剂, 包装, N01…), or previous fragment ends
         mid-token (broken-suffix set / trailing hyphen / Latin letter / digit)
      2. split: fragments share a ≥2-char prefix (二甲双胍 | 二甲双胍Ⅱ)
      3. otherwise split (雷公藤片 | 雷公藤多苷[甙]片 handled by prefix 3).
    """
    frags = [f.strip() for f in (cell or "").split("\n") if f.strip()]
    if not frags:
        return []
    names = [frags[0]]
    for nxt in frags[1:]:
        prev = names[-1]
        common = 0
        for a, b in zip(prev, nxt):
            if a != b:
                break
            common += 1
        unbalanced = (
            prev.count("(") + prev.count("（") + prev.count("[") + prev.count("［")
            > prev.count(")") + prev.count("）") + prev.count("]") + prev.count("］")
        )
        if unbalanced or nxt[0] in "([、/%用口" or len(nxt) <= 3:
            names[-1] = prev + nxt  # unambiguous continuation fragment
        elif common >= 2:
            names.append(nxt)  # sibling names sharing one cell
        elif prev[-1] in BROKEN_ENDS or (prev[-1].isascii() and prev[-1].isalnum()):
            names[-1] = prev + nxt  # previous fragment broke mid-token
        else:
            names.append(nxt)
    # 中成药 merged rows: '消癌平丸、消癌平颗粒(…)、消癌平片…' -> 4 drugs
    out = []
    for nm in names:
        parts = [p for p in re.split(r"、(?![^()（）]*[)）])", nm) if p.strip()]
        out.extend(p.strip() for p in parts) if len(parts) > 1 else out.append(nm)
    return out


BROKEN_ENDS = set("注输激葡萄糖肪生疫多白蛋重胞微皮充口软合细人溶酸/-，")


def main():
    doc = pymupdf.open(PDF)
    section = "west"  # 西药部分 begins after 凡例; TOC pages have no table rows
    drugs: "OrderedDict[tuple, list]" = OrderedDict()
    seen = {"west": set(), "patent": set(), "nego_w": set(), "nego_p": set()}
    nego_bucket = seen["nego_w"]

    for page in doc:
        text = page.get_text()
        if "序号 饮片名称" in text or standalone_line(text, "中药饮片部分"):
            break  # remaining pages are 饮片 (not drug rows)
        if standalone_line(text, "中成药部分"):
            section = "patent"
        if standalone_line(text, "协议期内谈判药品部分"):
            section = "nego"
        if section == "nego":
            if standalone_line(text, "（一）西药") or standalone_line(text, "(一)西药"):
                nego_bucket = seen["nego_w"]
            if standalone_line(text, "（二）中成药") or standalone_line(text, "(二)中成药"):
                nego_bucket = seen["nego_p"]
        in_west = section == "west" or (section == "nego" and nego_bucket is seen["nego_w"])
        in_patent = not in_west

        for tbl in page.find_tables().tables:
            for row in tbl.extract():
                cells = row
                cat = num = None
                idx = None
                for i, c in enumerate(cells):
                    if c is None:
                        continue
                    cand = c.strip().replace(" ", "")
                    if NUM.match(cand) and i:
                        left = (cells[i - 1] or "").strip()
                        if left in CATS:
                            cat, num, idx = left, cand, i
                            break
                if num is None:
                    continue
                name_cell = (cells[idx + 1] or "") if idx + 1 < len(cells) else ""
                next_cell = (cells[idx + 2] or "").strip() if idx + 2 < len(cells) else ""
                names = split_names(name_cell)
                if not names:
                    continue
                if not num.startswith("★"):
                    nego_bucket.add(int(num)) if section == "nego" else seen[section].add(int(num))
                for nm in names:
                    forms = []
                    if in_west and next_cell in WEST_FORMS:
                        forms = [next_cell]
                    elif in_patent:
                        m = re.match(r"^([^(（]+)[(（]([^)）]+)[)）]$", nm)
                        if m and all(
                            len(x.strip()) <= 8
                            for x in re.split(r"[、，,;；]", m.group(2)) if x.strip()
                        ):
                            nm = m.group(1)
                            forms = [
                                x.strip()
                                for x in re.split(r"[、，,;；]", m.group(2))
                                if x.strip()
                            ]
                    key = (nm, cat)
                    bucket = drugs.setdefault(key, [])
                    for f in forms:
                        if f not in bucket:
                            bucket.append(f)

    merged = {"west": 1446, "patent": 1335, "nego_w": 399, "nego_p": 61}
    ok = True
    for sec, expected in merged.items():
        nums = seen[sec]
        missing = set(range(1, expected + 1)) - nums
        if missing:
            ok = False
            print(f"WARN {sec}: missing 编号 {sorted(missing)[:15]}"
                  f"{'…' if len(missing) > 15 else ''} ({len(missing)} total)",
                  file=sys.stderr)
        extra = nums - set(range(1, expected + 1))
        if extra:
            ok = False
            print(f"WARN {sec}: unexpected 编号 {sorted(extra)[:10]}", file=sys.stderr)

    out = {
        "source": "国家医保局《国家基本医疗保险、生育保险和工伤保险药品目录(2025年)》",
        "updated": "2025-12",
        "drugs": [
            {"name": n, "category": c, "forms": f} for (n, c), f in drugs.items()
        ],
    }
    cats = OrderedDict()
    for (_n, c) in drugs:
        cats[c] = cats.get(c, 0) + 1
    print(f"rows merged: {len(out['drugs'])} (甲 {cats.get('甲', 0)} / 乙 {cats.get('乙', 0)})")
    OUT.write_text(
        json.dumps(out, ensure_ascii=False, separators=(",", ":")) + "\n",
        encoding="utf-8",
    )
    print(f"wrote {OUT} ({OUT.stat().st_size / 1e6:.2f} MB)")
    if not ok:
        sys.exit(1)


if __name__ == "__main__":
    main()
