#!/usr/bin/env bash
# 构建知识库 RAG 镜像 doctor-agent-qdrant（docker/qdrant-context 独立 context）
#
# 架构（2026-08-30）：
#   只有两个镜像：
#     doctor-agent-qdrant   纯 gz 知识库(alpine 51 数据集) + 标准 Qdrant + 向量
#                           → 只有知识库变化才更新
#     doctor-agent          Go 源码 + 前端 → 只有代码变化才更新
#   doctor-agent-data 已废弃，gz 数据合并进本镜像。
#
# 为什么独立 context：项目根 .dockerignore 排除了 internal/knowledge/gz
# （为了 app 镜像瘦身）。若本镜像也从项目根构建，gz 会被 dockerignore 吞掉。
# 独立 context = 构建上下文里只有 gz + src + 本 Dockerfile。
#
# 烘焙工具 = ./cmd/vector-bake（独立于业务代码，2026-08-30 分离）：
#   本脚本把 cmd/vector-bake 及其依赖包源码同步进 context 的 src/，
#   Dockerfile 在构建期自行编译执行。不依赖 doctor-agent app 镜像。
#
# 用法:
#   ./docker/qdrant-context/build.sh [镜像名]
#     默认: doctor-agent-qdrant:latest
set -euo pipefail
cd "$(dirname "$0")"

IMAGE="${1:-doctor-agent-qdrant:latest}"
REPO_ROOT="$(cd ../.. && pwd)"

echo "== 同步 gz 到独立 context =="
rm -rf gz
cp -a "$REPO_ROOT/internal/knowledge/gz" gz
CNT=$(ls gz/*.json.*z* 2>/dev/null | wc -l | tr -d ' ')
if [ "$CNT" -eq 0 ]; then
  echo "ERROR: 没有找到知识库压缩文件（internal/knowledge/gz 为空）"
  exit 1
fi
echo "  已同步 $CNT 个知识库压缩数据集（.json.gz / .json.zst）"

echo "== 同步烘焙工具源码到独立 context (src/) =="
# 依赖闭包由 go list 自动推导：cmd/vector-bake + 其引用的本模块内部包，
# 新增 import 无需手动维护此列表。
PKGS=$(cd "$REPO_ROOT" && go list -deps -f \
  '{{if .Module}}{{if eq .Module.Path "github.com/doctor-agent"}}{{.Dir}}{{end}}{{end}}' \
  ./cmd/vector-bake)
rm -rf src
mkdir -p src
cp "$REPO_ROOT/go.mod" "$REPO_ROOT/go.sum" src/
for d in $PKGS; do
  rel="${d#"$REPO_ROOT"/}"
  mkdir -p "src/$(dirname "$rel")"
  cp -a "$d" "src/$rel"
done
# internal/knowledge 自带的 gz 数据子目录不进 src（数据走 datalayer，瘦身 context）
rm -rf src/internal/knowledge/gz
echo "  已同步: cmd/vector-bake + $(echo "$PKGS" | grep -c internal) 个依赖包"

echo "== 构建 RAG 镜像: $IMAGE =="
docker build --platform linux/amd64 \
  -t "$IMAGE" \
  -f Dockerfile \
  --provenance false \
  .

echo "== 完成: $IMAGE =="
echo "  包含: 纯 gz 知识库(51 数据集) + 标准 Qdrant + 烘好的向量"
echo "  依赖: ./cmd/vector-bake（独立烘焙工具，不依赖 app 镜像）"
echo "  触发: 只有知识库变化时重建本镜像"
