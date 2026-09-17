"""python3 -m medkb fetch|convert|validate|stats [source|all]

source ∈ all_sources()（plugins/ 下模块名）。fetch 幂等；convert 产出
internal/knowledge/data/corpus_<source>.json（icd11 → icd11_terms.json）。
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

from . import schema
from .plugins import base, load, all_sources


def do_fetch(source: str, args):
    ctx = base.raw_ctx(source, vars(args))
    ctx.raw_dir.mkdir(parents=True, exist_ok=True)
    load(source).fetch(ctx)


def do_convert(source: str, args):
    ctx = base.raw_ctx(source, vars(args))
    load(source).convert(ctx)


def do_validate(source: str, args) -> bool:
    if source == "icd11":
        p = base.DATA_DIR / "icd11_terms.json"
        errs = schema.validate_icd11(p) if p.exists() else [f"{p}: 不存在"]
    else:
        p = base.out_path(source)
        errs = schema.validate_docs(p) if p.exists() else [f"{p}: 不存在"]
    status = "OK" if not errs else f"{len(errs)} ERRORS"
    print(f"[validate] {source}: {p.name} {status} ({p.stat().st_size >> 10}KiB)"
          if p.exists() else f"[validate] {source}: {p.name} 缺失")
    for e in errs[:20]:
        print(f"  - {e}")
    return not errs


def do_stats(source: str, args):
    p = base.DATA_DIR / "icd11_terms.json" if source == "icd11" else base.out_path(source)
    if not p.exists():
        print(f"[stats] {source}: 无数据文件")
        return
    import json
    data = json.loads(p.read_text())
    if source == "icd11":
        terms = data.get("terms", [])
        zh = sum(1 for t in terms if t.get("title_zh"))
        print(f"[stats] icd11: {len(terms)} 术语, {zh} 有中文")
        return
    entries = data.get("entries", [])
    kinds: dict = {}
    zh = 0
    body_bytes = 0
    for e in entries:
        kinds[e.get("kind", "")] = kinds.get(e.get("kind", ""), 0) + 1
        zh += 1 if e.get("title_zh") else 0
        body_bytes += len(e.get("body", "").encode())
    print(f"[stats] {source}: {len(entries)} 条, kind={kinds}, "
          f"title_zh={zh}, body 总量 {body_bytes >> 20}MiB")


def main(argv=None):
    ap = argparse.ArgumentParser(prog="medkb", description=__doc__)
    sub = ap.add_subparsers(dest="cmd", required=True)
    for name in ("fetch", "convert", "validate", "stats"):
        p = sub.add_parser(name)
        p.add_argument("source", help=f"{', '.join(all_sources())} 或 all")
        p.add_argument("--max-chapters", type=int, default=0,
                       help="限抓章节数（试跑用，0=全量）")
        p.add_argument("--max-body-bytes", type=int, default=schema.MAX_BODY_BYTES)
    args = ap.parse_args(argv)

    sources = all_sources() if args.source == "all" else [args.source]
    ok = True
    for s in sources:
        try:
            if args.cmd == "fetch":
                do_fetch(s, args)
            elif args.cmd == "convert":
                do_convert(s, args)
            elif args.cmd == "validate":
                ok = do_validate(s, args) and ok
            elif args.cmd == "stats":
                do_stats(s, args)
        except Exception as e:  # noqa: BLE001
            print(f"[{s}] FAILED: {e}", file=sys.stderr)
            ok = False
    sys.exit(0 if ok else 1)


if __name__ == "__main__":
    main()
