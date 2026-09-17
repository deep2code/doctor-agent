#!/usr/bin/env python3
"""
embed_server.py — 本机 ONNX embedding 服务（OpenAI /v1/embeddings 兼容，本机向量化第 3 步）

用与 bake_onnx.py 相同的 ONNX 模型 + CLS pooling + L2 normalize，保证查询向量与
烘焙向量同空间。替代 Ollama，运行期查询向量化也全在本机完成，Go 端零改动:
    EMBEDDING_BASE_URL=http://localhost:18080/v1 EMBEDDING_MODEL=bge-m3 ./server

依赖: pip install onnxruntime transformers numpy（与 bake_onnx.py 相同）

用法:
  python3 external/embed_server.py                       # ./bge-m3-onnx/model.onnx @ :18080
  python3 external/embed_server.py --port 18080 --int8   # INT8 量化模型
  curl -s localhost:18080/v1/embeddings -d '{"input":["小儿发热"]}'

端点:
  POST /v1/embeddings   {"input": "text" | ["t1","t2"], "model"?, "dimensions"?(忽略)}
  GET  /healthz         -> {"status":"ok","model":...,"dimensions":N}
"""
import argparse
import json
import os
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class Embedder:
    """ONNX RT embedder: tokenize -> 推理 -> CLS pooling -> L2 normalize"""

    def __init__(self, model_path: str, tokenizer_name: str,
                 workers: int = 8, int8: bool = False, max_text_chars: int = 1024):
        self.max_text_chars = max_text_chars
        self.model_path = _ensure_model(model_path, int8)

        from transformers import AutoTokenizer
        self.tokenizer = AutoTokenizer.from_pretrained(tokenizer_name)

        import onnxruntime as ort
        opts = ort.SessionOptions()
        opts.graph_optimization_level = ort.GraphOptimizationLevel.ORT_ENABLE_ALL
        opts.intra_op_num_threads = workers

        # 仅 CPU: CoreML EP 对该模型动态 shape 分区执行不稳定（短输入能过,
        # 真实 batch 中途报错），确定性优先
        providers = ["CPUExecutionProvider"]
        print(f"  providers:  {providers}")
        self.session = ort.InferenceSession(self.model_path, sess_options=opts, providers=providers)

        self.input_names = [i.name for i in self.session.get_inputs()]
        self.output_names = [o.name for o in self.session.get_outputs()]
        self.dimensions = None  # 首次推理后填充

    def embed_batch(self, texts: list) -> list:
        import numpy as np

        processed = []
        for t in texts:
            if not isinstance(t, str):
                raise ValueError(f"input 必须是 string, 得到 {type(t).__name__}")
            if not t:
                t = ""
            if self.max_text_chars > 0 and len(t) > self.max_text_chars:
                t = t[:self.max_text_chars]
            processed.append(t)

        encoded = self.tokenizer(
            processed,
            padding=len(processed) > 1,
            truncation=True,
            max_length=8192,
            return_tensors="np",
        )
        inputs = {}
        for name in self.input_names:
            if name in encoded:
                inputs[name] = encoded[name]
            elif name == "token_type_ids":
                inputs[name] = np.zeros_like(encoded["input_ids"])

        outputs = self.session.run(None, inputs)
        for key in ("last_hidden_state", "token_embeddings"):
            if key in self.output_names:
                last_hidden = outputs[self.output_names.index(key)]
                break
        else:
            last_hidden = outputs[0]

        embeddings = last_hidden[:, 0]  # CLS pooling
        norms = np.linalg.norm(embeddings, axis=1, keepdims=True)
        embeddings = embeddings / np.maximum(norms, 1e-9)
        if self.dimensions is None:
            self.dimensions = int(embeddings.shape[1])
        return embeddings.astype(np.float32).tolist()


def _ensure_model(model_path: str, int8: bool) -> str:
    """与 bake_onnx.py --int8 缓存逻辑一致: <model>.int8.onnx"""
    if not int8:
        return model_path
    basename = os.path.basename(model_path).lower()
    if "int8" in basename or "quant" in basename:
        return model_path
    int8_path = model_path.replace(".onnx", ".int8.onnx")
    if os.path.exists(int8_path):
        return int8_path
    sys.exit(f"ERROR: 未找到 INT8 模型 {int8_path}（先跑 external/export_onnx.py --int8）")


