#!/usr/bin/env python3
"""Split oversized knowledge seeds into git-safe parts (GitHub hard-limits a
single file at 100 MiB) and reassemble them losslessly.

Why this exists: a few datasets in internal/knowledge/data are far larger than
the limit (medical_qa_pairs.json ~327 MiB, huatuo_qa.json ~141 MiB). Their
gz/*.zst artifacts are ~30 MiB and commit fine, but the source JSONs cannot be
committed whole — and some of them cannot be regenerated from anything else in
the repo, so "just gitignore it" would silently make the seed unbuildable.

Layout, for source "X.json":
    X.json.part000 ... X.json.partNNN   byte slices, concat == original
    X.json.parts                        manifest (per-part + whole-file sha256)

Parts are split on raw byte offsets, so pretty-printed and single-line JSON
both work. external/make_gz.py merges parts automatically when X.json itself is
absent, so `python3 external/make_gz.py` needs no manual prep step.

Usage:
    python3 external/split_data.py split <name.json> [--max-mib 90] [--remove-source]
    python3 external/split_data.py merge <name.json>
    python3 external/split_data.py verify [<name.json>]
    python3 external/split_data.py status
"""

import argparse
import hashlib
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SRC_DIR = ROOT / "internal" / "knowledge" / "data"
DEFAULT_MAX_MIB = 90  # stay under GitHub's 100 MiB blob limit with headroom


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as fh:
        for chunk in iter(lambda: fh.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def part_paths(name: str) -> list[Path]:
    # `.part*` would also match the `.parts` manifest, so pin the digit suffix.
    return sorted(p for p in SRC_DIR.glob(f"{name}.part[0-9][0-9][0-9]"))


def manifest_path(name: str) -> Path:
    return SRC_DIR / f"{name}.parts"


def read_merged(name: str) -> bytes | None:
    """Reassemble a split source, verifying every part and the whole-file
    digest. Returns None when the dataset is not split (no manifest); raises
    ValueError when parts are missing or corrupt. external/make_gz.py calls
    this so a checkout without the whole JSON can still build gz/."""
    mf = manifest_path(name)
    if not mf.is_file():
        return None
    manifest = json.loads(mf.read_text(encoding="utf-8"))
    parts = part_paths(name)
    if len(parts) != len(manifest["parts"]):
        raise ValueError(f"{name}: manifest expects {len(manifest['parts'])} parts, found {len(parts)}")

    buf = bytearray()
    for entry, path in zip(manifest["parts"], parts):
        data = path.read_bytes()
        if len(data) != entry["bytes"] or sha256_bytes(data) != entry["sha256"]:
            raise ValueError(f"{path.name} is corrupt or misaligned")
        buf += data

    if len(buf) != manifest["total_bytes"] or sha256_bytes(bytes(buf)) != manifest["sha256"]:
        raise ValueError(f"{name}: reassembled bytes do not match the manifest sha256")
    return bytes(buf)


def split(name: str, max_bytes: int, remove_source: bool) -> int:
    src = SRC_DIR / name
    if not src.is_file():
        if part_paths(name) or manifest_path(name).is_file():
            print(f"{name}: source absent but parts exist — run `merge {name}` first to re-split")
            return 1
        print(f"ERROR: {src} not found", file=sys.stderr)
        return 1
    size = src.stat().st_size
    if size <= max_bytes:
        print(f"{name}: {size/1048576:.1f}MiB <= {max_bytes/1048576:.0f}MiB, no split needed")
        return 0

    # Drop any previous split so a shrunk file can't leave orphan parts behind.
    for stale in part_paths(name) + [manifest_path(name)]:
        stale.unlink(missing_ok=True)

    total = sha256_file(src)
    parts = []
    with src.open("rb") as fh:
        idx = 0
        while True:
            data = fh.read(max_bytes)
            if not data:
                break
            pname = f"{name}.part{idx:03d}"
            (SRC_DIR / pname).write_bytes(data)
            parts.append({"name": pname, "bytes": len(data), "sha256": sha256_bytes(data)})
            print(f"  wrote {pname}  {len(data)/1048576:.1f}MiB")
            idx += 1

    manifest = {
        "file": name,
        "algorithm": "concat-v1",
        "max_part_bytes": max_bytes,
        "total_bytes": size,
        "sha256": total,
        "parts": parts,
    }
    manifest_path(name).write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    print(f"  wrote {name}.parts ({len(parts)} parts, {size/1048576:.1f}MiB, sha256 {total[:12]})")

    if remove_source:
        src.unlink()
        print(f"  removed {name} (recreate with: python3 external/split_data.py merge {name})")
    return 0


def merge(name: str) -> int:
    try:
        data = read_merged(name)
    except (ValueError, json.JSONDecodeError) as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1
    if data is None:
        print(f"ERROR: {manifest_path(name)} not found", file=sys.stderr)
        return 1
    (SRC_DIR / name).write_bytes(data)
    print(f"merged {name} ({len(data)/1048576:.1f}MiB, sha256 verified)")
    return 0


def verify(name: str | None) -> int:
    manifests = [manifest_path(name)] if name else sorted(SRC_DIR.glob("*.parts"))
    if not manifests:
        print("no split datasets found")
        return 0
    bad = 0
    for mf in manifests:
        target = mf.name[: -len(".parts")]
        manifest = json.loads(mf.read_text(encoding="utf-8"))
        parts = part_paths(target)
        if len(parts) != len(manifest["parts"]):
            print(f"FAIL {target}: {len(manifest['parts'])} parts expected, {len(parts)} present")
            bad += 1
            continue
        total = 0
        digest = hashlib.sha256()
        ok = True
        for entry, path in zip(manifest["parts"], parts):
            data = path.read_bytes()
            if len(data) != entry["bytes"] or sha256_bytes(data) != entry["sha256"]:
                print(f"FAIL {target}: {path.name} corrupt")
                ok = False
                break
            total += len(data)
            digest.update(data)
        if ok and (total != manifest["total_bytes"] or digest.hexdigest() != manifest["sha256"]):
            print(f"FAIL {target}: reassembled sha256 mismatch")
            ok = False
        if ok:
            whole = SRC_DIR / target
            note = "" if not whole.is_file() else f" (source present, {sha256_file(whole) == manifest['sha256'] and 'matches' or 'DIFFERS'})"
            print(f"OK   {target}: {len(parts)} parts, {total/1048576:.1f}MiB{note}")
        else:
            bad += 1
    return 1 if bad else 0


def status() -> int:
    manifests = sorted(SRC_DIR.glob("*.parts"))
    if not manifests:
        print("no split datasets found")
        return 0
    for mf in manifests:
        target = mf.name[: -len(".parts")]
        manifest = json.loads(mf.read_text(encoding="utf-8"))
        have = len(part_paths(target))
        src = "source present" if (SRC_DIR / target).is_file() else "source absent (merge to materialise)"
        print(f"{target}: {manifest['total_bytes']/1048576:.1f}MiB, {have}/{len(manifest['parts'])} parts, {src}")
    return 0


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("cmd", choices=["split", "merge", "verify", "status"])
    ap.add_argument("name", nargs="?", help="file name inside internal/knowledge/data (e.g. huatuo_qa.json)")
    ap.add_argument("--max-mib", type=int, default=DEFAULT_MAX_MIB)
    ap.add_argument("--remove-source", action="store_true", help="delete the whole file after splitting")
    args = ap.parse_args()

    if args.cmd == "status":
        return status()
    if args.cmd == "verify":
        return verify(args.name)
    if not args.name:
        print(f"ERROR: {args.cmd} needs a file name", file=sys.stderr)
        return 1
    if args.cmd == "split":
        return split(args.name, args.max_mib * 1048576, args.remove_source)
    return merge(args.name)


if __name__ == "__main__":
    sys.exit(main())
