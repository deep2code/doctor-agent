#!/usr/bin/env python3
"""Compress the embedded knowledge JSON files into internal/knowledge/gz/.

Uses Zstandard (zstd) level 19 — measured ~38% smaller than gzip-9 on the
medical QA corpora (46MB -> 28.6MB) at equal decompression speed.

Output extension is `.zst` (distinct from legacy `.gz`). The Go loaders
(seed.go decompressFile, bake.go) auto-detect by magic bytes so both old
gzip and new zstd files are readable.

Usage:
  python3 external/make_gz.py [--level 19]

Idempotent: regenerates every .json.zst from internal/knowledge/data/*.json.
Run after editing any data JSON.
"""
import argparse
import os
import sys
from pathlib import Path

try:
    import zstandard as zstd
except ImportError:  # pragma: no cover
    zstd = None

ROOT = Path(__file__).parent.parent
SRC_DIR = ROOT / "internal" / "knowledge" / "data"
OUT_DIR = ROOT / "internal" / "knowledge" / "gz"


def compress_zstd(data: bytes, level: int) -> bytes:
    if zstd is not None:
        cctx = zstd.ZstdCompressor(level=level)
        return cctx.compress(data)
    # Fallback: system zstd CLI
    import subprocess
    import tempfile

    with tempfile.NamedTemporaryFile(suffix=".zst", delete=False) as tf:
        tf.write(data)
        tmp_in = tf.name
    try:
        subprocess.run(
            ["zstd", f"-{level}", "-q", "-f", tmp_in, "-o", tmp_in + ".out"],
            check=True,
            capture_output=True,
        )
        with open(tmp_in + ".out", "rb") as f:
            return f.read()
    finally:
        for p in (tmp_in, tmp_in + ".out"):
            try:
                os.unlink(p)
            except OSError:
                pass


def is_lfs_pointer(data: bytes) -> bool:
    """True if the file content is a Git-LFS pointer (real data not pulled)."""
    return data.startswith(b"version https://git-lfs.github.com/spec/v1")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--level", type=int, default=19, help="zstd level (1-22)")
    args = parser.parse_args()

    OUT_DIR.mkdir(parents=True, exist_ok=True)
    src_bytes = out_bytes = 0
    count = 0
    skipped_lfs = 0
    for src in sorted(SRC_DIR.glob("*.json")):
        data = src.read_bytes()
        # LFS pointer files (real data not pulled locally) must NOT be
        # compressed — otherwise the pointer text (134B) replaces the real
        # dataset in gz/. Keep any previously built artifact for them.
        if is_lfs_pointer(data):
            skipped_lfs += 1
            print(f"{src.name:35s} SKIP (Git-LFS pointer, run `git lfs pull`)")
            continue
        dst = OUT_DIR / (src.name + ".zst")
        dst.write_bytes(compress_zstd(data, args.level))
        src_bytes += len(data)
        out_bytes += dst.stat().st_size
        count += 1
        print(f"{src.name:35s} {len(data)/1e6:8.2f}MB -> {dst.stat().st_size/1e6:6.2f}MB")

    # Remove stale artifacts whose source no longer exists (both extensions).
    known = {f.name for f in SRC_DIR.glob("*.json")}
    for old in list(OUT_DIR.glob("*.zst")) + list(OUT_DIR.glob("*.gz")):
        if old.stem not in known:
            old.unlink()
            print(f"removed stale {old.name}")

    summary = f"{count} files compressed"
    if skipped_lfs:
        summary += f", {skipped_lfs} skipped (LFS)"
    print(f"─── {summary}: {src_bytes/1e6:.1f}MB -> {out_bytes/1e6:.1f}MB "
          f"(saved {(1 - out_bytes/src_bytes) * 100:.0f}%)")


if __name__ == "__main__":
    main()