class Handler(BaseHTTPRequestHandler):
    embedder: Embedder = None  # type: ignore[assignment]
    model_label: str = ""

    def log_message(self, fmt, *args):  # 静默默认访问日志
        pass

    def _json(self, code: int, payload: dict) -> None:
        body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path in ("/healthz", "/health"):
            self._json(200, {"status": "ok", "model": self.model_label,
                             "dimensions": self.embedder.dimensions})
        else:
            self._json(404, {"error": {"message": f"not found: {self.path}"}})

    def do_POST(self):
        if self.path.rstrip("/") != "/v1/embeddings":
            self._json(404, {"error": {"message": f"not found: {self.path}"}})
            return

        try:
            length = int(self.headers.get("Content-Length", 0))
            payload = json.loads(self.rfile.read(length) or b"{}")
        except (ValueError, json.JSONDecodeError) as e:
            self._json(400, {"error": {"message": f"invalid JSON: {e}"}})
            return

        raw = payload.get("input")
        if raw is None:
            self._json(400, {"error": {"message": "missing field: input"}})
            return
        texts = [raw] if isinstance(raw, str) else raw
        if not isinstance(texts, list) or not texts or \
                not all(isinstance(t, str) for t in texts):
            self._json(400, {"error": {"message": "input 必须是 string 或 string[]"}})
            return

        try:
            vectors = self.embedder.embed_batch(texts)
        except Exception as e:  # noqa: BLE001 — 统一转 500 JSON
            self._json(500, {"error": {"message": f"embed failed: {e}"}})
            return

        prompt_tokens = sum(len(t) for t in texts)  # 近似值, 仅为兼容字段
        self._json(200, {
            "object": "list",
            "data": [{"object": "embedding", "index": i, "embedding": v}
                     for i, v in enumerate(vectors)],
            "model": self.model_label,
            "usage": {"prompt_tokens": prompt_tokens, "total_tokens": prompt_tokens},
        })


def resolve_tokenizer(model_path: str, tokenizer_name: str) -> str:
    """tokenizer 未显式指定时, 优先用模型同目录（optimum 导出附带 tokenizer）"""
    if tokenizer_name:
        return tokenizer_name
    d = os.path.dirname(os.path.abspath(model_path))
    if os.path.exists(os.path.join(d, "tokenizer_config.json")) or \
            os.path.exists(os.path.join(d, "tokenizer.json")):
        return d
    return "BAAI/bge-m3"


def main() -> None:
    ap = argparse.ArgumentParser(description="本机 ONNX embedding 服务 (OpenAI 兼容)")
    ap.add_argument("--model", default="./bge-m3-onnx/model.onnx")
    ap.add_argument("--tokenizer", default="", help="默认用模型同目录, 否则 HF 名")
    ap.add_argument("--host", default="127.0.0.1")
    ap.add_argument("--port", type=int, default=18080)
    ap.add_argument("--workers", type=int, default=8, help="ORT intra-op 线程数")
    ap.add_argument("--int8", action="store_true", help="使用 <model>.int8.onnx")
    ap.add_argument("--max-text-chars", type=int, default=1024)
    args = ap.parse_args()

    tok = resolve_tokenizer(args.model, args.tokenizer)
    print("本机 embedding 服务启动:")
    print(f"  model:     {args.model}")
    print(f"  tokenizer: {tok}")
    print(f"  int8:      {bool(args.int8)}")
    embedder = Embedder(args.model, tok, workers=args.workers,
                        int8=args.int8, max_text_chars=args.max_text_chars)

    Handler.embedder = embedder
    Handler.model_label = os.path.splitext(os.path.basename(embedder.model_path))[0] or "bge-m3"

    # warmup: 填充 dimensions + 预热 ORT, 首条查询不再吃冷启动
    embedder.embed_batch(["warmup"])
    print(f"  dimensions: {embedder.dimensions}")

    server = ThreadingHTTPServer((args.host, args.port), Handler)
    print(f"  listening: http://{args.host}:{args.port}  (POST /v1/embeddings, GET /healthz)")
    print("  Go 端接入: EMBEDDING_BASE_URL="
          f"http://{args.host}:{args.port}/v1 EMBEDDING_MODEL={Handler.model_label}")
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\n停止")


if __name__ == "__main__":
    main()
