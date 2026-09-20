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
Sources that exceed git's 100 MiB per-file limit are committed as parts
(X.json.partNNN + X.json.parts, see external/split_data.py); this script
reassembles those in memory, so a checkout without the whole JSON still builds
gz/. Run after editing any data JSON.
"""
import argparse
import os
import sys
from pathlib import Path

try:
    import zstandard as zstd
except ImportError:  # pragma: no cover
    zstd = None

sys.path.insert(0, str(Path(__file__).resolve().parent))
import split_data  # noqa: E402  (external/split_data.py)

ROOT = Path(__file__).parent.parent
SRC_DIR = ROOT / "internal" / "knowledge" / "data"
OUT_DIR = ROOT / "internal" / "knowledge" / "gz"

# Datasets whose source JSON deliberately lives outside git (each is far over
# the 100 MiB per-file limit and its upstream is re-downloadable). The
# committed gz/ artifact is then the ONLY repository copy, so the stale-source
# sweep below must never treat it as removable.
NO_SOURCE_IN_GIT = {"corpus_statpearls.json"}


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


def source_names() -> set[str]:
    """Every dataset name make_gz can see: whole sources, split sources, and
    the out-of-git exceptions."""
    names = {f.name for f in SRC_DIR.glob("*.json")}
    names |= {f.name[: -len(".parts")] for f in SRC_DIR.glob("*.json.parts")}
    return names | NO_SOURCE_IN_GIT


def load_source(name: str) -> bytes | None:
    """Return the dataset bytes, or None when unavailable (LFS pointer, or a
    split whose parts are incomplete). Raises on corrupt parts."""
    path = SRC_DIR / name
    if path.is_file():
        data = path.read_bytes()
        if is_lfs_pointer(data):
            print(f"{name:35s} SKIP (Git-LFS pointer, run `git lfs pull`)")
            return None
        if split_data.manifest_path(name).is_file():
            # A whole file and a manifest coexist: warn unless they agree, so a
            # stale split can't silently win on another machine. The whole file
            # still wins here — a broken split must not block a good build.
            try:
                agrees = split_data.read_merged(name) == data
            except ValueError:
                agrees = False
            if not agrees:
                print(f"{name:35s} WARN (parts disagree with {name}; re-run split_data.py split)")
        return data
    if split_data.manifest_path(name).is_file():
        data = split_data.read_merged(name)
        print(f"{name:35s} {len(data)/1e6:8.2f}MB (reassembled from parts)")
        return data
    print(f"{name:35s} SKIP (no source in repo; gz/ artifact is the only copy)")
    return None


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--level", type=int, default=19, help="zstd level (1-22)")
    args = parser.parse_args()

    OUT_DIR.mkdir(parents=True, exist_ok=True)
    src_bytes = out_bytes = 0
    count = 0
    skipped = 0
    for name in sorted(source_names()):
        try:
            data = load_source(name)
        except ValueError as exc:
            print(f"ERROR: {exc}", file=sys.stderr)
            return 1
        if data is None:
            skipped += 1
            continue
        dst = OUT_DIR / (name + ".zst")
        dst.write_bytes(compress_zstd(data, args.level))
        src_bytes += len(data)
        out_bytes += dst.stat().st_size
        count += 1
        print(f"{name:35s} {len(data)/1e6:8.2f}MB -> {dst.stat().st_size/1e6:6.2f}MB")

    # Remove stale artifacts whose source no longer exists (both extensions).
    known = source_names()
    for old in list(OUT_DIR.glob("*.zst")) + list(OUT_DIR.glob("*.gz")):
        if old.stem not in known:
            old.unlink()
            print(f"removed stale {old.name}")

    summary = f"{count} files compressed"
    if skipped:
        summary += f", {skipped} skipped (no usable source)"
    print(f"─── {summary}: {src_bytes/1e6:.1f}MB -> {out_bytes/1e6:.1f}MB "
          f"(saved {(1 - out_bytes/src_bytes) * 100:.0f}%)")


if __name__ == "__main__":
    main()
