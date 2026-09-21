#!/usr/bin/env python3
"""Compress the embedded knowledge JSON files into internal/knowledge/gz/.

Uses Zstandard (zstd) level 19 — measured ~38% smaller than gzip-9 on the
medical QA corpora (46MB -> 28.6MB) at equal decompression speed.

Output extension is `.zst` (distinct from legacy `.gz`). The Go loaders
(seed.go decompressFile, bake.go) auto-detect by magic bytes so both old
gzip and new zstd files are readable.

Usage:
  python3 external/make_gz.py [--level 19]
  python3 external/make_gz.py --check   # CI gate: gz/ in sync with data/? (read-only)

Idempotent: regenerates every .json.zst from internal/knowledge/data/*.json.
Sources that exceed git's 100 MiB per-file limit are committed as parts
(X.json.partNNN + X.json.parts, see external/split_data.py); this script
reassembles those in memory, so a checkout without the whole JSON still builds
gz/. Run after editing any data JSON.
"""
import argparse
import io
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


def find_artifact(name: str) -> Path | None:
    """The committed compressed copy of one dataset (`<name>.zst`), if any."""
    p = OUT_DIR / (name + ".zst")
    return p if p.is_file() else None


def decompress_artifact(path: Path) -> bytes:
    data = path.read_bytes()
    if data[:2] == b"\x1f\x8b":  # legacy gzip frame
        import gzip

        return gzip.decompress(data)
    if zstd is not None:
        reader = zstd.ZstdDecompressor().stream_reader(io.BytesIO(data))
        with reader:
            return reader.read()
    import subprocess

    return subprocess.run(
        ["zstd", "-d", "-q", "-c", str(path)], check=True, capture_output=True
    ).stdout


def check_all() -> int:
    """--check: does every gz/ artifact still hold its source bytes?

    Compares decompressed payloads rather than the compressed bytes, because a
    zstd frame is only byte-stable for one library version — pinning the check
    to bytes would tie CI to `zstandard==0.23.0` forever and turn any version
    bump into a false "stale gz" failure.
    """
    known = source_names()
    drift, missing = [], []
    for name in sorted(known):
        try:
            data = load_source(name)
        except ValueError as exc:
            print(f"ERROR: {exc}", file=sys.stderr)
            return 1
        art = find_artifact(name)
        if data is None:
            # No usable source in this checkout: the artifact is the only copy,
            # so there is nothing to compare against — just require it to exist.
            if art is None and name not in NO_SOURCE_IN_GIT:
                missing.append(name)
            continue
        if art is None or decompress_artifact(art) != data:
            drift.append(f"{name} ({'no artifact' if art is None else 'content differs'})")
    for old in OUT_DIR.glob("*.zst"):
        if old.stem not in known:
            drift.append(f"{old.name} (orphan: no source)")
    if drift or missing:
        for d in drift + [f"{m} (never compressed)" for m in missing]:
            print(f"  ❌ {d}", file=sys.stderr)
        print(
            f"─── gz/ is out of sync with data/ ({len(drift) + len(missing)} files); "
            "run 'python3 external/make_gz.py' and commit",
            file=sys.stderr,
        )
        return 1
    print(f"─── gz/ matches data/ ({len(known)} datasets)")
    return 0


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--level", type=int, default=19, help="zstd level (1-22)")
    parser.add_argument(
        "--check",
        action="store_true",
        help="verify gz/ mirrors data/ without writing anything (exit 1 on drift)",
    )
    args = parser.parse_args()

    OUT_DIR.mkdir(parents=True, exist_ok=True)
    if args.check:
        return check_all()
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
    return 0


if __name__ == "__main__":
    sys.exit(main())
