#!/usr/bin/env bash
# ============================================================================
# bake-gpu.sh — 把向量化烘焙丢到租的 GPU 服务器上跑，烘完把 qdrant-storage 拉回本机
#
# 流程: upload(代码+gz+模型) -> setup(pip 依赖 + Qdrant) -> bake(CUDA 后台+轮询) -> fetch(点数门+优雅停服+打包拉回)
# 本机产物与 bake-local.sh 完全一致: ./qdrant-storage/ (可直接 ./build.sh qdrant 打包)
#
# 用法:
#   GPU_HOST=root@1.2.3.4 ./bake-gpu.sh              # 一条龙: 传文件+装环境+烘焙+取回
#   GPU_HOST=root@1.2.3.4 ./bake-gpu.sh upload       # 分步: 只传文件 (断点续传, 可重跑)
#   GPU_HOST=root@1.2.3.4 ./bake-gpu.sh setup        # 分步: 只装远程依赖 + 启 Qdrant
#   GPU_HOST=root@1.2.3.4 ./bake-gpu.sh bake         # 分步: 启动烘焙 (远程后台, 断 ssh 不死)
#   GPU_RECREATE=0 GPU_HOST=... ./bake-gpu.sh bake   # 中途失败后接着烘 (不清库, 已烘的不再 embed)
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
#   GPU_BATCH_SIZE=24                CUDA 批大小。2026-10-06 在 3080 Ti 12G 上实测:
#                                    48 会在「最长文本那一尾批」CUDA OOM (单层 attention 缓冲要
#                                    3.07GB, 报 Failed to allocate ... 3072000000, 整批 48 行没写进去);
#                                    24 全量抽样 0 错误, 只比 48 慢 3.6% (126 vs 131 行/秒)
#   GPU_RECREATE=1                   1=每次先清空 collection 从零烘 (默认);
#                                    0=断点续烘: 保住已有向量, 库里已有的行不再 embed
#                                    (中途失败后接着跑用这个, 见「注意」第 ③ 条)
#   EXPECTED_POINTS=                 期望向量条数; 留空 = 烘之前从本地 gz/ 现算
#                                    (用远端同一套 bake_onnx 规则扫目录, 所以数据一变它就跟着变,
#                                     不是又一份需要手动维护的登记数字)
#   HF_ENDPOINT=https://hf-mirror.com  远程 HuggingFace 镜像
#   PIP_INDEX_URL=                   远程 pip 镜像 (如 https://pypi.tuna.tsinghua.edu.cn/simple)
#   GH_PROXY=                        GitHub 加速前缀 (如 https://ghfast.top/), 下载 qdrant 二进制用
#                                    (无 docker 的机器才用得到; 包缓存在本机 .cache/qdrant-<版本>/)
#   QDRANT_VERSION=v1.19.0           与 Dockerfile.qdrant 一致
#   STORAGE_DIR=./qdrant-storage     本机产物目录
#
# 前提: 本机 -> GPU 服务器 ssh 免密已配好 (ssh-copy-id user@host)。
#       bake 后台轮询每 20s ssh 一次, 密码登录无法交互, 必须 key 免密。
#
# 注意:
#   - 烘完有两道自动门 (付费跑唯一的兜底, 别绕过):
#       ① 点数门: 拉产物前趁远程 Qdrant 还在跑读一次 points_count, 和期望值对上才准打包。
#          远端自己打印的 "points:" 只是「我成功发出去多少批」, 和库里真存了多少是两件事。
#          期望值默认由本地 gz 现算 (和远端同一套规则), 不是又一份要手动同步的数字。
#       ② 停服: 用 docker stop / SIGTERM 正常关闭, 且 **保留 WAL 一起打包** —— v1.19 没有
#          flush 接口, 删 WAL 就是删掉还没进 segment 的那部分数据, 且不会有任何报错。
#          WAL 只能在 storage 里: 它是每个 shard 的 <shard>/wal 子目录, 挪不走 ——
#          compose 以前那句 QDRANT__STORAGE__WAL_PATH=/tmp/wal 是无效变量（2026-10-06 已删，
#          出处见 do_fetch 的「已核实」）, 所以首启会正常回放 WAL, 留着没有代价, 删了才有。
#       ③ 续烘 (GPU_RECREATE=0): 向量 id 由 sha256(数据集|行 key + 原始 JSON) 派生,
#          所以"这条烘过没有"是可以查的 —— 库里已有的行直接跳过, 不再花第二次 embedding。
#          只有一种情况别用: gz 数据变过。那会留下上一版的旧点, 清不干净。
#          兜底仍是第 ① 道门 —— 它比的是「相等」不是「不少」, 多出来的旧点一样会被拦下。
#   - GPU 上用 fp32 原模型烘焙 (--int8 是 CPU NEON/AVX 优化, GPU 无效)
#   - fp32(GPU 烘) 与 INT8(CPU 本机 embed_server.py 查询) 向量 cos≈1.0, 直接混用无需重烘焙
#   - 远程 docker 优先跑 Qdrant; 没 docker 退回官方静态二进制, 且**先下到你本机 .cache/qdrant-<版本>/
#     验完 (gzip 完整性 + 体积下限) 再 scp 过去** —— 断流截断下来的半截包会在远程解压成一个
#     同名的跑不起来的 ./qdrant, 症状是「Qdrant 60s 未就绪」, 第二次起就是内网 scp 几秒。
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
GPU_BATCH_SIZE="${GPU_BATCH_SIZE:-24}"
GPU_RECREATE="${GPU_RECREATE:-1}"
# 每次清库 vs 保住已有的接着烘 (两条互斥, 见头部「注意」第 ③ 条)
if [[ "$GPU_RECREATE" == "0" ]]; then BAKE_MODE="--resume"; else BAKE_MODE="--recreate"; fi
EXPECTED_POINTS="${EXPECTED_POINTS:-}"   # 留空 = 由本地 gz 现算 (见 compute_expected_points)
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

