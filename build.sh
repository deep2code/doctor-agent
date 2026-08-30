#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

# ── 模式 ──────────────────────────────────────────
#   ./build.sh          = app（默认）：只构建 app 镜像（代码/前端变化）
#   ./build.sh full     = full：构建 app + qdrant 全量（知识库也变化）
MODE="${1:-app}"

# ── 版本信息 ──────────────────────────────────────
GIT_COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
BUILD_TIME="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
GIT_TAG="$(git describe --tags --abbrev=0 2>/dev/null || echo latest)"

# ── 镜像仓库 ──────────────────────────────────────
REGISTRY_PUBLIC="crpi-0xi5k79l9j4opzta.cn-hangzhou.personal.cr.aliyuncs.com/codeup2026"
REGISTRY_VPC="crpi-0xi5k79l9j4opzta-vpc.cn-hangzhou.personal.cr.aliyuncs.com/codeup2026"

# Linux 机器走内网推送
if [[ "$(uname -s)" == "Linux" ]]; then
  REGISTRY="$REGISTRY_VPC"
else
  REGISTRY="$REGISTRY_PUBLIC"
fi

APP_IMAGE="${REGISTRY}/doctor-agent:${GIT_TAG}"
APP_IMAGE_LATEST="${REGISTRY}/doctor-agent:latest"
QDRANT_IMAGE="${REGISTRY}/doctor-agent-qdrant:latest"

echo "============================================"
echo "  doctor-agent 打包（双镜像）"
echo "  模式:      ${MODE}"
echo "  版本:      ${GIT_TAG} (commit ${GIT_COMMIT})"
echo "  构建时间:  ${BUILD_TIME}"
echo "  应用镜像:  ${APP_IMAGE}          ← Go 源码+前端，代码变化才更新"
echo "              ${APP_IMAGE_LATEST}"
echo "  RAG 镜像:  ${QDRANT_IMAGE}        ← gz知识库+Qdrant+向量，知识库变化才更新"
echo "============================================"

echo ""
echo "[1/2] 构建应用镜像（Go 源码 + 前端，.dockerignore 排除 gz）..."
docker build --platform linux/amd64 \
  --build-arg GIT_COMMIT="${GIT_COMMIT}" \
  --build-arg BUILD_TIME="${BUILD_TIME}" \
  -t "$APP_IMAGE" \
  -t "$APP_IMAGE_LATEST" \
  -f Dockerfile \
  --provenance false \
  .

# RAG 镜像只在 full 模式构建（知识库变化才需要重烤向量）
if [[ "$MODE" == "full" ]]; then
  echo ""
  echo "[2/2] 构建 RAG 镜像（独立 context：gz 知识库 + 标准 Qdrant + 烘好的向量）..."
  if [[ ! -d "internal/knowledge/gz" ]] || [[ -z "$(ls internal/knowledge/gz/*.gz 2>/dev/null)" ]]; then
    echo "  警告: gz 目录为空，跳过 RAG 镜像"
  else
    docker/qdrant-context/build.sh "$QDRANT_IMAGE" "$APP_IMAGE_LATEST"
  fi
else
  echo ""
  echo "[2/2] 跳过 RAG 镜像（模式=app；知识库变化时用 ./build.sh full 全量重建）"
fi

echo ""
echo "推送镜像 ..."
echo "  推送: $APP_IMAGE"
docker push "$APP_IMAGE"
echo "  推送: $APP_IMAGE_LATEST"
docker push "$APP_IMAGE_LATEST"

if [[ "$MODE" == "full" ]] && docker image inspect "$QDRANT_IMAGE" &>/dev/null; then
  echo "  推送: $QDRANT_IMAGE"
  docker push "$QDRANT_IMAGE"
fi

echo ""
echo "============================================"
echo "  打包完成"
echo "  版本标签: ${GIT_TAG}"
echo "  触发关系: 改 gz → ./build.sh full；改代码 → ./build.sh (app)"
echo "============================================"
