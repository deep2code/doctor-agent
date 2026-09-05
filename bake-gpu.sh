#!/usr/bin/env bash
# ============================================================================
# bake-gpu.sh — 把向量化烘焙丢到租的 GPU 服务器上跑，烘完把 qdrant-storage 拉回本机
#
# 流程: upload(代码+gz+模型) -> setup(pip 依赖 + Qdrant) -> bake(CUDA 后台+轮询) -> fetch(打包拉回)
# 本机产物与 bake-local.sh 完全一致: ./qdrant-storage/ (可直接 ./build.sh qdrant 打包)
#
# 用法:
#   GPU_HOST=root@1.2.3.4 ./bake-gpu.sh              # 一条龙: 传文件+装环境+烘焙+取回
#   GPU_HOST=root@1.2.3.4 ./bake-gpu.sh upload       # 分步: 只传文件 (断点续传, 可重跑)
#   GPU_HOST=root@1.2.3.4 ./bake-gpu.sh setup        # 分步: 只装远程依赖 + 启 Qdrant
#   GPU_HOST=root@1.2.3.4 ./bake-gpu.sh bake         # 分步: 启动烘焙 (远程后台, 断 ssh 不死)
#   GPU_HOST=root@1.2.3.4 ./bake-gpu.sh status       # 看远程进度
#   GPU_HOST=root@1.2.3.4 ./bake-gpu.sh log          # tail -f 远程日志 (Ctrl-C 退出)
#   GPU_HOST=root@1.2.3.4 ./bake-gpu.sh fetch        # 烘完后: 停 Qdrant, 打包拉回本机
#   GPU_HOST=root@1.2.3.4 ./bake-gpu.sh stop         # 杀掉远程烘焙进程
#
# 注意: 没有 clean 命令。远程数据不手动删——退租/释放实例时 AutoDL 自动销毁，
#       且它可能是唯一备份 (2026-09-05 事故: clean + build 顺序错误致两份产物全丢)。
#
# 环境变量:
#   默认值已按当前 AutoDL 实例写死 (RTX 3080 Ti, connect.nmb2.seetacloud.com:26790),
#   换实例/续租后改 GPU_HOST / GPU_PORT 即可 (AutoDL 重开会换地址)。
#   GPU_HOST         必填, ssh 目标 (user@host)
#   GPU_PORT=22      ssh 端口
#   GPU_SSH_OPTS     额外 ssh 参数 (如 "-i ~/.ssh/gpu_key -o StrictHostKeyChecking=no")
#   REMOTE_DIR=                      远程工作目录; 留空自动选 (AutoDL 数据盘 /root/autodl-tmp/doctor-agent-bake)
#   MODEL_DIR=./bge-m3-onnx          本地 ONNX fp32 模型目录 (model.onnx + model.onnx_data + tokenizer)
#   MODEL_SOURCE=auto  auto|upload|remote
#                     upload: 上传本地 fp32 模型 (~2.3GB, rsync 断点续传)
#                     remote: GPU 服务器自己从 HF 导出 (国内机器配 HF_ENDPOINT 镜像)
#   GPU_BATCH_SIZE=256               CUDA 批大小
#   HF_ENDPOINT=https://hf-mirror.com  远程 HuggingFace 镜像
#   PIP_INDEX_URL=                   远程 pip 镜像 (如 https://pypi.tuna.tsinghua.edu.cn/simple)
#   GH_PROXY=                        GitHub 加速前缀 (如 https://ghfast.top/), 远程下 qdrant 二进制用
#   QDRANT_VERSION=v1.19.0           与 Dockerfile.qdrant 一致
#   STORAGE_DIR=./qdrant-storage     本机产物目录
#
# 前提: 本机 -> GPU 服务器 ssh 免密已配好 (ssh-copy-id user@host)。
#       bake 后台轮询每 20s ssh 一次, 密码登录无法交互, 必须 key 免密。
#
# 注意:
#   - GPU 上用 fp32 原模型烘焙 (--int8 是 CPU NEON/AVX 优化, GPU 无效)
#   - fp32(GPU 烘) 与 INT8(CPU 本机 embed_server.py 查询) 向量 cos≈1.0, 直接混用无需重烘焙
#   - 远程 docker 优先跑 Qdrant; 没 docker 自动退回官方静态二进制 (GH_PROXY 可加速)
# ============================================================================
set -euo pipefail
cd "$(dirname "$0")"

