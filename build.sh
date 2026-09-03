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
#   doctor-agent-qdrant  gz 知识库+Qdrant+向量 → 知识库变化才更新
#                          (烘焙工具 cmd/vector-bake 在镜像内自行编译，
#                           不依赖 app 镜像)
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

  docker build --platform linux/amd64 \
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
  echo "[qdrant] 构建 RAG 镜像（根目录构建：gz 知识库 + Qdrant + 烘好的向量）..."
  if [[ ! -d "internal/knowledge/gz" ]] || [[ -z "$(ls internal/knowledge/gz/*.json.*z* 2>/dev/null)" ]]; then
    echo "  错误: internal/knowledge/gz 为空，先运行 python3 external/make_gz.py 生成知识库压缩包"
    exit 1
  fi
  docker build --platform linux/amd64 \
    --pull=false \
    -t "$QDRANT_IMAGE" \
    -f Dockerfile.qdrant \
    --provenance false \
    .
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
