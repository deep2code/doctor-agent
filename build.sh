#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

# ============================================================================
# doctor-agent 唯一打包入口（构建 + 推送阿里云）
#
# 用法:
#   ./build.sh           = app（默认）：只构建+推送 app 镜像（改代码/前端）
#   ./build.sh qdrant    = 只构建+推送 RAG 镜像（改知识库数据、跑 python3 external/make_gz.py 之后）
#   ./build.sh full      = 全量：app + qdrant 一起
#
# 双镜像架构:
#   doctor-agent         Go 源码 + 前端        → 代码变化才更新
#   doctor-agent-qdrant  Qdrant + 烘好的向量    → 知识库变化才更新
#   macOS 流程: 有 qdrant-storage 产物 → 直接打包; 无 → bake-local.sh 本机烘焙 → 打包
#               (有产物时不提供强制重烘选项; 要重烘先手动删产物或跑 bake-local.sh)
#   Linux  流程: Dockerfile.qdrant 内编译+烘焙(FNV hash)
# ============================================================================

MODE="${1:-app}"
case "$MODE" in
  app|qdrant|full) ;;
  *) echo "用法: $0 [app|qdrant|full]"; exit 1 ;;
esac

# ── 版本信息 ──────────────────────────────────────
GIT_COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
BUILD_TIME="$(TZ=Asia/Shanghai date '+%Y-%m-%dT%H:%M:%S+08:00')"
GIT_TAG="$(git describe --tags --abbrev=0 2>/dev/null || echo latest)"

# ── 镜像仓库（唯一定义处；Linux 推送机走内网）─────
REGISTRY_PUBLIC="crpi-0xi5k79l9j4opzta.cn-hangzhou.personal.cr.aliyuncs.com/codeup2026"
REGISTRY_VPC="crpi-0xi5k79l9j4opzta-vpc.cn-hangzhou.personal.cr.aliyuncs.com/codeup2026"
if [[ "$(uname -s)" == "Linux" ]]; then
  REGISTRY="$REGISTRY_VPC"
else
  REGISTRY="$REGISTRY_PUBLIC"
fi

APP_IMAGE="${REGISTRY}/doctor-agent:${GIT_TAG}"
APP_IMAGE_LATEST="${REGISTRY}/doctor-agent:latest"
QDRANT_IMAGE="${REGISTRY}/doctor-agent-qdrant:latest"

build_app() {
  echo "[app] 构建应用镜像（Go 源码 + 前端，.dockerignore 排除 gz）..."

  docker build --progress=plain --platform linux/amd64 \
    --pull=false \
    --build-arg GIT_COMMIT="${GIT_COMMIT}" \
    --build-arg BUILD_TIME="${BUILD_TIME}" \
    -t "$APP_IMAGE" \
    -t "$APP_IMAGE_LATEST" \
    -f Dockerfile \
    --provenance false \
    .
}

build_qdrant() {
  echo "[qdrant] 构建 RAG 镜像..."
  if [[ ! -d "internal/knowledge/gz" ]] || [[ -z "$(ls internal/knowledge/gz/*.json.*z* 2>/dev/null)" ]]; then
    echo "  错误: internal/knowledge/gz 为空，先运行 python3 external/make_gz.py 生成知识库压缩包"
    exit 1
  fi

  # macOS：本机烘焙 + slim 镜像打包（全量真向量，不省资源）
  #
  # 两步流程：
  #   1. bake-local.sh — Mac 本机直跑 vector-bake，直连 localhost:11434
  #      （无 Docker 网络开销），OLLAMA_NUM_PARALLEL=8 + workers=8 + batch=128，
  #      bake.go 按文本长度排序消除 padding 浪费。
  #      全部 743k 条数据用 bge-m3 生成真实语义向量（不跳过任何数据集）。
  #   2. Dockerfile.qdrant.slim — 只 COPY 预烘焙 storage 到 Qdrant 基础镜像
  #      （~30 秒纯 COPY，无编译无烘焙）。
  #
  # 对比旧方案（Docker 内烘焙）：
  #   - 消除 host.docker.internal 网络开销
  #   - 消除 BuildKit 输出缓冲（看不到进度）
  #   - 消除 Docker 内存限制（16GB 全可用）
  #   - 文本长度排序 3-5x 加速（padding 浪费消除）
  #   - OLLAMA_NUM_PARALLEL=8 embedding 并发（无 KV cache，几乎零额外内存）
  if [[ "$(uname -s)" == "Darwin" ]]; then
    echo "  macOS → RAG 镜像（有烘焙产物直接打包，无产物才本机烘焙）"

    # Step 1: 烘焙产物检测 — 有产物直接打包，不允许强制重烘
    #   ./build.sh qdrant    本机已有 qdrant-storage → 直接打包；没有 → 本机烘焙
    if [[ -d "qdrant-storage/collections/medical_knowledge" ]]; then
      echo "  [1/2] 检测到已有烘焙产物 → 跳过烘焙，直接打包"
    else
      echo "  [1/2] 无烘焙产物，本机烘焙..."
      # 传递 BAKE_RECREATE 和自定义参数
      export EMBEDDING_MODEL="${EMBEDDING_MODEL:-bge-m3}"
      export BAKE_WORKERS="${BAKE_WORKERS:-8}"
      export BAKE_BATCH_SIZE="${BAKE_BATCH_SIZE:-128}"
      ./bake-local.sh

      # 检查烘焙产物
      if [[ ! -d "qdrant-storage" ]] || [[ -z "$(ls qdrant-storage/ 2>/dev/null)" ]]; then
        echo "  错误: 烘焙产物 qdrant-storage/ 为空"
        exit 1
      fi
    fi

    # Step 2: slim 镜像打包（只 COPY storage）
    echo "  [2/2] 打包 slim 镜像（Dockerfile.qdrant.slim）..."
    docker build --progress=plain --platform linux/amd64 \
      --pull=false \
      -t "$QDRANT_IMAGE" \
      -f Dockerfile.qdrant.slim \
      --provenance false \
      .

    # 清理本机烘焙产物（镜像已包含 storage）
    rm -rf qdrant-storage
  else
    echo "  Linux → Docker 内烘焙（FNV hash 离线 embedding，无 Ollama）"
    docker build --progress=plain --platform linux/amd64 \
      --pull=false \
      -t "$QDRANT_IMAGE" \
      -f Dockerfile.qdrant \
      --provenance false \
      .
  fi
}

push_image() {
  echo "  推送: $1"
  docker push "$1"
}

echo "============================================"
echo "  doctor-agent 打包（模式: ${MODE}）"
echo "  版本:      ${GIT_TAG} (commit ${GIT_COMMIT})"
echo "  构建时间:  ${BUILD_TIME}"
echo "  应用镜像:  ${APP_IMAGE_LATEST}      ← 代码变化才更新"
echo "  RAG 镜像:  ${QDRANT_IMAGE}   ← 知识库变化才更新"
echo "============================================"
echo ""

case "$MODE" in
  app)
    build_app
    push_image "$APP_IMAGE"
    push_image "$APP_IMAGE_LATEST"
    ;;
  qdrant)
    build_qdrant
    push_image "$QDRANT_IMAGE"
    ;;
  full)
    build_app
    push_image "$APP_IMAGE"
    push_image "$APP_IMAGE_LATEST"
    build_qdrant
    push_image "$QDRANT_IMAGE"
    ;;
esac

echo ""
echo "============================================"
echo "  打包完成（${MODE}）"
echo "  触发关系: 改代码 → ./build.sh | 改知识库 → ./build.sh qdrant | 都改 → ./build.sh full"
echo "============================================"
