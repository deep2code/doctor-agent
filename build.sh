#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

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
DATA_IMAGE="${REGISTRY}/doctor-agent-data:latest"

echo "============================================"
echo "  doctor-agent 打包"
echo "  版本:    ${GIT_TAG} (commit ${GIT_COMMIT})"
echo "  构建时间: ${BUILD_TIME}"
echo "  应用镜像: ${APP_IMAGE}"
echo "            ${APP_IMAGE_LATEST}"
echo "  数据镜像: ${DATA_IMAGE}"
echo "============================================"

echo ""
echo "[1/3] 构建应用镜像 ..."
docker build --platform linux/amd64 \
  --build-arg GIT_COMMIT="${GIT_COMMIT}" \
  --build-arg BUILD_TIME="${BUILD_TIME}" \
  -t "$APP_IMAGE" \
  -t "$APP_IMAGE_LATEST" \
  -f Dockerfile \
  --provenance false \
  .

echo ""
echo "[2/3] 构建知识库数据镜像 ..."
if [[ ! -d "internal/knowledge/gz" ]] || [[ -z "$(ls internal/knowledge/gz/*.gz 2>/dev/null)" ]]; then
  echo "  警告: gz 目录为空，跳过数据镜像"
else
  docker build --platform linux/amd64 \
    -t "$DATA_IMAGE" \
    -f Dockerfile.data \
    --provenance false \
    .
fi

echo ""
echo "[3/3] 推送镜像 ..."
echo "  推送: $APP_IMAGE"
docker push "$APP_IMAGE"
echo "  推送: $APP_IMAGE_LATEST"
docker push "$APP_IMAGE_LATEST"

if docker image inspect "$DATA_IMAGE" &>/dev/null; then
  echo "  推送: $DATA_IMAGE"
  docker push "$DATA_IMAGE"
fi

echo ""
echo "============================================"
echo "  打包完成"
echo "  版本标签: ${GIT_TAG}"
echo "============================================"
