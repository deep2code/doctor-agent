"""Plugin 协议：每个源一个模块，实现 fetch(ctx) / convert(ctx)。

ctx: Ctx 数据类（路径、参数、stderr 日志）。fetch 幂等（raw 已在则跳过）；
convert 读 raw → [CorpusDoc]，icd11 特例输出术语表（见 plugins/icd11.py）。
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from datetime import date
from pathlib import Path

from .. import http

ROOT = http.ROOT
DATA_DIR = ROOT / "internal" / "knowledge" / "data"


@dataclass
class Ctx:
    source: str
    raw_dir: Path
    args: dict = field(default_factory=dict)

    def log(self, msg: str):
        print(f"[{self.source}] {msg}")


def today() -> str:
    return date.today().isoformat()


def raw_ctx(source: str, args: dict) -> Ctx:
    return Ctx(source=source, raw_dir=ROOT / "external" / source / "raw", args=args)


def out_path(source: str) -> Path:
    return DATA_DIR / f"corpus_{source}.json"


def write_json(path: Path, data, *, pretty: bool = False):
    path.parent.mkdir(parents=True, exist_ok=True)
    with open(path, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False,
                  separators=(",", ":") if not pretty else None)
