#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

APP_IMAGE="crpi-0xi5k79l9j4opzta.cn-hangzhou.personal.cr.aliyuncs.com/codeup2026/doctor-agent:latest"
DATA_IMAGE="crpi-0xi5k79l9j4opzta.cn-hangzhou.personal.cr.aliyuncs.com/codeup2026/doctor-agent-data:latest"

# Linux 机器走内网推送
if [[ "$(uname -s)" == "Linux" ]]; then
  APP_IMAGE="crpi-0xi5k79l9j4opzta-vpc.cn-hangzhou.personal.cr.aliyuncs.com/codeup2026/doctor-agent:latest"
  DATA_IMAGE="crpi-0xi5k79l9j4opzta-vpc.cn-hangzhou.personal.cr.aliyuncs.com/codeup2026/doctor-agent-data:latest"
fi

echo "============================================"
echo "  doctor-agent 打包"
echo "  应用镜像 : $APP_IMAGE"
echo "  数据镜像 : $DATA_IMAGE"
echo "============================================"

echo ""
echo "[1/3] 构建应用镜像 ..."
docker build --platform linux/amd64 \
  -t "$APP_IMAGE" \
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

if docker image inspect "$DATA_IMAGE" &>/dev/null; then
  echo "  推送: $DATA_IMAGE"
  docker push "$DATA_IMAGE"
fi

echo ""
echo "============================================"
echo "  打包完成"
echo "============================================"
