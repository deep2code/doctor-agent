#!/usr/bin/env python3
"""Compress the knowledge seed JSON files into internal/knowledge/gz/.

Layout — one directory per dataset, mirrored exactly in both trees, which is
the only thing that decides what a file seeds into (the Go reader takes the
dataset from the directory it sits in; there is no filename registry to update):
    internal/knowledge/data/<dataset>/<name>.json   ->  gz/<dataset>/<name>.json.zst

Uses Zstandard (zstd) level 19 — measured ~38% smaller than gzip-9 on the
medical QA corpora (46MB -> 28.6MB) at equal decompression speed.

Output extension is `.zst` (distinct from legacy `.gz`). The Go loaders
(seed.go decompressFile, bake.go) auto-detect by magic bytes so both old
gzip and new zstd files are readable.

Usage:
  python3 external/make_gz.py [--level 19] [--jobs N] [--force]
  python3 external/make_gz.py --check   # CI gate: gz/ in sync with data/? (read-only)

Idempotent: regenerates every .json.zst from the data/ tree, and removes any
artifact whose source directory or file is gone.
Incremental by default: a local state file (`.cache/make_gz_state.json`,
gitignored) remembers (source sha256, artifact sha256) per dataset, so an
unchanged source is never re-compressed at level 19 — the ~7 min cold run
becomes seconds when only one data file changed. A checkout without the state
file is still handled cheaply: an artifact that decompresses to its source is
adopted into the state instead of being rebuilt. `--force` rebuilds everything;
changing `--level` invalidates the state (that is the point of the flag).

Sources that exceed git's 100 MiB per-file limit are committed as parts
(<name>.json.partNNN + <name>.json.parts next to them, see external/split_data.py);
this script reassembles those in memory, so a checkout without the whole JSON
still builds gz/. Run after editing any data JSON.
"""
import argparse
import hashlib
import io
import json
import os
import sys
import time
from concurrent.futures import ThreadPoolExecutor
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
# sweep below must never treat it as removable. Paths are relative to data/, so
# these entries carry their dataset directory.
NO_SOURCE_IN_GIT = {"corpus/corpus_statpearls.json"}


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
    """Every dataset name make_gz can see, as a `<dataset>/<name>.json` path:
    whole sources, split sources, and the out-of-git exceptions."""
    names = {rel(f) for f in SRC_DIR.glob("*/*.json")}
    names |= {rel(f)[: -len(".parts")] for f in SRC_DIR.glob("*/*.json.parts")}
    return names | NO_SOURCE_IN_GIT


def rel(path: Path) -> str:
    """A source path relative to data/ — the dataset directory plus the file."""
    return path.relative_to(SRC_DIR).as_posix()


def flat_strays() -> list[str]:
    """Sources sitting directly in data/ instead of data/<dataset>/.

    The dataset is the directory name and nothing else, so a flat file has no
    dataset at all: silently skipping it would mean a generator that still writes
    the old path produces a gz/ that does not contain the edit. Failing the build
    is the point (the same rule `go test ./internal/knowledge` enforces).
    """
    strays = [p.name for p in SRC_DIR.glob("*.json")]
    strays += [rel(p) for p in SRC_DIR.glob("*.json.parts")]
    return sorted(strays)


def report_strays() -> int:
    strays = flat_strays()
    if not strays:
        return 0
    for name in strays:
        print(f"  ❌ data/{name} is outside a <dataset>/ directory", file=sys.stderr)
    print(
        f"─── {len(strays)} source(s) sit directly in data/: move each one to "
        "data/<dataset>/<name>.json (the directory name IS the dataset) and re-run",
        file=sys.stderr,
    )
    return 1