# ── 期望点数: 从本地 gz 现算, 不钉死数字 ──────────────────────────
# 用的就是远端将要跑的那套规则 (external/bake_onnx.py 扫 gz/<dataset>/ + 它的跳过名单),
# 所以这条门是「两份内容对不对得上」而不是「记得同步改常量」。整批约几秒。
compute_expected_points() {
  python3 - "$GZ_DIR" <<'PY'
import json, sys
sys.path.insert(0, "external")
import bake_onnx as b
total = 0
for ds, base, path in b.list_seed_archives(sys.argv[1]):
    if ds in b.VECTOR_SKIP_DATASETS:
        continue
    with open(path, "rb") as fh:
        total += len(json.loads(b.decompress_archive(fh.read())))
print(total)
PY
}

# 远程 collection 的实际条数 (Qdrant REST, HTTP 端口 = gRPC-1)
remote_point_count() {
  rsh "curl -sf -m 15 http://localhost:$((QDRANT_PORT-1))/collections/$COLLECTION | tr -d ' ' | grep -o '\"points_count\":[0-9]*' | cut -d: -f2"
}

# ── 点数门: 存够了才准拉产物 ──────────────────────────────────────
check_points_landed() {
  local got
  echo "[gate] 点数门 (期望 $EXPECTED_POINTS)..."
  got=$(remote_point_count || true)
  if [[ -z "$got" ]]; then
    echo "❌ 点数门: 读不到远程 points_count (Qdrant 没在跑?)"
    echo "   远程数据未动: 起 Qdrant 后 ./bake-gpu.sh status 确认, 再手动 ./bake-gpu.sh fetch"
    exit 1
  fi
  if [[ "$got" != "$EXPECTED_POINTS" ]]; then
    echo "❌ 点数门: 库里实际 $got 条 ≠ 期望 $EXPECTED_POINTS 条 — 不拉产物"
    echo "   产物还不完整, 打包就等于把一个残缺的向量层推上线 (远程数据仍在, 未拉回未删除)"
    echo "   先 ./bake-gpu.sh log 看批次错误, 处理后再烘一轮"
    exit 1
  fi
  echo "  ✅ 点数门: $got 条全部在库"
}

# ── 远程烘焙命令 (setsid 后台, 断 ssh 不死; 结束写 bake.rc) ──
remote_bake_cmd() {
cat <<REMOTE
set -e
cd $REMOTE_DIR
mkdir -p qdrant-storage
# nvidia 是 namespace 包, find_spec(...).origin 恒为 None (实测 TypeError), 老写法等于没设;
# 且 cu13 wheel 把 cuBLAS 放在 nvidia/cu13/lib 而不是 nvidia/cublas/lib —— 直接收齐 */lib
NVIDIA_LIBS=\$(python3 -c "import glob,importlib.util as u; s=u.find_spec('nvidia'); p=(s.submodule_search_locations or [''])[0] if s else ''; print(':'.join(sorted(glob.glob(p+'/*/lib'))))" 2>/dev/null || true)
export LD_LIBRARY_PATH="\${NVIDIA_LIBS:+\$NVIDIA_LIBS:}\${LD_LIBRARY_PATH:-}"
rm -f $BAKE_RC
setsid nohup bash -c '
  python3 external/bake_onnx.py \\
    --src=gz --host=localhost --port=$QDRANT_PORT \\
    --collection=$COLLECTION \\
    --model=model/model.onnx --tokenizer=model \\
    --device=cuda --workers=8 --batch-size=$GPU_BATCH_SIZE \\
    --max-text-chars=1024 $BAKE_MODE \\
    > $BAKE_LOG 2>&1
  echo \$? > $BAKE_RC
' >/dev/null 2>&1 < /dev/null &
echo "烘焙已启动 (PID \$!)"
REMOTE
}

