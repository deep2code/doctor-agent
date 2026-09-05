#!/usr/bin/env bash
# ============================================================================
# bake-fast.sh —— 一键加速烘焙（封装 build.sh qdrant 的前置准备）
#
# 自动做 4 件事（全部幂等，重复跑无副作用）:
#   1. 杀掉所有旧的 qdrant docker build（自动，无确认）
#   2. 保证 Ollama 带 OLLAMA_FLASH_ATTENTION=1 + OLLAMA_KEEP_ALIVE=-1 运行
#      （若是你自己 brew services 起的，会先停掉换成托管实例；
#       构建结束后托管实例也会被杀掉 —— 全部退出，不留后台进程）
#   3. 确保 bge-m3:q8_0 量化版存在（首次创建，之后复用；不支持量化则自动回退 bge-m3）
#   4. EMBEDDING_MODEL=… ./build.sh qdrant
#
# 用法:
#   ./bake-fast.sh               常规加速烘焙
#   ./bake-fast.sh --recreate    完全从零（烘焙前删除 collection 重新灌）
# ============================================================================
set -euo pipefail
cd "$(dirname "$0")"

if [[ "${1:-}" == "--recreate" ]]; then
  export BAKE_RECREATE=1
fi

OLLAMA_LOG="logs/ollama.log"
OLLAMA_PIDFILE="logs/ollama.pid"
mkdir -p logs

ollama_up() { curl -s -m 2 http://127.0.0.1:11434/api/tags >/dev/null 2>&1; }

# ── 1. 旧构建检测 ────────────────────────────────────────────────────────────
# ── 1. 杀掉一切旧的 qdrant 构建（自动，无确认）────────────────────────────
for OLD_BUILD in $(pgrep -f "docker build.*Dockerfile.qdrant" || true); do
  echo "• 杀掉旧构建 pid $OLD_BUILD"
  kill "$OLD_BUILD" 2>/dev/null || true
done
# 等待退出，避免与本次构建抢 Ollama / docker
for i in $(seq 1 10); do
  pgrep -f "docker build.*Dockerfile.qdrant" >/dev/null || break
  sleep 1
done
if pgrep -f "docker build.*Dockerfile.qdrant" >/dev/null; then
  echo "• 旧构建未退出，强制 kill -9"
  pkill -9 -f "docker build.*Dockerfile.qdrant" || true
  sleep 2
fi

# ── 2. 托管 Ollama（flash attention + 常驻显存）────────────────────────────
MANAGED_PID=$(cat "$OLLAMA_PIDFILE" 2>/dev/null || true)
if [[ -n "$MANAGED_PID" ]] && kill -0 "$MANAGED_PID" 2>/dev/null && ollama_up; then
  echo "✓ Ollama 托管实例已在运行 (pid $MANAGED_PID, flash attention 已开)"
else
  # 托管实例不在：若是 brew services 的 Ollama 在跑，停掉它（它没带加速 env）
  if ollama_up; then
    echo "• 当前 Ollama 是外部启动的（无 flash attention），切换为托管实例..."
    brew services stop ollama >/dev/null 2>&1 || true
    pkill -f "ollama serve" 2>/dev/null || true
    sleep 2
  fi
  echo "• 启动托管 Ollama (OLLAMA_FLASH_ATTENTION=1 OLLAMA_KEEP_ALIVE=-1)..."
  OLLAMA_FLASH_ATTENTION=1 OLLAMA_KEEP_ALIVE=-1 \
    nohup ollama serve >>"$OLLAMA_LOG" 2>&1 &
  echo $! > "$OLLAMA_PIDFILE"
  for i in $(seq 1 30); do
    ollama_up && break
    sleep 1
  done
  ollama_up || { echo "❌ Ollama 启动失败，看 $OLLAMA_LOG"; exit 1; }
  echo "✓ Ollama 托管实例已启动 (pid $(cat "$OLLAMA_PIDFILE"))"
fi

# ── 3. 量化模型（首次创建，之后复用）───────────────────────────────────────
EMBED_MODEL="${EMBEDDING_MODEL:-}"
if [[ -z "$EMBED_MODEL" ]]; then
  if ollama list 2>/dev/null | grep -q "bge-m3:q8_0"; then
    EMBED_MODEL="bge-m3:q8_0"
    echo "✓ 使用已有量化模型 $EMBED_MODEL"
  else
    echo "• bge-m3:q8_0 不存在，尝试创建量化副本（一次性，约 1-2 分钟）..."
    printf 'FROM bge-m3\n' > /tmp/bake-mf
    if ollama create bge-m3:q8_0 -f /tmp/bake-mf -q q8_0 >/dev/null 2>&1; then
      EMBED_MODEL="bge-m3:q8_0"
      echo "✓ 量化模型创建成功: $EMBED_MODEL"
    else
      EMBED_MODEL="bge-m3"
      echo "⚠️  量化不支持，回退 $EMBED_MODEL（仅 flash attention 加速）"
    fi
  fi
else
  echo "✓ 使用指定模型 $EMBED_MODEL"
fi

# ── 4. 烘焙 ────────────────────────────────────────────────────────────────
echo ""
echo "============================================"
echo "  开始烘焙 (EMBEDDING_MODEL=$EMBED_MODEL)"
echo "============================================"

# 构建结束后杀掉托管 Ollama —— 全部退出，不留后台进程
cleanup() {
  MPID=$(cat "$OLLAMA_PIDFILE" 2>/dev/null || true)
  if [[ -n "$MPID" ]] && kill -0 "$MPID" 2>/dev/null; then
    echo "• 停止托管 Ollama (pid $MPID)"
    kill "$MPID" 2>/dev/null || true
  fi
  rm -f "$OLLAMA_PIDFILE"
}
trap cleanup EXIT

EMBEDDING_MODEL="$EMBED_MODEL" ./build.sh qdrant