def load_source(name: str) -> bytes | None:
    """Return the dataset bytes, or None when unavailable (LFS pointer, or a
    split whose parts are incomplete). Raises on corrupt parts."""
    path = SRC_DIR / name
    if path.is_file():
        data = path.read_bytes()
        if is_lfs_pointer(data):
            print(f"{name:40s} SKIP (Git-LFS pointer, run `git lfs pull`)")
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
                print(f"{name:40s} WARN (parts disagree with {name}; re-run split_data.py split)")
        return data
    if split_data.manifest_path(name).is_file():
        data = split_data.read_merged(name)
        print(f"{name:40s} {len(data)/1e6:8.2f}MB (reassembled from parts)")
        return data
    print(f"{name:40s} SKIP (no source in repo; gz/ artifact is the only copy)")
    return None


def artifact_path(name: str) -> Path:
    """`<dataset>/<name>.json` -> `gz/<dataset>/<name>.json.zst`."""
    return OUT_DIR / (name + ".zst")


def artifact_source(path: Path) -> str:
    """The source name an artifact was made from (`ds/x.json.zst` -> `ds/x.json`)."""
    rel_path = path.relative_to(OUT_DIR).as_posix()
    for ext in (".zst", ".gz"):  # ".gz" is the pre-zstd frame still readable by Go
        if rel_path.endswith(ext):
            return rel_path[: -len(ext)]
    return rel_path


def find_artifact(name: str) -> Path | None:
    """The committed compressed copy of one dataset, if any."""
    p = artifact_path(name)
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


# ---------------------------------------------------------------- incremental

STATE_PATH = ROOT / ".cache" / "make_gz_state.json"


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def load_state() -> dict:
    """The local (gitignored) skip-cache. Any problem reading it means 'rebuild
    everything', never 'fail the build' — it is an accelerator, not state."""
    try:
        st = json.loads(STATE_PATH.read_text(encoding="utf-8"))
    except (OSError, ValueError):
        return {}
    return st if isinstance(st, dict) else {}


def save_state(state: dict) -> None:
    try:
        STATE_PATH.parent.mkdir(parents=True, exist_ok=True)
        STATE_PATH.write_text(
            json.dumps(state, indent=1, sort_keys=True), encoding="utf-8"
        )
    except OSError as exc:  # a missing cache only costs the next run some time
        print(f"WARN state file not written: {exc}", file=sys.stderr)


def classify(
    name: str, data: bytes, level: int, force: bool, state: dict
) -> tuple[str, str, str]:
    """'cached' (identical to the last run), 'adopt' (the artifact on disk
    already holds these bytes), or 'build' — plus the (source, artifact) hashes
    so the caller can record what it just verified.

    The adopt path is what keeps a fresh checkout cheap: `--check` proves the
    same equivalence by decompressing, and decompression is ~2 orders of
    magnitude faster than level-19 compression, so verifying beats rebuilding.
    """
    art = artifact_path(name)
    if force or not art.is_file():
        return "build", "", ""
    src_fp, art_fp = sha256(data), sha256(art.read_bytes())
    prev = (state.get("files") or {}).get(name)
    if (
        state.get("level") == level
        and isinstance(prev, dict)
        and prev.get("src") == src_fp
        and prev.get("art") == art_fp
    ):
        return "cached", src_fp, art_fp
    if decompress_artifact(art) == data:
        return "adopt", src_fp, art_fp
    return "build", src_fp, art_fp


def build_one(name: str, level: int):
    """Compress one dataset. None when the source is unusable, else
    (name, source bytes, artifact bytes, source sha256, artifact sha256)."""
    data = load_source(name)
    if data is None:
        return None
    payload = compress_zstd(data, level)
    art = artifact_path(name)
    art.parent.mkdir(parents=True, exist_ok=True)
    art.write_bytes(payload)
    return name, len(data), len(payload), sha256(data), sha256(payload)


