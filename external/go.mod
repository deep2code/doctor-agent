// This go.mod makes external/ a nested module boundary so that `go build
// ./...`, `go vet ./...` and `golangci-lint run ./...` from the repo root
// skip this directory (it holds only scripts, data and the sandboxed module
// cache external/gomodcache — nothing to compile).
module github.com/doctor-agent/external

go 1.26
