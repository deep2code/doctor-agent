#!/bin/sh
set -e

# doctor-agent container entrypoint:
#   1. wait for MariaDB (and optionally Qdrant)
#   2. seed knowledge store into MariaDB (skip if already populated)
#   3. sync vector store (best-effort; non-fatal)
#   4. start the HTTP server

APP_BIN="${APP_BIN:-doctor-agent}"
KNOWLEDGE_SRC="${KNOWLEDGE_SRC:-/opt/knowledge/gz}"

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

# MariaDB is required.
wait_for "$MARIA_DB_HOST" "$MARIA_DB_PORT" "MariaDB"

# Qdrant is optional (vector retrieval degrades to keyword if absent).
if echo "$VECTOR_STORE_ENABLED" | grep -qiE "^(1|true|yes|on)$"; then
  if nc -z "${QDRANT_HOST:-qdrant}" "${QDRANT_PORT:-6333}" 2>/dev/null; then
    echo "Qdrant detected; vector retrieval will be enabled."
  else
    echo "WARN: Qdrant not reachable; vector retrieval disabled (keyword-only)."
    export VECTOR_STORE_ENABLED=false
  fi
else
  echo "Vector store disabled by VECTOR_STORE_ENABLED=$VECTOR_STORE_ENABLED"
fi

# --- Seed knowledge store (MariaDB; idempotent upsert via ON DUPLICATE KEY) ---
# Skip only when the store is COMPLETE. A bare COUNT(*) is not enough: an
# interrupted first seed can leave only small tables populated (e.g. 842 rows
# of medical/drug) while the big ones (medicalqa/nmpa/icd10/cpubmed…) are
# missing, and then every restart would "skip" seeding forever. We therefore
# require a real marker: the medicalqa dataset (the largest, ~506k rows).
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
  echo "Seeding knowledge store (MariaDB) from $KNOWLEDGE_SRC ..."
  "$APP_BIN" seed-knowledge --src="$KNOWLEDGE_SRC" \
    || echo "WARN: knowledge seeding failed (server will still start)"
fi

# --- Sync vector store (best-effort) ---
if echo "$VECTOR_STORE_ENABLED" | grep -qiE "^(1|true|yes|on)$"; then
  echo "Syncing vector store (best-effort) ..."
  "$APP_BIN" sync-knowledge --full \
    || echo "WARN: vector sync failed; continuing with keyword retrieval"
fi

echo "Starting doctor-agent server ..."
exec "$APP_BIN" serve
