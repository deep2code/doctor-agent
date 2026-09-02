# syntax=docker/dockerfile:1
# Pre-baked runtime base — business Dockerfile builds FROM this, apk step gone.
# Build ONCE on the packer (Aliyun ECS, aliyun mirror is in-region fast):
#   docker build -f base.Dockerfile -t doctor-runtime:3.20 .
# Rebuild only when runtime deps change, then retag/repush.
FROM alpine:3.20
RUN sed -i 's#dl-cdn.alpinelinux.org#mirrors.aliyun.com#g' /etc/apk/repositories \
    && apk add --no-cache ca-certificates curl mariadb-client
