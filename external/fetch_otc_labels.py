#!/usr/bin/env python3
"""广东省药监局镜像的 NMPA「处方药转换为非处方药」公告 → 说明书范本 .doc
→ textutil/pypdf 转文本 → internal/knowledge/data/corpus_otc_labels.json

NMPA 总站对本机 IP 有 WAF (412/空壳), 广东省局 mpa.gd.gov.cn 静态转发
NMPA 全部公告且附件 .doc 直链可下, 附件2 = 非处方药说明书范本.

用法: python3 fetch_otc_labels.py [--pages 60]
幂等: 已下载的公告附件跳过.
"""
import argparse
import json
import re
import subprocess
import time
import urllib.request
from pathlib import Path

BASE = Path(__file__).parent
RAW = BASE / "otc_labels" / "raw"
TXT = BASE / "otc_labels" / "txt"
LIST_BASE = "https://mpa.gd.gov.cn/xwdt/tzgg/"
UA = {"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/124.0"}

TITLE_RE = re.compile(r'href="([^"]*content/post_\d+\.html)"[^>]*title="([^"]*)"')
OTC_RE = re.compile(r"转换为非处方药")
ANN_RE = re.compile(r"（(\d{4})年第(\d+)号）")


def get(url: str, binary: bool = False, tries: int = 3):
    req = urllib.request.Request(url, headers=UA)
    for i in range(tries):
        try:
            with urllib.request.urlopen(req, timeout=60) as r:
                data = r.read()
            return data if binary else data.decode("utf-8", "replace")
        except Exception as e:
            if i == tries - 1:
                print(f"  ⚠️ {url[-60:]}: {e}", file=sys.stderr)
                return None
            time.sleep(2)


import sys  # noqa: E402


def doc_to_text(path: Path) -> str:
    """WPS .doc/.docx → textutil; .pdf → pypdf."""
    if path.suffix.lower() == ".pdf":
        import io
        from pypdf import PdfReader
        try:
            r = PdfReader(str(path))
            return "\n".join((p.extract_text() or "") for p in r.pages).strip()
        except Exception:
            return ""
    try:
        out = subprocess.run(
            ["textutil", "-convert", "txt", "-stdout", str(path)],
            capture_output=True, timeout=60)
        return out.stdout.decode("utf-8", "replace").strip()
    except Exception:
        return ""


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--pages", type=int, default=60)
    args = ap.parse_args()

    RAW.mkdir(parents=True, exist_ok=True)
    TXT.mkdir(parents=True, exist_ok=True)

    # 1) 枚举列表页 (首页 + index_2..N; 该站无 index_1, http 会 404)
    posts = {}  # url -> title
    misses = 0
    for page in range(0, args.pages + 1):
        if page == 0:
            url = LIST_BASE
        elif page == 1:
            continue
        else:
            url = f"{LIST_BASE}index_{page}.html"
        html = get(url)
        if not html:
            misses += 1
            if misses >= 3:
                break
            continue
        found = 0
        for u, t in TITLE_RE.findall(html):
            t = re.sub(r"<br/?>", "", t).strip()
            if OTC_RE.search(t) and "非处方药转换为处方药" not in t:
                if u not in posts:
                    posts[u] = t
                    found += 1
        print(f"列表页 {page}: +{found} (累计 {len(posts)})", file=sys.stderr)
        if found == 0:
            misses += 1
            if misses >= 3:
                break
        else:
            misses = 0
        time.sleep(0.4)

    print(f"共 {len(posts)} 个转换公告", file=sys.stderr)

    # 2) 逐公告下载附件
    ok = skip = 0
    docs = []
    for n, (u, title) in enumerate(sorted(posts.items()), 1):
        m = re.search(r"post_(\d+)\.html", u)
        pid = m.group(1)
        out_txt = TXT / f"{pid}.txt"
        if out_txt.exists():
            skip += 1
        else:
            html = get(u)
            if not html:
                continue
            atts = re.findall(r'href="([^"]*attachment[^"]*\.(?:docx?|pdf))"', html, flags=re.I)
            if not atts:
                atts = re.findall(r'href="([^"]*\.(?:docx?|pdf))"', html, flags=re.I)
            texts = []
            for a in atts:
                # 两附件常同名(公告id.doc), 用完整路径段做文件名防覆盖
                fname = re.sub(r"[^\w.\-]", "_", a.split("attachment/")[-1]).replace("/", "_")[-80:]
                raw_path = RAW / f"{pid}_{fname}"
                if not raw_path.exists():
                    raw = get(a, binary=True)
                    if not raw:
                        continue
                    raw_path.write_bytes(raw)
                    time.sleep(0.5)
                t = doc_to_text(raw_path)
                if t:
                    texts.append(t)
            if not texts:
                print(f"  [{n}] ⚠️ 无附件文本: {title[:40]}", file=sys.stderr)
                continue
            out_txt.write_text("\n\n".join(texts), encoding="utf-8")
            ok += 1
        # 3) 组装 corpus doc
        text = out_txt.read_text(encoding="utf-8")
        yr = ANN_RE.search(title)
        docs.append({
            "source": "otc_labels",
            "id": f"otc-{pid}",
            "lang": "zh",
            "title": title,
            "kind": "drug_label",
            "keywords": ["说明书", "非处方药", "OTC", "用药"],
            "year": yr.group(1) if yr else "",
            "url": u,
            "body": text[:24000],
        })
        if n % 10 == 0:
            print(f"  … {n}/{len(posts)}", file=sys.stderr)

    out = BASE.parent / "internal" / "knowledge" / "data" / "corpus_otc_labels.json"
    out.write_text(json.dumps({
        "source": "otc_labels",
        "updated": "2026-09",
        "entries": docs,
    }, ensure_ascii=False), encoding="utf-8")
    print(f"新转 {ok}, 已有 {skip}; wrote {out} ({len(docs)} docs)", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
