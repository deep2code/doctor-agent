#!/usr/bin/env python3
"""Compress the embedded knowledge JSON files into internal/knowledge/gz/.

The Go binary embeds the gzip-compressed copies (go:embed gz/*.gz) to cut the
binary size ~68%; the loader decompresses them at startup.

Usage:
  python3 external/make_gz.py [--level 9]

Idempotent: regenerates every .json.gz from internal/knowledge/data/*.json.
Run after editing any data JSON (or `make gz`).
"""
import argparse
import gzip
import os
from pathlib import Path

ROOT = Path(__file__).parent.parent
SRC_DIR = ROOT / "internal" / "knowledge" / "data"
OUT_DIR = ROOT / "internal" / "knowledge" / "gz"


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--level", type=int, default=9, help="gzip level (0-9)")
    args = parser.parse_args()

    OUT_DIR.mkdir(parents=True, exist_ok=True)
    src_bytes = out_bytes = 0
    count = 0
    for src in sorted(SRC_DIR.glob("*.json")):
        data = src.read_bytes()
        dst = OUT_DIR / (src.name + ".gz")
        with gzip.open(dst, "wb", compresslevel=args.level) as f:
            f.write(data)
        src_bytes += len(data)
        out_bytes += dst.stat().st_size
        count += 1
        print(f"{src.name:35s} {len(data)/1e6:8.2f}MB -> {dst.stat().st_size/1e6:6.2f}MB")

    # Remove stale .gz files whose source no longer exists.
    known = {f.name + ".gz" for f in SRC_DIR.glob("*.json")}
    for old in OUT_DIR.glob("*.gz"):
        if old.name not in known:
            old.unlink()
            print(f"removed stale {old.name}")

    print(f"─── {count} files: {src_bytes/1e6:.1f}MB -> {out_bytes/1e6:.1f}MB "
          f"(saved {(1 - out_bytes/src_bytes) * 100:.0f}%)")


if __name__ == "__main__":
    main()
