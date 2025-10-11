# MemStore Plugin Makefile
# Thinnest slice implementation for pattern SDK

.PHONY: all build test lint fmt clean help docker-compare docker-scratch docker-upx docker-distroless

# Binary name
BINARY := memstore
BUILD_DIR := bin

# Go commands
GOTEST := go test
GOBUILD := go build
GOCLEAN := go clean
GOFMT := go fmt
GOLINT := golangci-lint run

# Build flags
LDFLAGS := -ldflags="-s -w"
BUILD_FLAGS := CGO_ENABLED=0 GOOS=linux GOARCH=amd64

# Default target
all: fmt lint test build

help:
	@echo "MemStore Plugin Build System"
	@echo "============================="
	@echo ""
	@echo "Targets:"
	@echo "  all             - Run fmt, lint, test, and build (default)"
	@echo "  build           - Build the memstore binary"
	@echo "  test            - Run all tests"
	@echo "  test-cover      - Run tests with coverage report"
	@echo "  lint            - Run linters"
	@echo "  fmt             - Format code"
	@echo "  clean           - Remove build artifacts"
	@echo "  run             - Run memstore locally"
	@echo ""
	@echo "Docker targets:"
	@echo "  docker-compare     - Build and compare all image size variants"
	@echo "  docker-minimal     - Build minimal variant (~7-8 MB)"
	@echo "  docker-upx         - Build UPX compressed variant (~3-4 MB)"
	@echo "  docker-distroless  - Build distroless variant (~9-10 MB)"
	@echo ""
	@echo "Examples:"
	@echo "  make all              # Full build pipeline"
	@echo "  make test             # Run tests only"
	@echo "  make test-cover       # Run tests with coverage"
	@echo "  make docker-compare   # Compare Docker image sizes"

# Build binary
build:
	@echo "Building $(BINARY)..."
	@mkdir -p $(BUILD_DIR)
	@$(BUILD_FLAGS) $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/memstore
	@echo "✓ Binary built: $(BUILD_DIR)/$(BINARY)"

# Run tests
test:
	@echo "Running tests..."
	@$(GOTEST) -v -race -short ./...
	@echo "✓ Tests passed"

# Run tests with coverage
test-cover:
	@echo "Running tests with coverage..."
	@$(GOTEST) -v -race -coverprofile=coverage.out -covermode=atomic ./...
	@go tool cover -html=coverage.out -o coverage.html
	@go tool cover -func=coverage.out | grep total
	@echo "✓ Coverage report: coverage.html"

# Run linters
lint:
	@echo "Running linters..."
	@$(GOLINT) ./...
	@echo "✓ Linting passed"

# Format code
fmt:
	@echo "Formatting code..."
	@$(GOFMT) ./...
	@echo "✓ Code formatted"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@$(GOCLEAN)
	@echo "✓ Cleaned"

# Run locally
run: build
	@echo "Running $(BINARY)..."
	@./$(BUILD_DIR)/$(BINARY) -config config.yaml

# Install dependencies
deps:
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy
	@echo "✓ Dependencies installed"

# Docker image size comparison
docker-compare:
	@echo "Building and comparing Docker image sizes..."
	@./build-compare.sh

# Build individual Docker variants
BUILDER ?= docker

docker-minimal:
	@echo "Building MINIMAL variant (smallest possible)..."
	@cd .. && $(BUILDER) build --target minimal -t prism/memstore:minimal -f memstore/Dockerfile.optimized .
	@echo "✓ Image size:"
	@$(BUILDER) images prism/memstore:minimal --format "{{.Size}}"

docker-upx:
	@echo "Building UPX variant (compressed)..."
	@cd .. && $(BUILDER) build --target upx-compressed -t prism/memstore:upx -f memstore/Dockerfile.optimized .
	@echo "✓ Image size:"
	@$(BUILDER) images prism/memstore:upx --format "{{.Size}}"

docker-distroless:
	@echo "Building DISTROLESS variant (current production)..."
	@cd .. && $(BUILDER) build --target production -t prism/memstore:distroless -f memstore/Dockerfile.optimized .
	@echo "✓ Image size:"
	@$(BUILDER) images prism/memstore:distroless --format "{{.Size}}"