# ── 参数 ──────────────────────────────────────────────
# 当前 AutoDL 实例 (重开机后地址会变, 去控制台抄新的)
GPU_HOST="${GPU_HOST:-root@connect.nmb2.seetacloud.com}"
GPU_PORT="${GPU_PORT:-26790}"
GPU_SSH_OPTS="${GPU_SSH_OPTS:-}"
REMOTE_DIR="${REMOTE_DIR:-}"   # 留空 = 自动选 (AutoDL 有数据盘 /root/autodl-tmp 就用数据盘)
MODEL_DIR="${MODEL_DIR:-./bge-m3-onnx}"
MODEL_SOURCE="${MODEL_SOURCE:-auto}"
GPU_BATCH_SIZE="${GPU_BATCH_SIZE:-256}"
HF_ENDPOINT="${HF_ENDPOINT:-https://hf-mirror.com}"
PIP_INDEX_URL="${PIP_INDEX_URL:-}"
GH_PROXY="${GH_PROXY:-}"
QDRANT_VERSION="${QDRANT_VERSION:-v1.19.0}"
STORAGE_DIR="${STORAGE_DIR:-./qdrant-storage}"
COLLECTION="medical_knowledge"
QDRANT_PORT="${QDRANT_PORT:-6334}"   # gRPC; HTTP = PORT-1
GZ_DIR="internal/knowledge/gz"
BAKE_LOG="bake.log"
BAKE_RC="bake.rc"
QDRANT_NAME="doctor-agent-bake-gpu"

[[ -n "$GPU_HOST" ]] || { echo "错误: 缺少 GPU_HOST, 用法: GPU_HOST=user@host ./bake-gpu.sh"; exit 1; }

SSH_OPTS=(-p "$GPU_PORT" -o ServerAliveInterval=30 -o ServerAliveCountMax=8 -o BatchMode=yes)
[[ -n "$GPU_SSH_OPTS" ]] && SSH_OPTS+=($GPU_SSH_OPTS)

# 每条远程命令的前导: AutoDL 的 miniconda 只在交互 shell 生效
# (.bashrc 开头 [ -z "$PS1" ] && return), 非交互 ssh 没有 python3/pip3
RSH_PRE='for d in /root/miniconda3/bin /opt/conda/bin /usr/local/bin; do [ -x "$d/python3" ] && export PATH="$d:$PATH" && break; done'

# 远程执行 (单条命令)
rsh() { ssh "${SSH_OPTS[@]}" "$GPU_HOST" "$RSH_PRE; $*"; }
# 上传 stdin tar 流
rsh_tar_in() { ssh "${SSH_OPTS[@]}" "$GPU_HOST" "tar xf - -C $REMOTE_DIR"; }

# 解析远程目录为绝对路径。默认值带 ~, 而 ~ 只在 shell 解析字面量时展开,
# 变量展开结果不再做 tilde expansion (docker -v / 环境变量值会写到字面 "~" 目录)。
resolve_rdir() {
  if [[ -z "$REMOTE_DIR" ]]; then
    REMOTE_DIR=$(rsh 'if [ -d /root/autodl-tmp ]; then echo /root/autodl-tmp/doctor-agent-bake; else echo $HOME/doctor-agent-bake; fi')
  fi
  REMOTE_DIR=$(rsh "mkdir -p $REMOTE_DIR && cd $REMOTE_DIR && pwd")
  echo "[remote] 工作目录: $REMOTE_DIR"
}

# pip 镜像参数 (标量字符串; bash 3.2 空数组 + set -u 会报 unbound variable)
PIP_ARGS=""
[[ -n "$PIP_INDEX_URL" ]] && PIP_ARGS="-i $PIP_INDEX_URL"

