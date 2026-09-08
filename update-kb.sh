#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

# ============================================================================
# 一条命令更新医学知识库并发布数据镜像:
#   ./update-kb.sh
#
# 做三件事:
#   1. 把 internal/knowledge/data (改过的话先跑 python3 external/make_gz.py)
#      灌进本地 MariaDB 测试容器 (3307)
#   2. 全量导出 doctor_knowledge → docker/kb/init-doctor_knowledge.sql.gz
#   3. ./build.sh kb  → 打镜像 + 推 ACR (doctor-agent-kb:版本/latest)
#
# 前置: ACR 已 docker login; 容器不存在时脚本自动起 (mariadb:11.4, 空密码).
# ============================================================================

CONTAINER="${KB_CONTAINER:-doctor-kb-test}"
PORT="${KB_PORT:-3307}"
DUMP="docker/kb/init-doctor_knowledge.sql.gz"

# go 常不在非交互 shell PATH, 同 build.sh 的探测逻辑
if ! command -v go >/dev/null 2>&1; then
  for d in /usr/local/go/bin "$HOME/go/bin" /usr/lib/go/bin /usr/local/bin "$HOME/.local/bin"; do
    [ -x "$d/go" ] && export PATH="$d:$PATH" && break
  done
fi
command -v go >/dev/null 2>&1 || { echo "错误: 找不到 go"; exit 1; }

echo "[1/3] 灌本地知识库 ($CONTAINER:$PORT)..."
if ! docker ps --format '{{.Names}}' | grep -qx "$CONTAINER"; then
  echo "  容器不存在, 起新空库..."
  docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
  docker run -d --name "$CONTAINER" -p "$PORT:3306" \
    -e MARIADB_ALLOW_EMPTY_ROOT_PASSWORD=1 mariadb:11.4 >/dev/null
fi
for i in $(seq 1 30); do
  docker exec "$CONTAINER" mariadb -uroot -e "SELECT 1" >/dev/null 2>&1 && break
  [ "$i" = 30 ] && { echo "错误: 数据库 60s 未就绪"; exit 1; }
  sleep 2
done
KNOWLEDGE_DB_DSN="root@tcp(127.0.0.1:${PORT})/doctor_knowledge?parseTime=true&interpolateParams=true" \
  go run ./cmd/kbseed

echo "[2/3] 导出全量 dump..."
mkdir -p docker/kb
docker exec "$CONTAINER" mariadb-dump -uroot --quick --single-transaction --hex-blob \
  --default-character-set=utf8mb4 --databases doctor_knowledge 2>/dev/null \
  | gzip > "$DUMP"
echo "  dump: $(ls -lh "$DUMP" | awk '{print $5}')"

echo "[3/3] 打包推送 kb 镜像..."
./build.sh kb

echo ""
echo "完成: doctor-agent-kb 已更新并推送到 ACR。"
echo "  已部署环境升级: docker compose pull mariadb && docker compose up -d --force-recreate mariadb"
echo "  (注意: 旧 volume 有数据时 init 不重跑, 需 down -v 清卷一次)"
