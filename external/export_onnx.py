#!/usr/bin/env python3
"""
export_onnx.py — 用 optimum-cli 手动导出 embedding 模型为 ONNX（本机向量化第 1 步）

相比 ORTModelForFeatureExtraction(export=True) 一把梭，本脚本对导出过程完全可控:
  - opset 版本 (--opset)
  - 精度 dtype fp32/fp16/bf16 (--dtype)
  - 图优化级别 O1-O4 (--optimize)
  - 导出后可选 INT8 动态量化 (--int8, 产物 *.int8.onnx, 与 bake_onnx.py --int8 缓存名一致)

依赖: pip install optimum-onnx onnxruntime transformers torch

用法:
  python3 external/export_onnx.py                                  # BAAI/bge-m3 -> ./bge-m3-onnx
  python3 external/export_onnx.py --opset 17 --int8                # 导出 + 量化
  python3 external/export_onnx.py --model XAAI/xxx --out ./xxx-onnx
  HF_ENDPOINT=https://hf-mirror.com python3 external/export_onnx.py   # 国内镜像

产物: <out>/model.onnx (+ tokenizer), 供 bake_onnx.py / embed_server.py 推理
"""
import argparse
import os
import subprocess
import sys


def sh(cmd: list) -> None:
    print("+", " ".join(cmd))
    rc = subprocess.call(cmd)
    if rc != 0:
        sys.exit(f"ERROR: 命令失败 (exit {rc}): {' '.join(cmd)}")


def model_io(model_path: str) -> None:
    """打印模型 I/O 签名，确认输入名/输出名（推理脚本按名字取输出）"""
    import onnxruntime as ort

    sess = ort.InferenceSession(model_path, providers=["CPUExecutionProvider"])
    print("\n模型 I/O:")
    for i in sess.get_inputs():
        print(f"  input  {i.name}: {i.shape} {i.type}")
    for o in sess.get_outputs():
        print(f"  output {o.name}: {o.shape} {o.type}")


def quantize_int8(model_path: str) -> str:
    """INT8 动态量化（权重 QInt8），命名与 bake_onnx.py 的缓存查找一致"""
    from onnxruntime.quantization import quantize_dynamic, QuantType

    int8_path = model_path.replace(".onnx", ".int8.onnx")
    if os.path.exists(int8_path):
        print(f"INT8 已存在，跳过: {int8_path}")
        return int8_path

    print(f"INT8 动态量化（一次性，几分钟）...")
    quantize_dynamic(model_path, int8_path, weight_type=QuantType.QInt8)
    mb = lambda p: os.path.getsize(p) / 1024 / 1024
    print(f"量化完成: {mb(model_path):.1f}MB -> {mb(int8_path):.1f}MB")
    return int8_path


def main() -> None:
    ap = argparse.ArgumentParser(description="optimum-cli ONNX 导出封装")
    ap.add_argument("--model", default="BAAI/bge-m3", help="HF 模型名或本地路径")
    ap.add_argument("--out", default="./bge-m3-onnx", help="输出目录")
    ap.add_argument("--task", default="feature-extraction", help="导出任务")
    ap.add_argument("--opset", type=int, default=17)
    ap.add_argument("--dtype", default="fp32", choices=["fp32", "fp16", "bf16"],
                    help="CPU 推理建议 fp32（fp16 在 CPU 上更慢）")
    ap.add_argument("--optimize", default=None, choices=["O1", "O2", "O3", "O4"],
                    help="图优化级别；INT8 量化前勿用 O4")
    ap.add_argument("--atol", type=float, default=None, help="导出校验容差")
    ap.add_argument("--int8", action="store_true", help="导出后做 INT8 动态量化")
    args = ap.parse_args()

    os.makedirs(args.out, exist_ok=True)
    model_path = os.path.join(args.out, "model.onnx")

    cmd = [
        sys.executable, "-m", "optimum.exporters.onnx",
        "-m", args.model,
        "--task", args.task,
        "--opset", str(args.opset),
        "--dtype", args.dtype,
        args.out,
    ]
    if args.optimize:
        cmd = cmd[:2] + ["--optimize", args.optimize] + cmd[2:]
    if args.atol is not None:
        cmd = cmd[:2] + ["--atol", str(args.atol)] + cmd[2:]

    print("=" * 50)
    print(f"导出 {args.model} -> {model_path}")
    print(f"  opset={args.opset} dtype={args.dtype} optimize={args.optimize or 'default'}")
    print("=" * 50)
    sh(cmd)

    if not os.path.exists(model_path):
        # 某些版本产物名带前缀（如 encoder_model.onnx），取目录里最大的 onnx 归位
        import glob
        cands = [p for p in glob.glob(os.path.join(args.out, "*.onnx"))
                 if not p.endswith(".int8.onnx")]
        if not cands:
            sys.exit(f"ERROR: {args.out} 下未找到导出的 .onnx")
        biggest = max(cands, key=os.path.getsize)
        os.replace(biggest, model_path)
        print(f"产物归位: {os.path.basename(biggest)} -> model.onnx")

    final = quantize_int8(model_path) if args.int8 else model_path
    model_io(final)
    print("\n下一步:")
    print(f"  烘焙:  BAKE_BACKEND=onnx ONNX_MODEL={final} ./bake-local.sh")
    print(f"  查询:  python3 external/embed_server.py --model {final}")


if __name__ == "__main__":
    main()
