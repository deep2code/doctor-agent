#!/usr/bin/env bash
# ============================================================================
# bake-local.sh — Mac 本机烘焙向量（不在 Docker 内编译和烘焙）
#
# 两种后端:
#   ollama (默认): go run ./cmd/vector-bake, 直连 Ollama bge-m3
#   onnx:          python3 external/bake_onnx.py, ONNX RT INT8 进程内推理
#
# 用法:
#   ./bake-local.sh                           # 默认: ollama, bge-m3, workers=8, batch=128
#   BAKE_BACKEND=onnx ./bake-local.sh         # ONNX RT (快 5-10x)
#   BAKE_BACKEND=onnx BAKE_INT8=1 ./bake-local.sh            # INT8 量化
#   BAKE_BACKEND=onnx ONNX_MODEL=./bge-m3-onnx/model.onnx ./bake-local.sh
#   BAKE_WORKERS=12 ./bake-local.sh           # 自定义 worker 数
#   BAKE_BATCH_SIZE=256 ./bake-local.sh
#   BAKE_RECREATE=1 ./bake-local.sh           # 删除 collection 从零开始
#
# ONNX 后端首次使用:
#   1. pip install optimum-onnx onnxruntime transformers torch qdrant-client zstandard numpy
#   2. optimum-cli 导出 ONNX (opset/量化完全可控):
#      HF_ENDPOINT=https://hf-mirror.com python3 external/export_onnx.py
#   3. BAKE_BACKEND=onnx ONNX_MODEL=./bge-m3-onnx/model.onnx ./bake-local.sh
#
# 运行期查询向量化也走本机 ONNX (替代 Ollama):
#   python3 external/embed_server.py
#   EMBEDDING_BASE_URL=http://localhost:18080/v1 EMBEDDING_MODEL=bge-m3 ./server
#
# 产物: ./qdrant-storage/  (可直接 COPY 进 Dockerfile.qdrant.slim)
# ============================================================================
set -euo pipefail
cd "$(dirname "$0")"

# ── 参数 ──────────────────────────────────────────────
BAKE_BACKEND="${BAKE_BACKEND:-ollama}"
EMBEDDING_MODEL="${EMBEDDING_MODEL:-bge-m3}"
EMBEDDING_BASE_URL="${EMBEDDING_BASE_URL:-http://localhost:11434/v1}"
BAKE_WORKERS="${BAKE_WORKERS:-8}"
BAKE_BATCH_SIZE="${BAKE_BATCH_SIZE:-128}"
BAKE_RECREATE="${BAKE_RECREATE:-}"
BAKE_INT8="${BAKE_INT8:-}"
ONNX_MODEL="${ONNX_MODEL:-./bge-m3-onnx/model.onnx}"
# tokenizer 默认用模型同目录（export_onnx.py 导出时附带），否则回退 HF 名
ONNX_TOKENIZER="${ONNX_TOKENIZER:-}"
if [[ -z "$ONNX_TOKENIZER" ]]; then
  _model_dir="$(cd "$(dirname "$ONNX_MODEL")" 2>/dev/null && pwd)"
  if [[ -f "$_model_dir/tokenizer_config.json" || -f "$_model_dir/tokenizer.json" ]]; then
    ONNX_TOKENIZER="$_model_dir"
  else
    ONNX_TOKENIZER="BAAI/bge-m3"
  fi
fi
QDRANT_PORT="${QDRANT_PORT:-6334}"
COLLECTION="medical_knowledge"
GZ_DIR="internal/knowledge/gz"
STORAGE_DIR="./qdrant-storage"
CONTAINER_NAME="doctor-agent-bake-tmp"

# ── 前置检查: gz 目录 ─────────────────────────────────
if [[ ! -d "$GZ_DIR" ]] || [[ -z "$(ls "$GZ_DIR"/*.json.*z* 2>/dev/null)" ]]; then
  echo "错误: $GZ_DIR 为空，先运行 python3 external/make_gz.py"
  exit 1
fi

# ── 后端特定初始化 ────────────────────────────────────
OLLAMA_STARTED_BY_US=false