# ── 远程烘焙命令 (setsid 后台, 断 ssh 不死; 结束写 bake.rc) ──
remote_bake_cmd() {
cat <<REMOTE
set -e
cd $REMOTE_DIR
mkdir -p qdrant-storage
NVIDIA_DIR=\$(python3 -c "import importlib.util,os; s=importlib.util.find_spec('nvidia'); print(os.path.dirname(s.origin) if s else '')" 2>/dev/null || true)
export LD_LIBRARY_PATH="\${NVIDIA_DIR:+\$NVIDIA_DIR/cudnn/lib:}\${NVIDIA_DIR:+\$NVIDIA_DIR/cublas/lib:}\${LD_LIBRARY_PATH:-}"
rm -f $BAKE_RC
setsid nohup bash -c '
  python3 external/bake_onnx.py \\
    --src=gz --host=localhost --port=$QDRANT_PORT \\
    --collection=$COLLECTION \\
    --model=model/model.onnx --tokenizer=model \\
    --device=cuda --workers=8 --batch-size=$GPU_BATCH_SIZE \\
    --max-text-chars=1024 --recreate \\
    > $BAKE_LOG 2>&1
  echo \$? > $BAKE_RC
' >/dev/null 2>&1 < /dev/null &
echo "烘焙已启动 (PID \$!)"
REMOTE
}

# ── Qdrant 启动 (docker 优先, 二进制兜底) ──
remote_start_qdrant() {
cat <<REMOTE
set -e
cd $REMOTE_DIR
# AutoDL 学术加速 (GitHub/HF 直连慢/断流时必须开)
[ -f /etc/network_turbo ] && source /etc/network_turbo || true
if curl -sf http://localhost:$((QDRANT_PORT-1))/healthz >/dev/null 2>&1; then
  echo "Qdrant 已在运行"
  exit 0
fi
if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  docker rm -f $QDRANT_NAME 2>/dev/null || true
  mkdir -p qdrant-storage
  docker run -d --name $QDRANT_NAME \\
    -p ${QDRANT_PORT}:6334 -p $((QDRANT_PORT-1)):6333 \\
    -v $REMOTE_DIR/qdrant-storage:/qdrant/storage \\
    qdrant/qdrant:$QDRANT_VERSION
else
  if [[ ! -x ./qdrant ]]; then
    echo "无 docker, 下载 qdrant 二进制 $QDRANT_VERSION (musl 静态构建, 不依赖系统 glibc)..."
    curl -fsSL --retry 3 --connect-timeout 15 "${GH_PROXY}https://github.com/qdrant/qdrant/releases/download/$QDRANT_VERSION/qdrant-x86_64-unknown-linux-musl.tar.gz" | tar xz
  fi
  mkdir -p qdrant-storage
  QDRANT__STORAGE__STORAGE_PATH=$REMOTE_DIR/qdrant-storage \\
    setsid nohup ./qdrant > qdrant.log 2>&1 < /dev/null &
fi
for i in \$(seq 1 60); do
  if curl -sf http://localhost:$((QDRANT_PORT-1))/healthz >/dev/null 2>&1; then
    echo "Qdrant 就绪 (\${i}s)"; exit 0
  fi
  sleep 1
done
echo "错误: Qdrant 60s 未就绪"; docker logs $QDRANT_NAME 2>/dev/null | tail -20; tail -20 qdrant.log 2>/dev/null; exit 1
REMOTE
}

# ── 前置检查 ──
check_host() {
  echo "[check] 远程环境..."
  rsh 'echo "  host:  $(uname -srm)"; python3 -V 2>/dev/null || echo "  警告: 无 python3"; nvidia-smi --query-gpu=name,memory.total --format=csv,noheader 2>/dev/null | head -1 | sed "s/^/  gpu:   /" || echo "  警告: nvidia-smi 不可用"'
}

