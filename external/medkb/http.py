"""共享 HTTP：UA、重试退避、磁盘缓存（幂等续跑）。仅 stdlib。"""

from __future__ import annotations

import hashlib
import json
import os
import sys
import time
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent.parent  # 仓库根
CACHE_DIR = ROOT / "external" / ".cache" / "http"
UA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) medkb/1.0 (personal research; contact: local)"


def _cache_path(url: str) -> Path:
    return CACHE_DIR / hashlib.sha256(url.encode()).hexdigest()


def get(url: str, *, binary: bool = False, retries: int = 4, timeout: int = 90,
        cache: bool = False, headers: dict | None = None):
    """GET url → str（binary=False）或 bytes。cache=True 时磁盘缓存。"""
    cp = _cache_path(url)
    if cache and cp.exists():
        data = cp.read_bytes()
        return data if binary else data.decode("utf-8", "replace")
    hdrs = {"User-Agent": UA}
    if headers:
        hdrs.update(headers)
    last = None
    for attempt in range(retries):
        req = urllib.request.Request(url, headers=hdrs)
        try:
            with urllib.request.urlopen(req, timeout=timeout) as resp:
                data = resp.read()
            if cache:
                CACHE_DIR.mkdir(parents=True, exist_ok=True)
                cp.write_bytes(data)
            return data if binary else data.decode("utf-8", "replace")
        except Exception as e:  # noqa: BLE001 — 网络/HTTP 全部退避重试
            last = e
            wait = min(2 ** attempt * 2, 30)
            print(f"  GET {url[:120]} failed ({e}), retry in {wait}s", file=sys.stderr)
            time.sleep(wait)
    raise RuntimeError(f"GET {url} failed after {retries} attempts: {last}")


def get_json(url: str, **kw):
    txt = get(url, **kw)
    return json.loads(txt)


def post_json(url: str, payload: dict, *, headers: dict | None = None,
              retries: int = 3, timeout: int = 120) -> dict:
    """POST JSON → 解析响应 JSON。"""
    hdrs = {"Content-Type": "application/json", "User-Agent": UA}
    if headers:
        hdrs.update(headers)
    data = json.dumps(payload).encode()
    last = None
    for attempt in range(retries):
        req = urllib.request.Request(url, data=data, headers=hdrs, method="POST")
        try:
            with urllib.request.urlopen(req, timeout=timeout) as resp:
                return json.loads(resp.read())
        except Exception as e:  # noqa: BLE001
            last = e
            time.sleep(min(2 ** attempt * 2, 20))
    raise RuntimeError(f"POST {url} failed after {retries} attempts: {last}")


def download(url: str, dest: Path, *, retries: int = 4, timeout: int = 300,
             headers: dict | None = None) -> Path:
    """流式下载大文件（tar/zip），已存在则跳过（幂等）。"""
    if dest.exists() and dest.stat().st_size > 0:
        return dest
    dest.parent.mkdir(parents=True, exist_ok=True)
    hdrs = {"User-Agent": UA}
    if headers:
        hdrs.update(headers)
    last = None
    for attempt in range(retries):
        req = urllib.request.Request(url, headers=hdrs)
        try:
            with urllib.request.urlopen(req, timeout=timeout) as resp, open(dest, "wb") as f:
                total = int(resp.headers.get("Content-Length") or 0)
                done = 0
                while True:
                    chunk = resp.read(1 << 20)
                    if not chunk:
                        break
                    f.write(chunk)
                    done += len(chunk)
                    if total and done % (64 << 20) < (1 << 20):
                        print(f"  {dest.name}: {done >> 20}MiB / {total >> 20}MiB", file=sys.stderr)
            return dest
        except Exception as e:  # noqa: BLE001
            last = e
            dest.unlink(missing_ok=True)
            wait = min(2 ** attempt * 2, 30)
            print(f"  DL {url[:120]} failed ({e}), retry in {wait}s", file=sys.stderr)
            time.sleep(wait)
    raise RuntimeError(f"download {url} failed: {last}")