if [[ "$BAKE_BACKEND" == "onnx" ]]; then
  # ── ONNX 后端: 检查 Python 依赖和模型文件 ──
  echo "============================================"
  echo "  本机烘焙向量（ONNX RT, bge-m3, dense-only）"
  echo "  backend:     onnx"
  echo "  int8:        ${BAKE_INT8:-no}"
  echo "  model:       $ONNX_MODEL"
  echo "  tokenizer:   $ONNX_TOKENIZER"
  echo "  workers:     $BAKE_WORKERS"
  echo "  batch_size:  $BAKE_BATCH_SIZE"
  echo "  gz 源:       $GZ_DIR"
  echo "  storage 产物: $STORAGE_DIR"
  echo "============================================"
  echo ""

  # 检查 ONNX 模型文件
  if [[ ! -f "$ONNX_MODEL" ]]; then
    echo "错误: ONNX 模型不存在: $ONNX_MODEL"
    echo ""
    echo "  导出方式 (首次, 完全可控 opset/量化):"
    echo "    pip install optimum-onnx onnxruntime transformers torch"
    echo "    HF_ENDPOINT=https://hf-mirror.com python3 external/export_onnx.py"
    exit 1
  fi

  # 检查 Python 依赖
  echo "  检查 Python 依赖..."
  PYTHON_MISSING=""
  for pkg in onnxruntime transformers qdrant_client zstandard numpy; do
    if ! python3 -c "import $pkg" 2>/dev/null; then
      PYTHON_MISSING="$PYTHON_MISSING $pkg"
    fi
  done
  if [[ -n "$PYTHON_MISSING" ]]; then
    echo "错误: 缺少 Python 依赖:$PYTHON_MISSING"
    echo "  安装: pip install onnxruntime transformers qdrant-client zstandard numpy"
    exit 1
  fi
  echo "  Python 依赖 OK"
  echo ""

elif [[ "$BAKE_BACKEND" == "ollama" ]]; then
  # ── Ollama 后端: 管理 Ollama 进程 ──
  echo "============================================"
  echo "  本机烘焙向量（bge-m3 via Ollama）"
  echo "  backend:     ollama"
  echo "  workers:     $BAKE_WORKERS"
  echo "  batch_size:  $BAKE_BATCH_SIZE"
  echo "  embedding:   $EMBEDDING_MODEL @ $EMBEDDING_BASE_URL"
  echo "  gz 源:       $GZ_DIR"
  echo "  storage 产物: $STORAGE_DIR"
  echo "============================================"
  echo ""

  # OLLAMA_NUM_PARALLEL 是服务端设置，必须在 Ollama 启动前设好。
  export OLLAMA_NUM_PARALLEL="${OLLAMA_NUM_PARALLEL:-8}"
  echo "  OLLAMA_NUM_PARALLEL=$OLLAMA_NUM_PARALLEL"

  if curl -sf http://localhost:11434/api/tags >/dev/null 2>&1; then
    echo "  Ollama 已在运行（使用现有实例；如并发度不匹配，可 kill 后重跑本脚本）"
  else
    echo "  Ollama 未启动，正在启动..."
    OLLAMA_NUM_PARALLEL="$OLLAMA_NUM_PARALLEL" nohup ollama serve > /tmp/ollama-bake.log 2>&1 &
    OLLAMA_STARTED_BY_US=true
    for i in $(seq 1 30); do
      if curl -sf http://localhost:11434/api/tags >/dev/null 2>&1; then
        echo "  Ollama 就绪 (${i}s)"
        break
      fi
      if [[ $i -eq 30 ]]; then
        echo "错误: Ollama 30s 内未就绪"
        cat /tmp/ollama-bake.log 2>/dev/null | tail -20
        exit 1
      fi
      sleep 1
    done
  fi

  # 检查 bge-m3 模型是否已拉取
  if ! curl -sf http://localhost:11434/api/tags 2>/dev/null | grep -q "$EMBEDDING_MODEL"; then
    echo "  模型 $EMBEDDING_MODEL 未找到，正在拉取..."
    ollama pull "$EMBEDDING_MODEL"
  fi

  # 检查 Go 工具链
  if ! command -v go >/dev/null 2>&1; then
    echo "错误: 未找到 go 命令"
    exit 1
  fi
  echo ""

else
  echo "错误: 未知 BAKE_BACKEND=$BAKE_BACKEND (支持: ollama, onnx)"
  exit 1
fi

# ── 清理旧产物 ────────────────────────────────
# 防误删: 已有烘焙产物时要求显式确认 (GPU 产物拉回后误跑本脚本曾致其被清)
if [[ -d "$STORAGE_DIR/collections/medical_knowledge" ]] && [[ -z "${BAKE_OVERWRITE:-}" ]]; then
  echo "错误: $STORAGE_DIR 已有烘焙产物 (medical_knowledge)"
  echo "  确认覆盖重烘: BAKE_OVERWRITE=1 $0"
  echo "  产物已就位想直接打包: 直接 ./build.sh qdrant (会自动检测已有产物)"
  exit 1
