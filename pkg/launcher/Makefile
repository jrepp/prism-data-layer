.PHONY: help test test-short test-coverage test-integration clean build install examples lint fmt vet

# Default target
help:
	@echo "Pattern Launcher Development Makefile"
	@echo ""
	@echo "Available targets:"
	@echo "  help              Show this help message"
	@echo "  test              Run all tests (unit + integration)"
	@echo "  test-short        Run only fast unit tests"
	@echo "  test-coverage     Generate test coverage report"
	@echo "  test-integration  Run only integration tests"
	@echo "  clean             Clean build artifacts and test output"
	@echo "  build             Build test pattern binary"
	@echo "  install           Install launcher (to GOPATH/bin)"
	@echo "  examples          Build all example programs"
	@echo "  lint              Run linters"
	@echo "  fmt               Format code"
	@echo "  vet               Run go vet"
	@echo ""
	@echo "Examples:"
	@echo "  make test-short        # Quick feedback during development"
	@echo "  make test-coverage     # Generate HTML coverage report"
	@echo "  make build examples    # Build test pattern and examples"

# Run all tests (unit + integration)
test:
	@echo "Running all tests..."
	go test -v -race ./...

# Run only fast unit tests (skip integration)
test-short:
	@echo "Running unit tests (short mode)..."
	go test -v -short -race ./...

# Generate test coverage report
test-coverage:
	@echo "Generating coverage report..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"
	@go tool cover -func=coverage.out | grep total
	@echo ""
	@echo "Open coverage.html in your browser to see detailed coverage"

# Run only integration tests
test-integration:
	@echo "Running integration tests..."
	go test -v -run Integration ./...

# Clean build artifacts and test output
clean:
	@echo "Cleaning build artifacts..."
	rm -f coverage.out coverage.html
	rm -f ../../patterns/test-pattern/test-pattern
	rm -f examples/basic_launch examples/embedded_launcher
	rm -f examples/isolation_levels examples/metrics_monitoring
	@echo "Clean complete"

# Build test pattern binary
build:
	@echo "Building test pattern..."
	cd ../../patterns/test-pattern && go build -o test-pattern main.go
	chmod +x ../../patterns/test-pattern/test-pattern
	@echo "Test pattern built: patterns/test-pattern/test-pattern"

# Install launcher (assumes cmd/pattern-launcher exists in project root)
install:
	@echo "Installing pattern-launcher..."
	cd ../../cmd/pattern-launcher && go install
	@echo "Installed to: $$(which pattern-launcher)"

# Build all example programs
examples:
	@echo "Building examples..."
	@cd examples && go build -o basic_launch basic_launch.go
	@cd examples && go build -o embedded_launcher embedded_launcher.go
	@cd examples && go build -o isolation_levels isolation_levels.go
	@cd examples && go build -o metrics_monitoring metrics_monitoring.go
	@echo "Examples built in examples/ directory"

# Run linters
lint:
	@echo "Running linters..."
	golangci-lint run ./...

# Format code
fmt:
	@echo "Formatting code..."
	gofmt -w -s .
	goimports -w .

# Run go vet
vet:
	@echo "Running go vet..."
	go vet ./...

# Run all checks (vet + lint + tests)
check: vet lint test-short
	@echo ""
	@echo "All checks passed! ✓"

# Quick development cycle (format + test + build)
dev: fmt test-short build
	@echo ""
	@echo "Development cycle complete! ✓"

# CI target (all checks + coverage)
ci: vet lint test test-coverage
	@echo ""
	@echo "CI checks passed! ✓"
