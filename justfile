set dotenv-load := false
set positional-arguments

# Default recipe: list available recipes
default:
    @just --list

# Run CLI in human mode (default)
run *args:
    go run ./cmd/remarkable-sync "$@"

# Run CLI in agent-facing mode
run-ai *args:
    AGENT=1 go run ./cmd/remarkable-sync "$@"

# Run test suite
test:
    go test -race -v ./...

# Build binary into bin/
build:
    mkdir -p bin
    go build -o bin/remarkable-sync ./cmd/remarkable-sync

# Install binary to GOPATH bin directory
install:
    go install ./cmd/remarkable-sync

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

# Clean up build binaries and temporary files
clean:
    rm -rf bin coverage.out .tmp dist