# ── Qdrant 二进制兜底: 先落本机缓存再 scp ──
# 无 docker 的机器每次重烘都要在远程下 30MB 二进制; AutoDL 直连 GitHub 会断流
# (脚本里那句 source /etc/network_turbo 就是为它加的), 而半截包 tar 出来是个跑不起来的
# ./qdrant, 报错误判成「Qdrant 60s 未就绪」—— 原因在本机这次 curl, 症状在远程。
# 所以下载改到本机做, 验完再推; 包留在 .cache/ (已 gitignore) 里, 第二次起就是内网 scp。
QDRANT_TGZ="qdrant-x86_64-unknown-linux-musl.tar.gz"
QDRANT_CACHE_DIR=".cache/qdrant-$QDRANT_VERSION"
# 实测 (2026-10-06, 该 release 的 HEAD content-length): 整包 31,974,028 字节。
# 断流截断是唯一会产出的坏包形态, 所以门槛就钉在体积 + gzip 完整性两条上。
QDRANT_MIN_BYTES=20000000
QDRANT_URL="${GH_PROXY}https://github.com/qdrant/qdrant/releases/download/$QDRANT_VERSION/$QDRANT_TGZ"

remote_has_docker() { rsh 'command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1'; }

ensure_qdrant_cache() {
  local tgz="$QDRANT_CACHE_DIR/$QDRANT_TGZ"
  mkdir -p "$QDRANT_CACHE_DIR"
  if [[ -f "$tgz" ]] && gzip -t "$tgz" 2>/dev/null \
     && [[ "$(wc -c <"$tgz" | tr -d '[:space:]')" -ge "$QDRANT_MIN_BYTES" ]]; then
    echo "  qdrant 二进制: 缓存命中 ${tgz#./}"
    return 0
  fi
  echo "  qdrant 二进制: 本机下载 $QDRANT_VERSION (musl 静态, 不依赖系统 glibc)..."
  # 先写 .part 再 mv: 直接 curl -o "$tgz" 的话一次断流就留下个半截包,
  # 而下一轮 -f 判断会把它当缓存命中, 于是这个脚本会反复推出同一个坏二进制。
  # 两次独立尝试是实测需要的: curl 的 --retry 只重试 HTTP 层的瞬时状态码, 而这里真正的失败
  # 是连接层的 "Error in the HTTP2 framing layer" (2026-10-06 本机复现, 第二次就过了)。
  curl -fsSL --retry 5 --connect-timeout 15 -o "$tgz.part" "$QDRANT_URL" \
    || curl -fsSL --retry 5 --connect-timeout 15 -o "$tgz.part" "$QDRANT_URL" \
    || { echo "  下载失败 (GitHub 直连断了? 试试设 GH_PROXY=)"; rm -f "$tgz.part"; return 1; }
  if ! gzip -t "$tgz.part" 2>/dev/null \
     || [[ "$(wc -c <"$tgz.part" | tr -d '[:space:]')" -lt "$QDRANT_MIN_BYTES" ]]; then
    echo "  下载包不完整 (截断), 已丢弃"
    rm -f "$tgz.part"; return 1
  fi
  mv "$tgz.part" "$tgz"
  echo "  qdrant 二进制: 已缓存到 $tgz"
}