# ── upload ──
do_upload() {
  echo "[upload] 代码 + gz 数据 (${GZ_DIR}, 87MB)..."
  rsh "mkdir -p $REMOTE_DIR/external $REMOTE_DIR/gz"
  tar cf - external/bake_onnx.py external/export_onnx.py | rsh_tar_in
  tar cf - -C "$GZ_DIR" . | ssh "${SSH_OPTS[@]}" "$GPU_HOST" "tar xf - -C $REMOTE_DIR/gz"
  echo "  gz 完成: $(ls "$GZ_DIR"/*.json.*z* | wc -l | tr -d ' ') 个文件"

  # 模型来源决策
  if [[ "$MODEL_SOURCE" == "auto" ]]; then
    if [[ -f "$MODEL_DIR/model.onnx_data" ]] || [[ -f "$MODEL_DIR/model.onnx" && $(stat -f%z "$MODEL_DIR/model.onnx" 2>/dev/null || stat -c%s "$MODEL_DIR/model.onnx" 2>/dev/null || echo 0) -gt 100000000 ]]; then
      MODEL_SOURCE="upload"
    else
      MODEL_SOURCE="remote"
    fi
  fi

  if [[ "$MODEL_SOURCE" == "upload" ]]; then
    [[ -f "$MODEL_DIR/model.onnx" ]] || { echo "错误: $MODEL_DIR/model.onnx 不存在"; exit 1; }
    echo "[upload] fp32 模型 (~2.3GB, rsync 断点续传, 中断后重跑本命令继续)..."
    rsh "mkdir -p $REMOTE_DIR/model"
    RSYNC_EXCLUDE_STR=""
    [[ -f "$MODEL_DIR/model.int8.onnx" ]] && RSYNC_EXCLUDE_STR="--exclude=model.int8.onnx"
    if command -v rsync >/dev/null 2>&1; then
      if ! rsync -azP $RSYNC_EXCLUDE_STR -e "ssh ${SSH_OPTS[*]}" "$MODEL_DIR/" "$GPU_HOST:$REMOTE_DIR/model/" 2>/tmp/bake-gpu-rsync.log; then
        if grep -q "rsync.*not found\|command not found" /tmp/bake-gpu-rsync.log 2>/dev/null; then
          echo "  远程无 rsync, 改 tar 直传 (不可续传)..."
          tar cf - -C "$MODEL_DIR" $RSYNC_EXCLUDE_STR . | rsh_tar_in_cd_model
        else
          cat /tmp/bake-gpu-rsync.log; exit 1
        fi
      fi
    else
      echo "  本机无 rsync, 改 tar 直传 (不可续传)..."
      tar cf - -C "$MODEL_DIR" $RSYNC_EXCLUDE_STR . | rsh_tar_in_cd_model
    fi
    echo "  模型上传完成"
  else
    echo "[upload] MODEL_SOURCE=remote: GPU 服务器自行从 HF 导出模型..."
    rsh "cd $REMOTE_DIR && mkdir -p model && HF_ENDPOINT=$HF_ENDPOINT pip3 install -q $PIP_ARGS optimum-onnx onnxruntime transformers torch && HF_ENDPOINT=$HF_ENDPOINT python3 external/export_onnx.py --out model && ls -lh model/model.onnx*"
  fi
  echo "  上传完成"
}

rsh_tar_in_cd_model() {
  ssh "${SSH_OPTS[@]}" "$GPU_HOST" "mkdir -p $REMOTE_DIR/model && tar xf - -C $REMOTE_DIR/model"
}

