set dotenv-load := false
set positional-arguments

# Default recipe: list available recipes
default:
    @just --list

# Run CLI in human mode (default)
run *args:
    go run ./cmd/remarkable-cli "$@"

# Run CLI in agent-facing mode
run-ai *args:
    AGENT=1 go run ./cmd/remarkable-cli "$@"

# Run test suite
test:
    go test -race -v ./...

# Build binary into bin/
build:
    mkdir -p bin
    go build -o bin/remarkable-cli ./cmd/remarkable-cli

# Install binary to GOPATH bin directory
install:
    go install ./cmd/remarkable-cli

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
