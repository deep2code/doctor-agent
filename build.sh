#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

# ============================================================================
# doctor-agent 唯一打包入口（构建 + 推送阿里云）
#
# 用法:
#   ./build.sh           = app（默认）：只构建+推送 app 镜像（改代码/前端）
#   ./build.sh qdrant    = 只构建+推送 RAG 镜像（改知识库数据、跑 python3 external/make_gz.py 之后）
#   ./build.sh embed     = 只构建+推送 bge-m3 INT8 查询端 embedding 镜像
#   ./build.sh kb        = 只构建+推送预灌知识的 MariaDB 镜像（只发 :latest，不打版本号标签）
#   ./build.sh full      = 全量：app + qdrant + embed + kb
#
# 四镜像架构（compose 四个服务，触发条件解耦）:
#   doctor-agent         Go 源码 + 前端        → 代码变化才更新
#   doctor-agent-qdrant  Qdrant + 烘好的向量    → 知识库变化才更新
#   doctor-agent-embed   bge-m3 INT8 embedding → embedding 服务/模型变化才更新
#   doctor-agent-kb      预灌 doctor_knowledge 的 MariaDB → 知识数据变化才更新
#   macOS 流程: 有 qdrant-storage 产物 → 直接打包; 无 → 调 bake-local.sh 本机烘焙 → 打包
#               (有产物时不提供强制重烘选项; 要重烘先手动删产物或跑 bake-local.sh)
#               ⚠️ bake-local.sh 当前不在仓库里（2026-09-20 核实），无产物时此路径会失败；
#                  可用 GPU 烘焙管线 bake-gpu.sh 产出 qdrant-storage/ 后再打包。
#   Linux  流程: Dockerfile.qdrant 内编译 + 烘焙，需构建期传入 EMBEDDING_BASE_URL
#               （bge-m3 OpenAI 兼容端点；2026-09-06 起无本地 hash 回退，未配置直接报错退出）
#               ⚠️ 本脚本的 Linux 分支没有转发任何 --build-arg，故裸跑会在烘焙阶段
#                  因 EMBEDDING_BASE_URL 为空而失败（2026-09-20 核实，未修）。
#                  临时绕过: docker build -f Dockerfile.qdrant --build-arg EMBEDDING_BASE_URL=... .
# ============================================================================

MODE="${1:-app}"
case "$MODE" in
  app|qdrant|embed|kb|full) ;;
  *) echo "用法: $0 [app|qdrant|embed|kb|full]"; exit 1 ;;
esac

# ── 版本信息 ──────────────────────────────────────
GIT_COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
BUILD_TIME="$(TZ=Asia/Shanghai date '+%Y-%m-%dT%H:%M:%S+08:00')"
GIT_TAG="$(git describe --tags --abbrev=0 2>/dev/null || echo latest)"

# ── 镜像仓库（唯一定义处；Linux 推送机走内网）─────
REGISTRY_PUBLIC="crpi-0xi5k79l9j4opzta.cn-hangzhou.personal.cr.aliyuncs.com/codeup2026"
REGISTRY_VPC="crpi-0xi5k79l9j4opzta-vpc.cn-hangzhou.personal.cr.aliyuncs.com/codeup2026"
if [[ "$(uname -s)" == "Linux" ]]; then
  REGISTRY="$REGISTRY_VPC"
else
  REGISTRY="$REGISTRY_PUBLIC"
fi

APP_IMAGE="${REGISTRY}/doctor-agent:${GIT_TAG}"
APP_IMAGE_LATEST="${REGISTRY}/doctor-agent:latest"
QDRANT_IMAGE="${REGISTRY}/doctor-agent-qdrant:latest"
EMBED_IMAGE="${REGISTRY}/doctor-agent-embed:latest"
# kb 镜像只发 :latest —— 数据镜像不做版本号标签, version.json 只作为构建期溯源信息打印。
KB_DATA_VERSION="$(python3 -c "import json;print(json.load(open('internal/knowledge/data/version.json'))['version'])" 2>/dev/null || echo unknown)"
KB_IMAGE="${REGISTRY}/doctor-agent-kb:latest"

build_embed() {
  echo "[embed] 构建 embedding 查询服务镜像 (bge-m3 INT8 模型打入镜像)..."
  [[ -f "bge-m3-onnx/model.int8.onnx" ]] || {
    echo "  错误: bge-m3-onnx/model.int8.onnx 不存在, 先运行 python3 external/export_onnx.py --int8"
    exit 1
  }
  docker build --progress=plain --platform linux/amd64 \
    --pull=false \
    -t "$EMBED_IMAGE" \
    -f Dockerfile.embed \
    --provenance false \
    .
}

