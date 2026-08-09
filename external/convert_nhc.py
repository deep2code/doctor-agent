#!/usr/bin/env python3
"""Convert the downloaded NHC (国家卫健委) 诊疗方案/指南 JSON corpus into
internal/knowledge/data/nhc_guides.json — a full-text search layer, mirroring
the MSD/MedlinePlus integration.

Input:
  external/nhc/guides/*.json       (30 文字版, {title,url,year,content})
  external/nhc/guides_ocr/*.json   (9 OCR 版,  {title,url,year,content}; url 常为空)

Output:
  internal/knowledge/data/nhc_guides.json
  {"source":"nhc","updated":"2026-08","entries":[{title,url,year,content,source}]}

Titles are cleaned (关于印发 … 的通知 removed) so that full-query title
matching in the retriever hits the disease name directly.
"""
import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).parent.parent
SRC_DIRS = [
    (ROOT / "external" / "nhc" / "guides", "nhc"),
    (ROOT / "external" / "nhc" / "guides_ocr", "nhc_ocr"),
]
OUT = ROOT / "internal" / "knowledge" / "data" / "nhc_guides.json"


def clean_title(t: str) -> str:
    """Strip 关于印发/关于将…纳入/国家卫生健康委办公厅关于印发 prefixes and
    the trailing 的通知/通知, normalize runs of whitespace/`-` to a single
    space, then drop a leading/trailing -."""
    t = t.strip()
    t = re.sub(r"^(国家卫生健康委办公厅)?关于(印发|将)", "", t)
    t = re.sub(r"^(关于印发|印发)", "", t)
    t = re.sub(r"的通知$", "", t)
    t = re.sub(r"通知$", "", t)
    t = re.sub(r"[-\s]+", " ", t).strip(" -")
    return t


def extract_year(text: str, filename: str) -> str:
    m = re.search(r"(20\d{2})", text + filename)
    return m.group(1) if m else ""


def fallback_title(content: str) -> str:
    """Recover a truncated title (e.g. the fetched filename was cut at
    '…和突发事') from the first substantive line of the body."""
    for l in content.splitlines():
        l = l.strip()
        if not l or re.fullmatch(r"附件\d+", l):
            continue
        if "版" in l or l.endswith(("方案", "规范", "指南", "流程")):
            return l
    return ""


def main() -> int:
    entries = []
    seen = set()
    for src_dir, source in SRC_DIRS:
        for f in sorted(src_dir.glob("*.json")):
            try:
                d = json.loads(f.read_text(encoding="utf-8"))
            except Exception as e:
                print(f"skip {f.name}: {e}", file=sys.stderr)
                continue
            content = (d.get("content") or "").strip()
            if not content:
                print(f"skip {f.name}: empty content", file=sys.stderr)
                continue
            title = clean_title(d.get("title") or f.stem)
            if title.endswith(("和突发事", "-", "和")) or len(title) < 6:
                fb = fallback_title(content)
                if fb:
                    print(f"fix truncated title: {title!r} -> {fb!r}", file=sys.stderr)
                    title = fb
            if title in seen:
                continue
            seen.add(title)
            entries.append({
                "title": title,
                "url": (d.get("url") or "").strip(),
                "year": d.get("year") or extract_year(d.get("url", ""), f.name),
                "content": content,
                "source": source,
            })
    entries.sort(key=lambda e: e["title"])
    out = {
        "source": "nhc",
        "updated": "2026-08",
        "entries": entries,
    }
    OUT.write_text(json.dumps(out, ensure_ascii=False, indent=1), encoding="utf-8")
    total = sum(len(e["content"]) for e in entries)
    print(f"wrote {OUT} ({len(entries)} guides, {total/1024:.0f} KB text)")
    for e in entries:
        if not e["url"]:
            print(f"  (no url) {e['title']}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