# ── setup ──
do_setup() {
  echo "[setup] pip 依赖 (onnxruntime-gpu transformers qdrant-client zstandard numpy)..."
  rsh "pip3 install -q $PIP_ARGS onnxruntime-gpu transformers qdrant-client zstandard numpy"
  echo "[setup] 验证 CUDA EP..."
  if ! rsh 'python3 -c "
import onnxruntime as o
ps = o.get_available_providers()
print(\"  providers:\", ps)
assert \"CUDAExecutionProvider\" in ps, \"CUDAExecutionProvider 缺失\"
"'; then
    echo "错误: CUDA EP 不可用。检查: nvidia-smi / pip install onnxruntime-gpu / CUDA+cuDNN 版本"
    echo "  (pip 装的 ORT 需要 cuDNN: pip install nvidia-cudnn-cu12 nvidia-cublas-cu12, 脚本 bake 时会自动加 LD_LIBRARY_PATH)"
    exit 1
  fi
  echo "[setup] 启动 Qdrant..."
  rsh "$(remote_start_qdrant)"
  echo "  setup 完成"
}

# ── bake ──
do_bake() {
  # 确保 Qdrant 在跑
  rsh "$(remote_start_qdrant)" >/dev/null
  echo "[bake] 启动远程烘焙 (batch=$GPU_BATCH_SIZE, 后台)..."
  rsh "$(remote_bake_cmd)"
  echo ""
  echo "轮询进度 (每 20s, Ctrl-C 退出不影响远程; 之后可用 ./bake-gpu.sh status / log / fetch)..."
  local rc=""
  while true; do
    sleep 20
    rc=$(rsh "cat $REMOTE_DIR/$BAKE_RC 2>/dev/null || true" || true)
    if [[ -n "$rc" ]]; then
      echo ""
      if [[ "$rc" == "0" ]]; then
        echo "✅ 远程烘焙完成 (耗时见日志)"
        do_fetch
      else
        echo "❌ 远程烘焙失败 (exit $rc), 末尾日志:"
        rsh "tail -30 $REMOTE_DIR/$BAKE_LOG"
        exit 1
      fi
      return
    fi
    # 进度行: 最新一行日志
    rsh "tail -n 1 $REMOTE_DIR/$BAKE_LOG 2>/dev/null" || true
  done
}

# ── status / log / stop ──
do_status() {
  echo "=== 远程状态 ==="
  rsh "cd $REMOTE_DIR 2>/dev/null && { tail -n 8 $BAKE_LOG 2>/dev/null; echo '---'; if [[ -f $BAKE_RC ]]; then echo \"烘焙已结束 (exit \$(cat $BAKE_RC))\"; else echo '烘焙进行中或未启动'; pgrep -af '[b]ake_onnx.py' | head -2 || true; fi; } || echo '远程目录不存在'"
}
do_log() {
  rsh "tail -f -n 30 $REMOTE_DIR/$BAKE_LOG"
}
do_stop() {
  rsh "pkill -f bake_onnx.py 2>/dev/null && echo 已停止 || echo 无进程"
}

# ── fetch ──
do_fetch() {
  echo "[fetch] 停 Qdrant, 固化 storage..."
  rsh "cd $REMOTE_DIR && { docker rm -f $QDRANT_NAME 2>/dev/null || pkill -x qdrant 2>/dev/null || true; sleep 2; find qdrant-storage -path '*/wal/*' -type f -delete 2>/dev/null || true; tar czf bundle.tgz qdrant-storage; du -sh bundle.tgz; }"
  echo "[fetch] 拉回本机 $STORAGE_DIR ..."
  rm -rf "$STORAGE_DIR"
  mkdir -p "$STORAGE_DIR"
  scp -P "$GPU_PORT" ${GPU_SSH_OPTS:+$GPU_SSH_OPTS} "$GPU_HOST:$REMOTE_DIR/bundle.tgz" /tmp/bake-gpu-bundle.tgz
  tar xzf /tmp/bake-gpu-bundle.tgz -C "$STORAGE_DIR" --strip-components=1
  rm -f /tmp/bake-gpu-bundle.tgz
  local size segs
  size=$(du -sh "$STORAGE_DIR" | cut -f1)
  segs=$(find "$STORAGE_DIR" -name "*.seg" -o -name "*.idx" | wc -l | tr -d ' ')
  echo ""
  echo "============================================"
  echo "  ✅ GPU 烘焙产物已就位: $STORAGE_DIR ($size, $segs 个 segment 文件)"
  echo "  下一步: ./build.sh qdrant   (Dockerfile.qdrant.slim 打包)"
  echo "  退租前清理: ./bake-gpu.sh clean"
  echo "============================================"
}

# ── clean ──
# 已移除: 远程数据不手动删 (退租时 AutoDL 自动销毁, 且可能是唯一备份)

# ── 入口 ──
PHASE="${1:-all}"

case "$PHASE" in
  upload) resolve_rdir; do_upload ;;
  setup)  resolve_rdir; do_setup ;;
  bake)   resolve_rdir; do_bake ;;
  status) resolve_rdir; do_status ;;
  log)    resolve_rdir; do_log ;;
  stop)   do_stop ;;
  fetch)  resolve_rdir; do_fetch ;;
  clean)
    echo "已移除 clean 命令 (2026-09-05 产物两份全丢事故)。"
    echo "远程数据不用手动删: 退租/释放实例时 AutoDL 自动销毁。"
    exit 1
    ;;
  all)
    check_host
    resolve_rdir
    do_upload
    do_setup
    do_bake
    ;;
  *)
    echo "未知命令: $PHASE (支持: all|upload|setup|bake|status|log|fetch|stop|clean)"
    exit 1
    ;;
esac
