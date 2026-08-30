# doctor-agent 知识库 RAG 镜像 —— 一个镜像 = 纯 gz 知识库 + 标准 Qdrant + 烘好的向量
#
# 架构（2026-08-30）：
#   只有两个镜像：
#     doctor-agent-qdrant   本镜像：alpine(gz 知识库 51 数据集) + Qdrant + 向量
#                           → 只有知识库变化才更新
#     doctor-agent          Go 源码 + 前端（go:embed web/*.html）
#                           → 只有代码变化才更新
#   （doctor-agent-data 已废弃合并进本镜像）
#
# 本 Dockerfile 必须用【独立 context】构建（docker/qdrant-context 目录）：
#   ./docker/qdrant-context/build.sh
# 独立 context 里只有 gz + src(烘焙工具源码) + 本 Dockerfile，与项目根
# .dockerignore（排除了 internal/knowledge/gz 以给 app 瘦身）彻底隔离。
#
# 烘焙工具 = ./cmd/vector-bake（独立于业务代码的小工具，2026-08-30 分离）：
#   本镜像在构建期用 golang 编译它并执行 vector-bake，把 51 个 gz 离线
#   灌入 Qdrant storage。不依赖 doctor-agent app 镜像 → 业务代码变化
#   完全不触发本镜像重建；本镜像也不需要 app 镜像先行构建。
#
# context 结构（docker/qdrant-context/，由 build.sh 同步）：
#   gz/          ← internal/knowledge/gz 的拷贝（51 个 .json.gz/.zst）
#   src/         ← cmd/vector-bake + 其依赖包源码 + go.mod/go.sum
#   Dockerfile   ← 本文件
#
# 最终镜像内容：
#   /opt/knowledge/gz     纯 gz 知识库（51 数据集，alpine 数据层）
#   /qdrant/storage       烘好的 Qdrant 向量（构建期 vector-bake 注入）
#   qdrant 服务            容器启动即加载知识，零灌入等待

# syntax=docker/dockerfile:1

# ---------- stage 1: 编译独立烘焙工具 ./cmd/vector-bake（不依赖 app 镜像） ----------
FROM golang:1.26-alpine AS build

RUN sed -i 's#dl-cdn.alpinelinux.org#mirrors.aliyun.com#g' /etc/apk/repositories \
    && apk add --no-cache ca-certificates git

ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=off

WORKDIR /src

# Cache module downloads (BuildKit cache mount survives across builds)
COPY src/go.mod src/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY src/ .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
      -ldflags "-s -w" \
      -o /out/vector-bake ./cmd/vector-bake

# ---------- stage 2: 数据层（纯 gz 知识库，alpine） ----------
# 独立小层，供最终镜像 COPY --from 取 gz；也作为镜像里的"纯 gz 知识库"。
FROM alpine:3.20 AS datalayer
RUN sed -i 's#dl-cdn.alpinelinux.org#mirrors.aliyun.com#g' /etc/apk/repositories \
    && apk add --no-cache ca-certificates
COPY gz /opt/knowledge/gz

# ---------- stage 3: 烘焙（构建期把 51 个 gz 灌入 Qdrant storage） ----------
FROM qdrant/qdrant:latest AS bake

COPY --from=build /out/vector-bake /usr/local/bin/vector-bake
COPY --from=datalayer /opt/knowledge/gz /opt/knowledge/gz

# vector-bake 默认走本地离线 hash embedding（1024 维），无需外部 API；
# vector-bake 内部自带 Qdrant 就绪等待（最多 120s）。
# 注: 新版 qdrant 镜像移除了 --storage-path CLI 参数，改用 env 配置
#     （QDRANT__STORAGE__STORAGE_PATH 对新旧版本均有效）。
RUN sh -c ' \
    echo "== starting qdrant for bake ==" && \
    QDRANT__STORAGE__STORAGE_PATH=/qdrant/storage /qdrant/qdrant & \
    QPID=$!; \
    /usr/local/bin/vector-bake --src=/opt/knowledge/gz --host=127.0.0.1 --port=6334 --collection=medical_knowledge --workers=4; \
    RC=$?; \
    sleep 2; \
    kill $QPID 2>/dev/null; wait $QPID 2>/dev/null; \
    exit $RC'

# ---------- stage 4: 最终镜像 = 纯 gz 知识库 + 标准 Qdrant + 烘好的向量 ----------
FROM qdrant/qdrant:latest

# 纯 gz 知识库（51 数据集）—— 供挂载/关键词兜底用
COPY --from=datalayer /opt/knowledge/gz /opt/knowledge/gz

# 烘好的向量（构建期已注入）
COPY --from=bake /qdrant/storage /qdrant/storage

EXPOSE 6333 6334
