#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

# ============================================================================
# 一条命令更新医学知识库并发布数据镜像:
#   ./update-kb.sh
#
# 做四件事:
#   1. 把 internal/knowledge/data (改过的话先跑 python3 external/make_gz.py)
#      灌进本地 MariaDB 测试容器 (3307)
#   2. 校验 doctor_knowledge 里只剩 kb_items —— 业务库表 (users/sessions/
#      family_members…) 一旦混进来, 就会被下一步打进 doctor-agent-kb 发布镜像,
#      所有部署首启都会带着它们; 空表自动清掉, 非空直接失败 (见下)
#   3. 全量导出 doctor_knowledge → docker/kb/init-doctor_knowledge.sql.gz
#   4. ./build.sh kb  → 打镜像 + 推 ACR (doctor-agent-kb:latest，不打版本号标签)
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

echo "[1/4] 灌本地知识库 ($CONTAINER:$PORT)..."
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

docker exec "$CONTAINER" mariadb -uroot -e \
	"CREATE DATABASE IF NOT EXISTS doctor_knowledge CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"

KNOWLEDGE_DB_DSN="root@tcp(127.0.0.1:${PORT})/doctor_knowledge?parseTime=true&interpolateParams=true" \
  go run ./cmd/kbseed

echo "[2/4] 校验知识库表结构 (只应有 kb_items)..."
# 为什么这道门必须存在: internal/database.New() 一连上就建整套业务表, 历史上有
# 测试把 MariaDB 测试库指到 doctor_knowledge, 于是 users/sessions/family_members
# 混进了知识库, 下一步的 --databases 全量导出会把它们一起打进 doctor-agent-kb,
# 每个部署首启都带上。空表删掉即可; 有行的一律拒绝发布 —— 那可能是真数据。
KB_DB=doctor_knowledge
stray="$(docker exec "$CONTAINER" mariadb -uroot -N -e \
  "SELECT table_name FROM information_schema.tables WHERE table_schema='${KB_DB}' AND table_name<>'kb_items' ORDER BY table_name")"
if [ -n "$stray" ]; then
  echo "  发现外来表:"
  for tb in $stray; do
    rows="$(docker exec "$CONTAINER" mariadb -uroot -N -e "SELECT COUNT(*) FROM \`${KB_DB}\`.\`${tb}\`")"
    if [ "$rows" != "0" ]; then
      echo "错误: ${KB_DB}.${tb} 有 ${rows} 行业务数据, 不能打进知识镜像, 也不敢替你删。"
      echo "      先查是不是 MARIA_DB_APP_DB 被指到了 ${KB_DB}, 把数据迁回业务库后重跑。"
      exit 1
    fi
    docker exec "$CONTAINER" mariadb -uroot -e "DROP TABLE \`${KB_DB}\`.\`${tb}\`"
    echo "    已清掉空表 $tb"
  done
fi

echo "[3/4] 导出全量 dump..."
mkdir -p docker/kb
docker exec "$CONTAINER" mariadb-dump -uroot --quick --single-transaction --hex-blob \
  --default-character-set=utf8mb4 --databases doctor_knowledge 2>/dev/null \
  | gzip > "$DUMP"
echo "  dump: $(ls -lh "$DUMP" | awk '{print $5}')"

echo "[4/4] 打包推送 kb 镜像..."
./build.sh kb

echo ""
echo "完成: doctor-agent-kb 已更新并推送到 ACR。"
echo "  已部署环境升级: docker compose pull mariadb && docker compose up -d --force-recreate mariadb"
echo "  (注意: 旧 volume 有数据时 init 不重跑, 需 down -v 清卷一次)"