build_app() {
  echo "[app] 宿主机编译（复用本机 Go 缓存, 暖构建秒级）..."
  # go 常不在非交互 shell 的 PATH 里 (bashrc 的 PATH 只对交互终端生效), 探测常见位置
  if ! command -v go >/dev/null 2>&1; then
    for d in /usr/local/go/bin "$HOME/go/bin" /usr/lib/go/bin /usr/local/bin "$HOME/.local/bin"; do
      [ -x "$d/go" ] && export PATH="$d:$PATH" && break
    done
  fi
  command -v go >/dev/null 2>&1 || {
    echo "  错误: 打包机找不到 go (host compile 模式)。装 Go 1.21+ 后重试:"
    echo "    wget -qO- https://golang.google.cn/dl/go1.27.1.linux-amd64.tar.gz | tar xz -C /usr/local"
    echo "    并确认 /usr/local/go/bin/go 存在 (脚本会自动探测该路径)"
    exit 1
  }
  echo "  go: $(go version | awk '{print $3}')"
  export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
  # 静态二进制: 无 CGO (go-sql-driver/mysql 纯 Go), 目标 linux/amd64
  # 版本信息在此注入 (原 Dockerfile 构建层的等价逻辑)
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
    -ldflags "-s -w -X main.gitCommit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o doctor-agent-linux .
  echo "  编译完成: $(du -h doctor-agent-linux | cut -f1)"

  echo "[app] 打包镜像（只 COPY 二进制, 秒级）..."
  docker build --progress=plain --platform linux/amd64 \
    --pull=false \
    -t "$APP_IMAGE" \
    -t "$APP_IMAGE_LATEST" \
    -f Dockerfile \
    --provenance false \
    .
}

build_qdrant() {
  echo "[qdrant] 构建 RAG 镜像..."
  if [[ ! -d "internal/knowledge/gz" ]] || [[ -z "$(ls internal/knowledge/gz/*.json.*z* 2>/dev/null)" ]]; then
    echo "  错误: internal/knowledge/gz 为空，先运行 python3 external/make_gz.py 生成知识库压缩包"
    exit 1
  fi

  # macOS：本机烘焙 + slim 镜像打包（全量真向量，不省资源）
  #
  # 两步流程：
  #   1. bake-local.sh — Mac 本机直跑 vector-bake，直连 localhost:11434
  #      （无 Docker 网络开销），OLLAMA_NUM_PARALLEL=8 + workers=8 + batch=128，
  #      bake.go 按文本长度排序消除 padding 浪费。
  #      可向量化的数据用 bge-m3 生成真实语义向量（bake.go 的 vectorSkipDatasets
  #      列出的数据集按设计跳过——它们有专用 lookup 工具或关键词全文层）。
  #   2. Dockerfile.qdrant.slim — 只 COPY 预烘焙 storage 到 Qdrant 基础镜像
  #      （~30 秒纯 COPY，无编译无烘焙）。
  #
  # 对比旧方案（Docker 内烘焙）：
  #   - 消除 host.docker.internal 网络开销
  #   - 消除 BuildKit 输出缓冲（看不到进度）
  #   - 消除 Docker 内存限制（16GB 全可用）
  #   - 文本长度排序 3-5x 加速（padding 浪费消除）
  #   - OLLAMA_NUM_PARALLEL=8 embedding 并发（无 KV cache，几乎零额外内存）
  if [[ "$(uname -s)" == "Darwin" ]]; then
    echo "  macOS → RAG 镜像（有烘焙产物直接打包，无产物才本机烘焙）"

    # Step 1: 烘焙产物检测 — 有产物直接打包，不允许强制重烘
    #   ./build.sh qdrant    本机已有 qdrant-storage → 直接打包；没有 → 本机烘焙
    if [[ -d "qdrant-storage/collections/medical_knowledge" ]]; then
      echo "  [1/2] 检测到已有烘焙产物 → 跳过烘焙，直接打包"
    else
      echo "  [1/2] 无烘焙产物，本机烘焙..."
      # 传递 BAKE_RECREATE 和自定义参数
      export EMBEDDING_MODEL="${EMBEDDING_MODEL:-bge-m3}"
      export BAKE_WORKERS="${BAKE_WORKERS:-8}"
      export BAKE_BATCH_SIZE="${BAKE_BATCH_SIZE:-128}"
      ./bake-local.sh

      # 检查烘焙产物
      if [[ ! -d "qdrant-storage" ]] || [[ -z "$(ls qdrant-storage/ 2>/dev/null)" ]]; then
        echo "  错误: 烘焙产物 qdrant-storage/ 为空"
        exit 1
      fi
    fi

    # Step 2: slim 镜像打包（只 COPY storage）
    echo "  [2/2] 打包 slim 镜像（Dockerfile.qdrant.slim）..."
    docker build --progress=plain --platform linux/amd64 \
      --pull=false \
      -t "$QDRANT_IMAGE" \
      -f Dockerfile.qdrant.slim \
      --provenance false \
      .

    # 不自动删 qdrant-storage: 产物来之不易 (GPU 烘焙 ~1 小时 + 手动恢复过),
    # 留在本机作为镜像之外的第二副本, 要清理请手动删。
  else
    echo "  Linux → Docker 内烘焙（需构建期传入 EMBEDDING_BASE_URL，无 hash 回退）"
    docker build --progress=plain --platform linux/amd64 \
      --pull=false \
      -t "$QDRANT_IMAGE" \
      -f Dockerfile.qdrant \
      --provenance false \
      .
  fi
}

