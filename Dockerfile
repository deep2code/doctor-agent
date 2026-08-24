# syntax=docker/dockerfile:1

# ---------- build stage ----------
FROM golang:1.26-alpine AS build
RUN apk add --no-cache ca-certificates git
WORKDIR /src

# China-accessible module proxy (proxy.golang.org is blocked on CN networks);
# disable the blocked Go checksum database as well.
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=off

# Cache module downloads
COPY go.mod go.sum ./
RUN go mod download

# Build the static binary (no CGO; go-sql-driver/mysql is pure Go)
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -o /out/doctor-agent .

# ---------- runtime stage ----------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates curl mariadb-client
WORKDIR /app

COPY --from=build /out/doctor-agent /usr/local/bin/doctor-agent
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

EXPOSE 7071

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