def check_all() -> int:
    """--check: does every gz/ artifact still hold its source bytes, and does
    the gz/ tree mirror the data/ tree's `<dataset>/<name>.json` layout?

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
    for old in OUT_DIR.rglob("*"):
        if old.is_file() and old.suffix in (".zst", ".gz") and artifact_source(old) not in known:
            drift.append(f"{old.relative_to(OUT_DIR).as_posix()} (orphan: no source)")
    if drift or missing:
        for d in drift + [f"{m} (never compressed)" for m in missing]:
            print(f"  ❌ {d}", file=sys.stderr)
        print(
            f"─── gz/ is out of sync with data/ ({len(drift) + len(missing)} files); "
            "run 'python3 external/make_gz.py' and commit",
            file=sys.stderr,
        )
        return 1
    datasets = {name.split("/", 1)[0] for name in known}
    print(f"─── gz/ matches data/ ({len(known)} sources across {len(datasets)} datasets)")
    return 0


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--level", type=int, default=19, help="zstd level (1-22)")
    parser.add_argument(
        "--check",
        action="store_true",
        help="verify gz/ mirrors data/ without writing anything (exit 1 on drift)",
    )
    parser.add_argument(
        "--force",
        action="store_true",
        help="ignore the skip-cache and rebuild every dataset",
    )
    parser.add_argument(
        "--jobs",
        type=int,
        default=1,
        help="parallel compressions among the datasets that really need rebuilding",
    )
    args = parser.parse_args()

    OUT_DIR.mkdir(parents=True, exist_ok=True)
    if report_strays():
        return 1
    if args.check:
        return check_all()

    started = time.monotonic()
    state = load_state()
    files: dict[str, dict] = {}
    pending: list[str] = []
    src_bytes = out_bytes = 0
    cached = adopted = no_source = 0
    for name in sorted(source_names()):
        try:
            data = load_source(name)
        except ValueError as exc:
            print(f"ERROR: {exc}", file=sys.stderr)
            return 1
        if data is None:
            no_source += 1  # LFS pointer / out-of-git source: artifact stays as-is
            continue
        action, src_fp, art_fp = classify(name, data, args.level, args.force, state)
        if action == "build":
            pending.append(name)
            continue
        files[name] = {"src": src_fp, "art": art_fp}
        src_bytes += len(data)
        out_bytes += artifact_path(name).stat().st_size
        cached += action == "cached"
        adopted += action == "adopt"
        if action == "adopt":
            print(f"{name:40s} {len(data)/1e6:8.2f}MB (gz already current)")

    def record(res) -> None:
        nonlocal src_bytes, out_bytes
        if not res:
            return
        name, size, packed, src_fp, art_fp = res
        files[name] = {"src": src_fp, "art": art_fp}
        src_bytes += size
        out_bytes += packed
        print(f"{name:40s} {size/1e6:8.2f}MB -> {packed/1e6:6.2f}MB")

    if args.jobs > 1 and len(pending) > 1:
        with ThreadPoolExecutor(max_workers=args.jobs) as pool:
            for res in pool.map(lambda n: build_one(n, args.level), pending):
                record(res)
    else:
        for name in pending:
            record(build_one(name, args.level))

    # Remove stale artifacts whose source no longer exists (both extensions,
    # at any depth), then the directories they leave behind.
    known = source_names()
    for old in sorted(OUT_DIR.rglob("*")):
        if old.is_file() and old.suffix in (".zst", ".gz") and artifact_source(old) not in known:
            old.unlink()
            print(f"removed stale {old.relative_to(OUT_DIR).as_posix()}")
    for d in sorted((p for p in OUT_DIR.rglob("*") if p.is_dir()), reverse=True):
        try:
            d.rmdir()  # only ever removes a directory we just emptied
        except OSError:
            pass

    save_state({"level": args.level, "files": files})
    parts = []
    if cached:
        parts.append(f"{cached} unchanged")
    if adopted:
        parts.append(f"{adopted} verified against existing gz")
    if len(files) - cached - adopted:
        parts.append(f"{len(files) - cached - adopted} compressed")
    if no_source:
        parts.append(f"{no_source} skipped (no usable source)")
    saved = (1 - out_bytes / src_bytes) * 100 if src_bytes else 0.0
    print(
        f"─── {len(files)} datasets: {src_bytes/1e6:.1f}MB -> {out_bytes/1e6:.1f}MB "
        f"(saved {saved:.0f}%), " + ", ".join(parts)
        + f" in {time.monotonic()-started:.0f}s"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
