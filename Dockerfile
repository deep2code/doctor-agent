# syntax=docker/dockerfile:1

# ============================================================================
# doctor-agent 应用镜像 —— 宿主机编译版
#
# Go 二进制由打包机宿主机编译 (build.sh build_app):
#   CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o doctor-agent-linux .
# 宿主机 Go 缓存直接复用 (暖构建 ~2s), 不再依赖 Docker builder 的 cache mount。
#
# 前提: 打包机需装 Go 1.26+。本镜像只 COPY 二进制 + entrypoint, 秒级完成。
# ============================================================================

# China-accessible Alpine mirror (dl-cdn.alpinelinux.org is slow/blocked on CN networks)
# BuildKit caches this layer — apk only runs once.
FROM alpine:3.20
RUN sed -i 's#dl-cdn.alpinelinux.org#mirrors.aliyun.com#g' /etc/apk/repositories \
    && apk add --no-cache ca-certificates curl mariadb-client

WORKDIR /app

COPY doctor-agent-linux /usr/local/bin/doctor-agent
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/doctor-agent /usr/local/bin/docker-entrypoint.sh

EXPOSE 7071

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