fi
rm -rf "$STORAGE_DIR"
mkdir -p "$STORAGE_DIR"

# ── 启动临时 Qdrant 容器 ───────────────────────
echo "[1/3] 启动临时 Qdrant 容器..."
docker rm -f "$CONTAINER_NAME" 2>/dev/null || true

docker run -d --name "$CONTAINER_NAME" \
  -p "${QDRANT_PORT}:6334" \
  -p "$((QDRANT_PORT-1)):6333" \
  -v "$(pwd)/$STORAGE_DIR:/qdrant/storage" \
  -e QDRANT__STORAGE__STORAGE_PATH=/qdrant/storage \
  -e QDRANT__LOG_LEVEL=INFO \
  qdrant/qdrant:v1.19.0

# 等待 Qdrant 就绪
echo "  等待 Qdrant 就绪..."
for i in $(seq 1 30); do
  if curl -sf "http://localhost:$((QDRANT_PORT-1))/healthz" >/dev/null 2>&1; then
    echo "  Qdrant 就绪 (${i}s)"
    break
  fi
  if [[ $i -eq 30 ]]; then
    echo "错误: Qdrant 30s 内未就绪"
    docker logs "$CONTAINER_NAME" 2>&1 | tail -20
    exit 1
  fi
  sleep 1
done

# ── 烘焙 ───────────────────────────────────────
RECREATE_FLAG=""
if [[ -n "$BAKE_RECREATE" ]]; then
  RECREATE_FLAG="--recreate"
fi

echo ""
echo "[2/3] 本机烘焙..."

set +e

if [[ "$BAKE_BACKEND" == "onnx" ]]; then
  INT8_FLAG=""
  if [[ -n "$BAKE_INT8" ]]; then
    INT8_FLAG="--int8"
  fi
  echo "  ONNX RT 进程内推理 (无 HTTP 开销)"
  echo ""
  python3 external/bake_onnx.py \
    --src="$GZ_DIR" \
    --host=localhost \
    --port="$QDRANT_PORT" \
    --collection="$COLLECTION" \
    --model="$ONNX_MODEL" \
    --tokenizer="$ONNX_TOKENIZER" \
    --workers="$BAKE_WORKERS" \
    --batch-size="$BAKE_BATCH_SIZE" \
    --max-text-chars=1024 \
    $INT8_FLAG \
    $RECREATE_FLAG
  BAKE_RC=$?
else
  echo "  直连 Ollama localhost:11434（无 Docker 网络开销）"
  echo ""
  export EMBEDDING_BASE_URL="$EMBEDDING_BASE_URL"
  export EMBEDDING_MODEL="$EMBEDDING_MODEL"
  go run ./cmd/vector-bake \
    --src="$GZ_DIR" \
    --host=localhost \
    --port="$QDRANT_PORT" \
    --collection="$COLLECTION" \
    --workers="$BAKE_WORKERS" \
    --batch-size="$BAKE_BATCH_SIZE" \
    --max-text-chars=1024 \
    --wait-green=600 \
    $RECREATE_FLAG
  BAKE_RC=$?
fi

set -e
echo ""

# ── 停止 Qdrant 容器 ───────────────────────────
echo "[3/3] 停止 Qdrant 容器，清理 WAL..."
docker stop "$CONTAINER_NAME" 2>/dev/null || true
docker rm "$CONTAINER_NAME" 2>/dev/null || true

# 清理 WAL 文件（数据已固化在 segment 中）
find "$STORAGE_DIR" -path '*/wal/*' -type f -delete 2>/dev/null || true

# ── 结果 ───────────────────────────────────────
if [[ $BAKE_RC -ne 0 ]]; then
  echo "❌ 烘焙失败 (exit $BAKE_RC)"
  exit 1
fi

STORAGE_SIZE=$(du -sh "$STORAGE_DIR" 2>/dev/null | cut -f1)
SEGMENT_COUNT=$(find "$STORAGE_DIR" -name "*.seg" -o -name "*.idx" 2>/dev/null | wc -l | tr -d ' ')
echo ""
echo "============================================"
echo "  ✅ 烘焙完成"
echo "  backend:    $BAKE_BACKEND"
echo "  storage:    $STORAGE_DIR ($STORAGE_SIZE)"
echo "  segments:   $SEGMENT_COUNT 个文件"
echo "  下一步:     ./build.sh qdrant （用 Dockerfile.qdrant.slim 打包）"
echo "============================================"

# 如果 Ollama 是本脚本启动的，停止它
if [[ "$OLLAMA_STARTED_BY_US" == "true" ]]; then
  pkill -f "ollama serve" 2>/dev/null || true
fi
