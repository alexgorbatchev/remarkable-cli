set dotenv-load := false
set positional-arguments

version := env('VERSION', `git describe --tags --always 2>/dev/null || echo "1.0.0"`)

# Default recipe: list available recipes
default:
    @just --list

# Run CLI in human mode (default)
run *args:
    go run -ldflags "-s -w -X main.version={{version}}" ./cmd/remarkable "$@"

# Run CLI in agent-facing mode
run-ai *args:
    AGENT=1 go run -ldflags "-s -w -X main.version={{version}}" ./cmd/remarkable "$@"

# Run test suite
test:
    go test -race -v ./...

# Build binary into bin/
build:
    mkdir -p bin
    go build -ldflags "-s -w -X main.version={{version}}" -o bin/remarkable ./cmd/remarkable

# Install binary to GOPATH bin directory
install:
    go install -ldflags "-s -w -X main.version={{version}}" ./cmd/remarkable

# Run static analysis and vet
lint:
    go mod tidy -diff
    go vet ./...

# Alias for lint
vet: lint

# Format Go code
fmt:
    go fmt ./...

# Run static analysis and test suite in sequence
check: lint test

# Create a disposable document from a source PDF page and verify native import
native-import-check *args: build
    bun install --cwd scripts/native-import-check --frozen-lockfile
    bun scripts/native-import-check/runCheck.ts "$@"

# Type-check the manual script without live cloud writes
native-import-typecheck:
    bun install --cwd scripts/native-import-check --frozen-lockfile
    bun run --cwd scripts/native-import-check typecheck

# Clean up build binaries and temporary files
clean:
    rm -rf bin coverage.out .tmp dist
