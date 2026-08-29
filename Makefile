.PHONY: build run test lint clean chat serve docker-build docker-push release

# ── 版本信息（git 注入，兜底 unknown）──────────────────────────
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
VERSION    := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")

LDFLAGS := -s -w \
	-X main.gitCommit=$(GIT_COMMIT) \
	-X main.buildTime=$(BUILD_TIME)

# ── Docker 镜像仓库（可用环境变量覆盖）──────────────────────
IMAGE_NAME  ?= doctor-agent:latest
DATA_IMAGE  ?= doctor-agent-data:latest

# Build the binary (with version injection + stripped symbols)
build:
	go build -ldflags "$(LDFLAGS)" -o bin/doctor-agent .

# Run in CLI chat mode
chat: build
	./bin/doctor-agent chat

# Run in HTTP server mode
serve: build
	./bin/doctor-agent serve

# Run all tests
test:
	go test ./... -v -count=1

# Run tests with race detection
test-race:
	go test ./... -v -race -count=1

# Run tests with coverage
test-cover:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

# Lint (requires golangci-lint)
lint:
	golangci-lint run ./...

# Format
fmt:
	go fmt ./...

# Vet
vet:
	go vet ./...

# Clean build artifacts
clean:
	rm -rf bin/ coverage.out coverage.html

# Install dependencies
deps:
	go mod tidy
	go mod download

# Verify knowledge JSON files
verify-knowledge:
	go run . verify-knowledge

# Regenerate gzip-embedded knowledge files (run after editing data/*.json)
gz:
	python3 external/make_gz.py

# ── Docker 构建 ──────────────────────────────────────────────
# 构建应用镜像（瘦镜像，不含知识库数据）
docker-build:
	docker build \
	  --build-arg GIT_COMMIT=$(GIT_COMMIT) \
	  --build-arg BUILD_TIME=$(BUILD_TIME) \
	  -t $(IMAGE_NAME) \
	  -f Dockerfile \
	  .

# 构建知识库数据镜像（137M gz，极少变动）
docker-build-data:
	docker build -t $(DATA_IMAGE) -f Dockerfile.data .

# 推送镜像到仓库
docker-push:
	docker push $(IMAGE_NAME)
	@if docker image inspect $(DATA_IMAGE) >/dev/null 2>&1; then \
		docker push $(DATA_IMAGE); \
	fi

# Help
help:
	@echo "make build            - Build binary (with version info + stripped symbols)"
	@echo "make chat             - Run in CLI chat mode"
	@echo "make serve            - Run in HTTP server mode"
	@echo "make test             - Run all tests"
	@echo "make test-race        - Run tests with race detection"
	@echo "make test-cover       - Run tests with coverage report"
	@echo "make lint             - Run golangci-lint"
	@echo "make fmt              - Format code"
	@echo "make vet              - Run go vet"
	@echo "make clean            - Remove build artifacts"
	@echo "make deps             - Install and tidy dependencies"
	@echo "make verify-knowledge - Verify knowledge JSON files"
	@echo "make gz               - Regenerate gzip knowledge files"
	@echo "make docker-build     - Build app Docker image"
	@echo "make docker-build-data - Build knowledge data Docker image"
	@echo "make docker-push      - Push Docker images to registry"
