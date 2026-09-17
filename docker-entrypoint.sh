#!/bin/sh
set -e

# doctor-agent container entrypoint:
#   1. wait for MariaDB (business DB) and optionally Qdrant (RAG)
#   2. OPTIONAL: seed MariaDB knowledge store (only when KNOWLEDGE_SRC has data;
#      the RAG image architecture bakes knowledge into Qdrant at build time)
#   3. OPTIONAL: sync vector store (best-effort; non-fatal)
#   4. start the HTTP server
#
# 架构（2026-08-29）：MariaDB 只做业务，Qdrant 做专业 RAG。
# 知识数据在 doctor-agent-qdrant 镜像构建期已通过 vector-bake 注入，
# 所以默认不需要 seed/sync。若要 MariaDB 关键词兜底，挂载含 gz 的卷到
# KNOWLEDGE_SRC 并设置 SEED_MARIADB_KB=true。

APP_BIN="${APP_BIN:-doctor-agent}"
KNOWLEDGE_SRC="${KNOWLEDGE_SRC:-/opt/knowledge/gz}"
SEED_MARIADB_KB="${SEED_MARIADB_KB:-false}"

MARIA_DB_HOST="${MARIA_DB_HOST:-mariadb}"
MARIA_DB_PORT="${MARIA_DB_PORT:-3306}"
MARIA_DB_USER="${MARIA_DB_USER:-root}"
MARIA_DB_PASSWORD="${MARIA_DB_PASSWORD:-changeme}"
MARIA_DB_KNOWLEDGE_DB="${MARIA_DB_KNOWLEDGE_DB:-doctor_knowledge}"

VECTOR_STORE_ENABLED="${VECTOR_STORE_ENABLED:-true}"

echo "== doctor-agent entrypoint =="

wait_for() {
  host="$1"; port="$2"; svc="$3"
  echo "Waiting for $svc at $host:$port ..."
  i=0
  until nc -z "$host" "$port" 2>/dev/null; do
    i=$((i+1))
    if [ "$i" -ge 60 ]; then
      echo "ERROR: $svc did not become reachable after 60 tries; aborting."
      exit 1
    fi
    sleep 1
  done
  echo "$svc is up."
}

# MariaDB is required (business DB: users/sessions/messages/feedback).
wait_for "$MARIA_DB_HOST" "$MARIA_DB_PORT" "MariaDB"

# Qdrant is required for RAG in this architecture; still tolerate absence by
# degrading to keyword retrieval when the operator explicitly disables it.
if echo "$VECTOR_STORE_ENABLED" | grep -qiE "^(1|true|yes|on)$"; then
  if nc -z "${QDRANT_HOST:-qdrant}" "${QDRANT_PORT:-6333}" 2>/dev/null; then
    echo "Qdrant detected; RAG retrieval will be enabled."
  else
    echo "WARN: Qdrant not reachable; vector retrieval disabled (keyword-only)."
    export VECTOR_STORE_ENABLED=false
  fi
else
  echo "Vector store disabled by VECTOR_STORE_ENABLED=$VECTOR_STORE_ENABLED"
fi

# --- OPTIONAL: seed MariaDB knowledge store (keyword fallback) ---
# Default OFF: knowledge lives in the Qdrant data image (baked at build time).
# Enable with SEED_MARIADB_KB=true when you mount a gz volume to KNOWLEDGE_SRC.
if echo "$SEED_MARIADB_KB" | grep -qiE "^(1|true|yes|on)$"; then
  if [ -d "$KNOWLEDGE_SRC" ] && ls "$KNOWLEDGE_SRC"/*.json.*z* >/dev/null 2>&1; then
    SEED_SKIP=0
    CNT=""
    if command -v mysql >/dev/null 2>&1; then
      PW_ARG=""
      if [ -n "$MARIA_DB_PASSWORD" ]; then PW_ARG="-p$MARIA_DB_PASSWORD"; fi
      CNT=$(mysql -h"$MARIA_DB_HOST" -P"$MARIA_DB_PORT" -u"$MARIA_DB_USER" $PW_ARG -N -e \
        "SELECT COUNT(*) FROM kb_items WHERE dataset='medicalqa';" "$MARIA_DB_KNOWLEDGE_DB" 2>/dev/null || true)
      if [ "$CNT" != "" ] && [ "$CNT" -gt 0 ] 2>/dev/null; then
        SEED_SKIP=1
      fi
    fi

    if [ "$SEED_SKIP" = "1" ]; then
      echo "Knowledge store already seeded ($CNT rows); skipping seed-knowledge."
    else
      echo "Seeding knowledge store (MariaDB, keyword fallback) from $KNOWLEDGE_SRC ..."
      "$APP_BIN" seed-knowledge --src="$KNOWLEDGE_SRC" \
        || echo "WARN: knowledge seeding failed (server will still start)"
    fi
  else
    echo "SEED_MARIADB_KB=true but no gz files at $KNOWLEDGE_SRC; skipping seed."
  fi
else
  echo "SEED_MARIADB_KB=false (default): knowledge served from Qdrant RAG image."
fi

# --- OPTIONAL: sync vector store (best-effort, only when enabled) ---
# Not needed with the baked RAG image; kept for manual/override scenarios.
if echo "$VECTOR_STORE_ENABLED" | grep -qiE "^(1|true|yes|on)$" \
   && echo "$SEED_MARIADB_KB" | grep -qiE "^(1|true|yes|on)$"; then
  echo "Syncing vector store (best-effort) ..."
  "$APP_BIN" sync-knowledge --full \
    || echo "WARN: vector sync failed; continuing with Qdrant RAG image data"
fi

echo "Starting doctor-agent server ..."
exec "$APP_BIN" serve
