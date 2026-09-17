"""WHO ICD-11 MMS 中文术语表（icd11_terms.json，非 CorpusDoc：exact_lookup 用）。

raw: 官方 2025-01 发布包（CDN 直链，HTML 页内探测）：
  - SimpleTabulation-ICD-11-MMS-zh.zip（zh/en 标题 + Code + 章节）
  - mapping.zip（11To10MapToOneCategory：ICD-11 entity → ICD-10）
输出：internal/knowledge/data/icd11_terms.json
  {icd11_code, title_zh, title_en, icd10_map, chapter}
"""

from __future__ import annotations

import csv
import io
import re
import zipfile

from .. import http
from . import base

RELEASE = "2025-01"
BROWSE_PAGE = f"https://icd.who.int/browse/{RELEASE}/mms/zh"
CDN = "https://icdcdn.who.int/static/releasefiles"
FILES = ("SimpleTabulation-ICD-11-MMS-zh.zip", "mapping.zip")


def fetch(ctx):
    page = http.get(BROWSE_PAGE)
    for name in FILES:
        dest = ctx.raw_dir / name
        if dest.exists() and dest.stat().st_size > 0:
            ctx.log(f"raw 已存在，跳过: {name}")
            continue
        m = re.search(rf'href="({re.escape(CDN)}/{RELEASE}/[^"]*{re.escape(name)})"', page)
        url = m.group(1) if m else f"{CDN}/{RELEASE}/{name}"
        http.download(url, dest)


def _tsv_rows(data: bytes):
    text = data.decode("utf-8-sig", "replace")
    reader = csv.reader(io.StringIO(text), delimiter="\t")
    header = next(reader)
    for row in reader:
        if len(row) < len(header):
            row += [""] * (len(header) - len(row))
        yield dict(zip(header, row))


def convert(ctx):
    # 11→10 映射：entity id → 第一个 ICD-10 码
    icd10_by_entity: dict[str, str] = {}
    with zipfile.ZipFile(ctx.raw_dir / "mapping.zip") as z:
        with z.open("11To10MapToOneCategory.txt") as f:
            for row in _tsv_rows(f.read()):
                uri = row.get("Linearization (release) URI") or next(iter(row.values()))
                m = re.search(r"/mms/(\d+)$", uri.strip())
                code10 = (row.get("icd10Code") or "").strip()
                if m and code10:
                    icd10_by_entity[m.group(1)] = code10

    terms = []
    with zipfile.ZipFile(ctx.raw_dir / FILES[0]) as z:
        with z.open(f"SimpleTabulation-ICD-11-MMS-zh.txt") as f:
            for row in _tsv_rows(f.read()):
                code = (row.get("Code") or "").strip()
                kind = (row.get("ClassKind") or "").strip()
                if not code or kind not in ("category",):
                    continue
                title_zh = re.sub(r"^(?:-\s+)+", "", (row.get("Title") or "").strip())
                title_en = re.sub(r"^(?:-\s+)+", "", (row.get("TitleEN") or "").strip())
                entity = ""
                m = re.search(r"/mms/(\d+)$", (row.get("Linearization URI") or "").strip())
                if m:
                    entity = m.group(1)
                terms.append({
                    "icd11_code": code,
                    "title_zh": title_zh,
                    "title_en": title_en,
                    "icd10_map": icd10_by_entity.get(entity, ""),
                    "chapter": (row.get("ChapterNo") or "").strip(),
                })
    ctx.log(f"{len(terms)} 编码条目（icd10 映射 {sum(1 for t in terms if t['icd10_map'])} 条）")
    out = base.DATA_DIR / "icd11_terms.json"
    base.write_json(out, {
        "source": "icd11", "updated": base.today(), "release": RELEASE, "terms": terms,
    })
    ctx.log(f"写出 {out} ({out.stat().st_size >> 10}KiB)")