push_image() {
  echo "  推送: $1"
  docker push "$1"
}

# kb_assert_only_kb_items <容器>: doctor_knowledge 里只允许 kb_items 一张表。
# 空的外来表直接清掉; 有数据的立即失败(不替你删业务数据)。与 update-kb.sh 步骤 2
# 的门是同一条规则(那边在导出前跑, 这里兜住 build.sh 自己现场导出的路径)。
kb_assert_only_kb_items() {
  local container stray tb rows
  container="${1:?usage: kb_assert_only_kb_items <container>}"
  stray="$(docker exec "$container" mariadb -uroot -N -e \
    "SELECT table_name FROM information_schema.tables WHERE table_schema='doctor_knowledge' AND table_name<>'kb_items' ORDER BY table_name")" || {
      echo "  错误: 读不到 ${container} 的 doctor_knowledge 表清单"
      return 1
    }
  for tb in $stray; do
    rows="$(docker exec "$container" mariadb -uroot -N -e "SELECT COUNT(*) FROM \`doctor_knowledge\`.\`${tb}\`")"
    if [ "$rows" != "0" ]; then
      echo "错误: doctor_knowledge.${tb} 有 ${rows} 行业务数据, 不能打进知识镜像。"
      echo "      先查是不是 MARIA_DB_APP_DB 被指到了 doctor_knowledge, 把数据迁回业务库后重跑。"
      return 1
    fi
    docker exec "$container" mariadb -uroot -e "DROP TABLE \`doctor_knowledge\`.\`${tb}\`"
    echo "    已清掉空表 $tb"
  done
}

# build_kb: 打包预灌知识的 MariaDB 数据镜像 (doctor-agent-kb).
# 前置: docker/kb/init-doctor_knowledge.sql.gz 存在 (从已灌库导出, 见 docker/Dockerfile.kb 头注释).
# 空洞兜底: 无 dump 则现场从本地 3307 测试容器导出.
build_kb() {
  [[ -s "docker/kb/init-doctor_knowledge.sql.gz" ]] || {
    echo "[kb] 无 dump, 从本地 doctor-kb-test 容器导出..."
    # 兜底路径同样要过这道门: internal/database.New() 一连知识库就把整套业务表建在
    # doctor_knowledge 里, 直接 dump 会把 users/sessions/… 打进数据镜像。
    kb_assert_only_kb_items doctor-kb-test || exit 1
    mkdir -p docker/kb
    docker exec doctor-kb-test mariadb-dump -uroot --quick --single-transaction --hex-blob \
      --default-character-set=utf8mb4 --databases doctor_knowledge 2>/dev/null \
      | gzip > docker/kb/init-doctor_knowledge.sql.gz
  }
  echo "[kb] 构建数据镜像 (知识库版本 ${KB_DATA_VERSION}, tag: latest)..."
  docker build --progress=plain --platform linux/amd64 \
    --pull=false \
    -t "$KB_IMAGE" \
    -f docker/Dockerfile.kb \
    --provenance false \
    .
}

echo "============================================"
echo "  doctor-agent 打包（模式: ${MODE}）"
echo "  版本:      ${GIT_TAG} (commit ${GIT_COMMIT})"
echo "  构建时间:  ${BUILD_TIME}"
echo "  应用镜像:  ${APP_IMAGE_LATEST}      ← 代码变化才更新"
echo "  RAG 镜像:  ${QDRANT_IMAGE}   ← 知识库变化才更新"
echo "============================================"
echo ""

case "$MODE" in
  app)
    build_app
    push_image "$APP_IMAGE"
    push_image "$APP_IMAGE_LATEST"
    ;;
  qdrant)
    build_qdrant
    push_image "$QDRANT_IMAGE"
    ;;
  embed)
    build_embed
    push_image "$EMBED_IMAGE"
    ;;
  kb)
    build_kb
    push_image "$KB_IMAGE"
    ;;
  full)
    build_app
    push_image "$APP_IMAGE"
    push_image "$APP_IMAGE_LATEST"
    build_qdrant
    push_image "$QDRANT_IMAGE"
    build_embed
    push_image "$EMBED_IMAGE"
    build_kb
    push_image "$KB_IMAGE"
    ;;
esac

echo ""
echo "============================================"
echo "  打包完成（${MODE}）"
echo "  触发关系: 改代码 → ./build.sh | 改知识库 → ./build.sh qdrant | 都改 → ./build.sh full"
echo "============================================"
