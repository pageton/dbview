# dbview development tasks
# Run `just` to list available recipes.

set positional-arguments

app       := "dbview"
cmd       := "./cmd/dbview"
go_version := "1.25.7"

# Default: list available recipes
default:
    @just --list

# ── Build ────────────────────────────────────────────────────────────────

# Build debug binary (current platform)
build:
    go build -trimpath -ldflags="-s -w" -o {{ app }} {{ cmd }}

# Build and run with given database path
run *args:
    go run {{ cmd }} {{ args }}

# Build release binary with version injected
build-release version:
    go build -trimpath \
        -ldflags="-s -w -X main.version={{ version }}" \
        -o {{ app }} {{ cmd }}

# Build all release targets locally (linux/mac/windows, amd64/arm64)
build-all version: (build-cross version "linux" "amd64") (build-cross version "linux" "arm64") (build-cross version "darwin" "amd64") (build-cross version "darwin" "arm64") (build-cross version "windows" "amd64")

# Cross-compile for a single OS/arch
build-cross version os arch:
    #!/usr/bin/env bash
    set -euo pipefail
    ext=""
    [[ "{{ os }}" == "windows" ]] && ext=".exe"
    mkdir -p dist
    CGO_ENABLED=0 GOOS={{ os }} GOARCH={{ arch }} \
        go build -buildvcs=false -trimpath \
        -ldflags="-s -w -X main.version={{ version }}" \
        -o "dist/{{ app }}_{{ version }}_{{ os }}_{{ arch }}${ext}" {{ cmd }}
    echo "built dist/{{ app }}_{{ version }}_{{ os }}_{{ arch }}${ext}"

# ── Check ────────────────────────────────────────────────────────────────

# Run all checks (fmt, vet, lint, test) — matches CI order
check: fmt vet lint test

# Verify go.mod/go.sum are tidy
deps:
    go mod verify

# Format code; fails if any file needs formatting
fmt:
    test -z "$(gofmt -l -s .)"

# Format code in-place
fmt-fix:
    gofmt -l -s -w .

# Run go vet
vet:
    go vet ./...

# Run golangci-lint
lint:
    golangci-lint run

# Run all tests with race detector
test *args="":
    go test -v -count=1 -race ./... {{ args }}

# Run a single test by name
test-one name:
    go test -v -run {{ name }} ./...

# ── Dev shortcuts ────────────────────────────────────────────────────────

# Quick smoke test: build + open example.db
smoke: build
    ./{{ app }} ./example.db

# Clean build artifacts
clean:
    rm -rf dist {{ app }}
