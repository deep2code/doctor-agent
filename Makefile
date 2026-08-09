.PHONY: build run test lint clean chat serve

# Build the binary
build:
	go build -o bin/doctor-agent .

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

# Help
help:
	@echo "make build       - Build the binary"
	@echo "make chat        - Run in CLI chat mode"
	@echo "make serve       - Run in HTTP server mode"
	@echo "make test        - Run all tests"
	@echo "make test-race   - Run tests with race detection"
	@echo "make test-cover  - Run tests with coverage report"
	@echo "make lint        - Run golangci-lint"
	@echo "make fmt         - Format code"
	@echo "make vet         - Run go vet"
	@echo "make clean       - Remove build artifacts"
	@echo "make deps        - Install and tidy dependencies"
