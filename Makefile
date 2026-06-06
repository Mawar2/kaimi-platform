# Makefile for Kaimi - autonomous BD pipeline for federal contracting
#
# Common targets:
#   make build      - Build all binaries
#   make test       - Run all tests
#   make lint       - Run linter
#   make clean      - Remove build artifacts
#   make all        - Build, test, and lint (use before PR)

.PHONY: all build test lint clean help

# Default target
all: build test lint

# Build all binaries
build:
	@echo "Building Kaimi..."
	go build -v ./...
	@echo "Building Hunter agent..."
	go build -v -o bin/hunter ./cmd/hunter
	@echo "Building Scorer agent..."
	go build -v -o bin/scorer ./cmd/scorer
	@echo "Build complete."

# Run all tests
test:
	@echo "Running tests..."
	go test -v -race -cover ./...
	@echo "Tests complete."

# Run linter (requires golangci-lint to be installed)
lint:
	@echo "Running linter..."
	golangci-lint run
	@echo "Lint complete."

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	go clean -cache
	@echo "Clean complete."

# Help target
help:
	@echo "Kaimi - Autonomous BD Pipeline for Federal Contracting"
	@echo ""
	@echo "Available targets:"
	@echo "  make build      - Build all binaries"
	@echo "  make test       - Run all tests"
	@echo "  make lint       - Run linter (requires golangci-lint)"
	@echo "  make clean      - Remove build artifacts"
	@echo "  make all        - Build, test, and lint (recommended before PR)"
	@echo "  make help       - Show this help message"