# 远程缺二进制时把缓存推上去; 推不动就返回 0, 让 remote_start_qdrant 里的远程直连下载兜底。
push_qdrant_binary() {
  if remote_has_docker; then return 0; fi          # 有 docker 用不到二进制
  if rsh "[ -x $REMOTE_DIR/qdrant ]" 2>/dev/null; then
    echo "  qdrant 二进制: 远程已有 ./qdrant, 跳过"
    return 0
  fi
  if ! ensure_qdrant_cache; then
    echo "  警告: 本机下载失败, 改由远程直连下载 (可能仍受 GitHub 断流影响)"
    return 0
  fi
  echo "  qdrant 二进制: scp 到远程..."
  if ! scp -q -P "$GPU_PORT" ${GPU_SSH_OPTS:+$GPU_SSH_OPTS} \
     "$QDRANT_CACHE_DIR/$QDRANT_TGZ" "$GPU_HOST:$REMOTE_DIR/$QDRANT_TGZ"; then
    echo "  警告: scp 失败, 交给远程直连下载兜底"
    return 0
  fi
  # 解完再验一次: 除了 -x, 还认 ELF magic —— 一个同名空文件或截断产物都过不了这条。
  # 校验不过必须把那个坏 ./qdrant 删掉再交回兜底: 否则 remote_start_qdrant 看到
  # 一个存在且可执行的文件就跳过下载, 症状又变回「Qdrant 60s 未就绪」。
  if rsh "cd $REMOTE_DIR && tar xzf $QDRANT_TGZ && rm -f $QDRANT_TGZ \
        && [ -x ./qdrant ] \
        && [ \"\$(head -c4 ./qdrant | od -An -tx1 | tr -d '[:space:]')\" = 7f454c46 ]"; then
    echo "  ✅ qdrant 二进制就位 (ELF 校验通过)"
    return 0
  fi
  echo "  警告: 远程解包校验失败, 删掉坏文件交给远程直连下载兜底"
  rsh "rm -f $REMOTE_DIR/qdrant $REMOTE_DIR/$QDRANT_TGZ" 2>/dev/null || true
  return 0
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
  # 兜底: 正常情况下 push_qdrant_binary 已经把 ./qdrant 放上来了 (本机缓存 scp)。
  # 只有本机下载失败时才走这条远程直连下载。
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
  echo "[upload] 代码 + gz 数据 (${GZ_DIR}, $(/usr/bin/du -sh "$GZ_DIR" 2>/dev/null | cut -f1))..."
  rsh "mkdir -p $REMOTE_DIR/external $REMOTE_DIR/gz"
  # COPYFILE_DISABLE: macOS 的 tar 默认给每个文件附一份 ._ 开头的元数据兄弟文件,
  # 而归档识别只看后缀 (._x.json.zst 也算 .json.zst), 远端会拿它去解压并当场失败。
  COPYFILE_DISABLE=1 tar cf - external/bake_onnx.py external/export_onnx.py | rsh_tar_in
  COPYFILE_DISABLE=1 tar cf - -C "$GZ_DIR" . | ssh "${SSH_OPTS[@]}" "$GPU_HOST" "tar xf - -C $REMOTE_DIR/gz"
  echo "  gz 完成: $(find "$GZ_DIR" -type f -name '*.json.*z*' | wc -l | tr -d ' ') 个文件"

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
  push_qdrant_binary || echo "  警告: qdrant 二进制推送失败, 交给远程下载兜底"
  rsh "$(remote_start_qdrant)"
  echo "  setup 完成"
}

# ── bake ──
do_bake() {
  # 先把期望点数算出来: 本地 gz 读不动就在花钱之前停下, 而不是烘完才发现没有对照值
  [[ -n "$EXPECTED_POINTS" ]] || EXPECTED_POINTS=$(compute_expected_points)
  echo "[bake] 期望向量条数: $EXPECTED_POINTS (由 $GZ_DIR 现算)"
  # 确保 Qdrant 在跑
  push_qdrant_binary || echo "  警告: qdrant 二进制推送失败, 交给远程下载兜底"
  rsh "$(remote_start_qdrant)" >/dev/null
  if [[ "$BAKE_MODE" == "--resume" ]]; then
    echo "[bake] 模式: 续烘 ($BAKE_MODE) — 库里已有的向量保留, 已烘过的行不再花 embedding"
    echo "       (前提: 这次没改过 gz 数据。改了数据请用默认模式, 否则上一版的旧点会留在库里)"
  else
    echo "[bake] 模式: 清库重烘 ($BAKE_MODE) — 中途失败后可用 GPU_RECREATE=0 接着烘"
  fi
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
        # 已经烘进去的向量还在远程库里, 没丢: 修好问题后接着跑, 不要重来一遍付费
        echo "接着烘 (跳过库里已有的行): GPU_RECREATE=0 $0 bake"
        echo "清库重烘 (改过 gz 数据时才需要): $0 bake"
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
  # [b] 括号技巧: 防止 pkill 匹配到自身 ssh 命令行 (否则自杀, ssh 退出码 255)
  rsh "pkill -f '[b]ake_onnx.py' 2>/dev/null && echo 已停止 || echo 无进程"
}

