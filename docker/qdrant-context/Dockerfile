# doctor-agent 知识库 RAG 镜像 —— 一个镜像 = 纯 gz 知识库 + 标准 Qdrant + 烘好的向量
#
# 架构（2026-08-30 修正）：
#   只有两个镜像：
#     doctor-agent-qdrant   本镜像：alpine(gz 知识库 51 数据集) + Qdrant + 向量
#                           → 只有知识库变化才更新
#     doctor-agent          Go 源码 + 前端（go:embed web/*.html）
#                           → 只有代码变化才更新
#   （doctor-agent-data 已废弃合并进本镜像）
#
# 本 Dockerfile 必须用【独立 context】构建（docker/qdrant-context 目录）：
#   ./docker/qdrant-context/build.sh
# 独立 context 里只有 gz + 本 Dockerfile，与项目根 .dockerignore
# （排除了 internal/knowledge/gz 以给 app 瘦身）彻底隔离。
#
# 构建顺序：先构建 doctor-agent（提供 vector-bake 二进制），再构建本镜像
# （从 doctor-agent COPY --from 拿二进制，不再自己编译 Go 源码）：
#   docker build -f Dockerfile -t doctor-agent:latest .            # 先
#   ./docker/qdrant-context/build.sh doctor-agent-qdrant:latest    # 后
#
# context 结构（docker/qdrant-context/）：
#   gz/          ← internal/knowledge/gz 的拷贝（51 个 .json.gz，由 build.sh 同步）
#   Dockerfile   ← 本文件
#
# 最终镜像内容：
#   /opt/knowledge/gz     纯 gz 知识库（51 数据集，alpine 数据层）
#   /qdrant/storage       烘好的 Qdrant 向量（构建期 vector-bake 注入）
#   qdrant 服务            容器启动即加载知识，零灌入等待

# syntax=docker/dockerfile:1

# ---------- stage 1: 烘焙工具二进制（从 app 镜像拿，不自己编译） ----------
ARG APP_IMAGE=doctor-agent:latest
FROM ${APP_IMAGE} AS tools

# ---------- stage 2: 数据层（纯 gz 知识库，alpine） ----------
# 独立小层，供最终镜像 COPY --from 取 gz；也作为镜像里的"纯 gz 知识库"。
FROM alpine:3.20 AS datalayer
RUN sed -i 's#dl-cdn.alpinelinux.org#mirrors.aliyun.com#g' /etc/apk/repositories \
    && apk add --no-cache ca-certificates
COPY gz /opt/knowledge/gz

# ---------- stage 3: 烘焙（构建期把 51 个 gz 灌入 Qdrant storage） ----------
FROM qdrant/qdrant:latest AS bake

COPY --from=tools /usr/local/bin/doctor-agent /usr/local/bin/doctor-agent
COPY --from=datalayer /opt/knowledge/gz /opt/knowledge/gz

# vector-bake 默认走本地离线 hash embedding（1024 维），无需外部 API；
# vector-bake 内部自带 Qdrant 就绪等待（最多 120s）。
RUN sh -c ' \
    echo "== starting qdrant for bake ==" && \
    /qdrant/qdrant --storage-path /qdrant/storage & \
    QPID=$!; \
    /usr/local/bin/doctor-agent vector-bake --src=/opt/knowledge/gz --host=127.0.0.1 --port=6334 --collection=medical_knowledge --workers=4; \
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
