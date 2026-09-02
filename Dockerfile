# syntax=docker/dockerfile:1

# ---------- build stage ----------
FROM golang:1.26-alpine AS build

# China-accessible Alpine mirror (dl-cdn.alpinelinux.org is slow/blocked on CN networks)
RUN sed -i 's#dl-cdn.alpinelinux.org#mirrors.aliyun.com#g' /etc/apk/repositories \
    && apk add --no-cache ca-certificates git

WORKDIR /src

# China-accessible module proxy (proxy.golang.org is blocked on CN networks);
# disable the blocked Go checksum database as well.
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=off

# Cache module downloads (BuildKit cache mount survives across builds)
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Build the static binary (no CGO; go-sql-driver/mysql is pure Go)
# Version info injected via build args (compatible with Makefile/builder.sh)
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
      -ldflags "-s -w -X main.gitCommit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME}" \
      -o /out/doctor-agent .

# ---------- runtime stage ----------
FROM alpine:3.20
RUN sed -i 's#dl-cdn.alpinelinux.org#mirrors.aliyun.com#g' /etc/apk/repositories \
    && apk add --no-cache ca-certificates curl mariadb-client
WORKDIR /app

COPY --from=build /out/doctor-agent /usr/local/bin/doctor-agent
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

EXPOSE 7071

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