# ── fetch ──
do_fetch() {
  # 点数门: 趁服务还在跑核一次实际条数 —— 这是唯一还能救的时机 (数据没拉回、远程没删)。
  # 服务已经停了就只告警不硬拦: 产物仍带着 WAL, 数据没被销毁, 缺不缺只能取回后另验。
  if rsh "curl -sf -m 15 'http://localhost:$((QDRANT_PORT-1))/healthz' >/dev/null" >/dev/null 2>&1; then
    [[ -n "$EXPECTED_POINTS" ]] || EXPECTED_POINTS=$(compute_expected_points 2>/dev/null || true)
    if [[ -n "$EXPECTED_POINTS" ]]; then
      check_points_landed
    else
      echo "  警告: 期望点数算不出来 (本地 $GZ_DIR 读不动?), 跳过点数门"
    fi
  else
    echo "  警告: Qdrant 未在运行, 读不到 points_count — 跳过点数门"
  fi
  echo "[fetch] 正常关闭 Qdrant (SIGTERM)..."
  # 用 stop 而不是 rm -f: 硬杀会切断进程自己的收尾落盘
  rsh "cd $REMOTE_DIR && { docker stop $QDRANT_NAME >/dev/null 2>&1 || pkill -TERM -x qdrant 2>/dev/null || true; sleep 5; docker rm $QDRANT_NAME >/dev/null 2>&1 || true; }"
  # 不再删 WAL。v1.19 的 REST 里没有 flush 接口 (openapi 53 条 path 逐条核过), 所以没有任何
  # 办法证明「segment 里已经全了」; 而 WAL 装的正是还没进 segment 的那部分写入本身 ——
  # 删它等于把尾巴抹掉且全程不报错。留着没有代价: 部署首启会正常回放它。
  # 已核实 (2026-10-06, v1.19.0 源码): WAL 挪不出 storage ——
  #   lib/collection/src/config.rs 的 WalConfig 只有 wal_capacity_mb / wal_segments_ahead /
  #   wal_retain_closed 三个字段, 没有路径; 目录由 LocalShard::wal_path(shard_path) 派生,
  #   即 collections/<collection>/<shard>/wal。StorageConfig 也没有 deny_unknown_fields,
  #   所以 compose 以前那句 QDRANT__STORAGE__WAL_PATH=/tmp/wal 是被静默丢弃的无效变量
  #   (2026-10-06 删掉), 「线上只认 segment、镜像里的 WAL 不会回放」这个旧说法是错的。
  # 结论不变但理由换了: 条数必须在打包前、问还在跑的服务 —— 那就是上面那道点数门。
  rsh "cd $REMOTE_DIR && { tar czf bundle.tgz qdrant-storage; du -sh bundle.tgz; }"
  echo "[fetch] 拉回本机 $STORAGE_DIR ..."
  rm -rf "$STORAGE_DIR"
  mkdir -p "$STORAGE_DIR"
  scp -P "$GPU_PORT" ${GPU_SSH_OPTS:+$GPU_SSH_OPTS} "$GPU_HOST:$REMOTE_DIR/bundle.tgz" /tmp/bake-gpu-bundle.tgz
  tar xzf /tmp/bake-gpu-bundle.tgz -C "$STORAGE_DIR" --strip-components=1
  rm -f /tmp/bake-gpu-bundle.tgz
  local size segs
  size=$(du -sh "$STORAGE_DIR" | cut -f1)
  # 原来这里找的是 *.seg/*.idx, 而 v1.19 落盘的文件扩展名是 .dat/.bin/.mmap (实测产物里
  # 一个 .seg 都没有), 于是每次 fetch 都打印「0 个 segment 文件」—— 一个恒为 0 的假指标。
  # 段目录才是一对一的计数单位: collections/<集合>/<shard>/segments/<uuid>。
  segs=$(find "$STORAGE_DIR/collections" -mindepth 4 -maxdepth 4 -type d 2>/dev/null | wc -l | tr -d ' ')
  echo ""
  echo "============================================"
  echo "  ✅ GPU 烘焙产物已就位: $STORAGE_DIR ($size, $segs 个段目录)"
  echo "  下一步: ./build.sh qdrant   (Dockerfile.qdrant.slim 打包)"
  echo "  退租: AutoDL 控制台关机/释放即销毁远程数据 (无 clean 命令)"
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
