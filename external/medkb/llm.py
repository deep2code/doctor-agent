"""glm 客户端 + 磁盘缓存（本阶段预留；英文源原文入库暂不依赖 LLM）。

用法与现有脚本一致：ZHIPU_API_KEY 环境变量、glm-4-flash（详见 external/LLM_PROVIDERS.md）。
注意：glm-4.7-flash 需 thinking:{"type":"disabled"} 关推理模式（structurize_dailymed.py 坑）。
"""

from __future__ import annotations

import hashlib
import json
import sys
import time
import urllib.request
from pathlib import Path

from . import http

ZHIPU_URL = "https://open.bigmodel.cn/api/paas/v4/chat/completions"
CACHE_DIR = http.ROOT / "external" / ".cache" / "llm"


def _cache_path(model: str, prompt: str) -> Path:
    return CACHE_DIR / f"{model}.{hashlib.sha256(prompt.encode()).hexdigest()}.json"


def chat(model: str, prompt: str, *, temperature: float = 0.1, cache: bool = True,
         thinking_disabled: bool = False, max_retries: int = 3) -> str:
    """单轮对话 → 文本。cache=True 时按 (model, prompt) 缓存，断点续跑不重扣 token。"""
    cp = _cache_path(model, prompt)
    if cache and cp.exists():
        return json.loads(cp.read_text())["content"]
    key = os_environ_key()
    if not key:
        raise RuntimeError("缺 ZHIPU_API_KEY 环境变量")
    payload = {"model": model, "messages": [{"role": "user", "content": prompt}],
               "temperature": temperature}
    if thinking_disabled:
        payload["thinking"] = {"type": "disabled"}
    last = None
    for attempt in range(max_retries):
        try:
            data = http.post_json(ZHIPU_URL, payload,
                                  headers={"Authorization": f"Bearer {key}"})
            content = data["choices"][0]["message"]["content"].strip()
            if cache:
                CACHE_DIR.mkdir(parents=True, exist_ok=True)
                cp.write_text(json.dumps({"content": content}))
            return content
        except Exception as e:  # noqa: BLE001
            last = e
            time.sleep(min(2 ** attempt * 3, 30))
    raise RuntimeError(f"glm chat failed: {last}")


def os_environ_key() -> str:
    import os
    return os.environ.get("ZHIPU_API_KEY", "")
