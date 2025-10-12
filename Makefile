.PHONY: all build clean test help proto
.DEFAULT_GOAL := all

# Environment setup
SHELL := /bin/bash
export PATH := $(HOME)/.cargo/bin:$(shell go env GOPATH)/bin:$(PATH)

# Colors for output (using printf-compatible format)
GREEN  := \\033[0;32m
YELLOW := \\033[0;33m
BLUE   := \\033[0;34m
NC     := \\033[0m

# Print helpers
define print_green
	@printf "$(GREEN)✓ %s$(NC)\n" "$(1)"
endef

define print_blue
	@printf "$(BLUE)%s$(NC)\n" "$(1)"
endef

define print_yellow
	@printf "$(YELLOW)%s$(NC)\n" "$(1)"
endef

##@ General

help: ## Display this help message
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

all: proto build ## Build all components (default target)
	$(call print_green,All components built successfully)

##@ Build

build: build-proxy build-patterns ## Build all components

build-proxy: ## Build Rust proxy
	$(call print_blue,Building Rust proxy...)
	@cd proxy && cargo build --release
	$(call print_green,Proxy built)

build-patterns: build-memstore build-redis build-nats ## Build all Go patterns

# Pattern: To add a new pattern, add three targets:
#   build-<pattern>: Build the pattern binary
#   test-<pattern>: Run pattern unit tests
#   coverage-<pattern>: Generate coverage report
# Then add to: build-patterns, test-patterns, coverage-patterns, clean-patterns, fmt-go, lint-go

build-memstore: ## Build MemStore pattern
	$(call print_blue,Building MemStore pattern...)
	@cd patterns/memstore && go build -o memstore cmd/memstore/main.go
	$(call print_green,MemStore built)

build-redis: ## Build Redis pattern
	$(call print_blue,Building Redis pattern...)
	@cd patterns/redis && go build -o redis cmd/redis/main.go
	$(call print_green,Redis built)

build-nats: ## Build NATS pattern
	$(call print_blue,Building NATS pattern...)
	@cd patterns/nats && go build -o nats cmd/nats/main.go
	$(call print_green,NATS built)

build-dev: ## Build all components in debug mode (faster)
	$(call print_blue,Building in debug mode...)
	@cd proxy && cargo build
	@cd patterns/memstore && go build -o memstore cmd/memstore/main.go
	@cd patterns/redis && go build -o redis cmd/redis/main.go
	@cd patterns/nats && go build -o nats cmd/nats/main.go
	$(call print_green,Debug builds complete)

##@ Testing

test: test-proxy test-patterns test-acceptance test-integration-go ## Run all tests (unit, acceptance, integration)
	$(call print_green,All tests passed)

test-proxy: ## Run Rust proxy unit tests
	$(call print_blue,Running Rust proxy tests...)
	@cd proxy && cargo test --lib
	$(call print_green,Proxy unit tests passed)

test-patterns: test-memstore test-redis test-nats test-core ## Run all Go pattern tests

test-memstore: ## Run MemStore tests
	$(call print_blue,Running MemStore tests...)
	@cd patterns/memstore && go test -v -cover ./...
	$(call print_green,MemStore tests passed)

test-redis: ## Run Redis tests
	$(call print_blue,Running Redis tests...)
	@cd patterns/redis && go test -v -cover ./...
	$(call print_green,Redis tests passed)

test-nats: ## Run NATS tests
	$(call print_blue,Running NATS tests...)
	@cd patterns/nats && go test -v -cover ./...
	$(call print_green,NATS tests passed)

test-core: ## Run Core SDK tests
	$(call print_blue,Running Core SDK tests...)
	@cd patterns/core && go test -v -cover ./...
	$(call print_green,Core SDK tests passed)

test-integration: build-memstore ## Run integration tests (requires built MemStore binary)
	$(call print_blue,Running integration tests...)
	@cd proxy && cargo test --test integration_test -- --ignored --nocapture
	$(call print_green,Integration tests passed)

test-all: test test-integration test-integration-go test-acceptance ## Run all tests (unit, integration, acceptance)
	$(call print_green,All tests (unit + integration + acceptance) passed)

test-acceptance: test-acceptance-interfaces test-acceptance-redis test-acceptance-nats ## Run all acceptance tests with testcontainers
	$(call print_green,All acceptance tests passed)

test-acceptance-interfaces: ## Run interface-based acceptance tests (tests multiple backends)
	$(call print_blue,Running interface-based acceptance tests...)
	@cd tests/acceptance/interfaces && go test -v -timeout 10m ./...
	$(call print_green,Interface-based acceptance tests passed)

test-acceptance-redis: ## Run Redis acceptance tests with testcontainers
	$(call print_blue,Running Redis acceptance tests...)
	@cd tests/acceptance/redis && go test -v -timeout 10m ./...
	$(call print_green,Redis acceptance tests passed)

test-acceptance-nats: ## Run NATS acceptance tests with testcontainers
	$(call print_blue,Running NATS acceptance tests...)
	@cd tests/acceptance/nats && go test -v -timeout 10m ./...
	$(call print_green,NATS acceptance tests passed)

test-acceptance-quiet: ## Run all acceptance tests in quiet mode (suppress container logs)
	$(call print_blue,Running acceptance tests in quiet mode...)
	@PRISM_TEST_QUIET=1 $(MAKE) test-acceptance
	$(call print_green,All acceptance tests passed (quiet mode))

test-integration-go: ## Run Go integration tests (proxy-pattern lifecycle)
	$(call print_blue,Running Go integration tests...)
	@cd tests/integration && go test -v -timeout 5m ./...
	$(call print_green,Go integration tests passed)

##@ Code Coverage

coverage: coverage-proxy coverage-patterns ## Generate coverage reports for all components

coverage-proxy: ## Generate Rust proxy coverage report
	$(call print_blue,Generating proxy coverage...)
	@cd proxy && cargo test --lib -- --test-threads=1
	$(call print_green,Proxy coverage report generated)

coverage-patterns: coverage-memstore coverage-redis coverage-nats coverage-core ## Generate coverage for all patterns

coverage-memstore: ## Generate MemStore coverage report
	$(call print_blue,Generating MemStore coverage...)
	@cd patterns/memstore && go test -coverprofile=coverage.out ./...
	@cd patterns/memstore && go tool cover -func=coverage.out | grep total
	@cd patterns/memstore && go tool cover -html=coverage.out -o coverage.html
	$(call print_green,MemStore coverage: patterns/memstore/coverage.html)

coverage-redis: ## Generate Redis coverage report
	$(call print_blue,Generating Redis coverage...)
	@cd patterns/redis && go test -coverprofile=coverage.out ./...
	@cd patterns/redis && go tool cover -func=coverage.out | grep total
	@cd patterns/redis && go tool cover -html=coverage.out -o coverage.html
	$(call print_green,Redis coverage: patterns/redis/coverage.html)

coverage-nats: ## Generate NATS coverage report
	$(call print_blue,Generating NATS coverage...)
	@cd patterns/nats && go test -coverprofile=coverage.out ./...
	@cd patterns/nats && go tool cover -func=coverage.out | grep total
	@cd patterns/nats && go tool cover -html=coverage.out -o coverage.html
	$(call print_green,NATS coverage: patterns/nats/coverage.html)

coverage-core: ## Generate Core SDK coverage report
	$(call print_blue,Generating Core SDK coverage...)
	@cd patterns/core && go test -coverprofile=coverage.out ./...
	@cd patterns/core && go tool cover -func=coverage.out | grep total
	@cd patterns/core && go tool cover -html=coverage.out -o coverage.html
	$(call print_green,Core SDK coverage: patterns/core/coverage.html)

coverage-acceptance: coverage-acceptance-interfaces coverage-acceptance-redis coverage-acceptance-nats ## Generate coverage for acceptance tests

coverage-acceptance-interfaces: ## Generate interface-based acceptance test coverage
	$(call print_blue,Generating interface-based acceptance test coverage...)
	@cd tests/acceptance/interfaces && go test -coverprofile=coverage.out -timeout 10m ./...
	@cd tests/acceptance/interfaces && go tool cover -func=coverage.out | grep total
	@cd tests/acceptance/interfaces && go tool cover -html=coverage.out -o coverage.html
	$(call print_green,Interface acceptance coverage: tests/acceptance/interfaces/coverage.html)

coverage-acceptance-redis: ## Generate Redis acceptance test coverage
	$(call print_blue,Generating Redis acceptance test coverage...)
	@cd tests/acceptance/redis && go test -coverprofile=coverage.out -timeout 10m ./...
	@cd tests/acceptance/redis && go tool cover -func=coverage.out | grep total
	@cd tests/acceptance/redis && go tool cover -html=coverage.out -o coverage.html
	$(call print_green,Redis acceptance coverage: tests/acceptance/redis/coverage.html)

coverage-acceptance-nats: ## Generate NATS acceptance test coverage
	$(call print_blue,Generating NATS acceptance test coverage...)
	@cd tests/acceptance/nats && go test -coverprofile=coverage.out -timeout 10m ./...
	@cd tests/acceptance/nats && go tool cover -func=coverage.out | grep total
	@cd tests/acceptance/nats && go tool cover -html=coverage.out -o coverage.html
	$(call print_green,NATS acceptance coverage: tests/acceptance/nats/coverage.html)

coverage-integration: ## Generate Go integration test coverage
	$(call print_blue,Generating Go integration test coverage...)
	@cd tests/integration && go test -coverprofile=coverage.out -timeout 5m ./...
	@cd tests/integration && go tool cover -func=coverage.out | grep total
	@cd tests/integration && go tool cover -html=coverage.out -o coverage.html
	$(call print_green,Integration coverage: tests/integration/coverage.html)

##@ Protobuf

proto: proto-go proto-rust ## Generate protobuf code for all languages

proto-go: ## Generate Go protobuf code
	$(call print_blue,Generating Go protobuf code...)
	@mkdir -p patterns/core/gen
	@cd proto && buf generate --template buf.gen.go.yaml
	$(call print_green,Go protobuf code generated)

proto-rust: ## Generate Rust protobuf code (via build.rs)
	$(call print_blue,Rust protobuf code generated via build.rs during cargo build)

##@ Cleaning

clean: clean-proxy clean-patterns clean-proto ## Clean all build artifacts
	$(call print_green,All artifacts cleaned)

clean-proxy: ## Clean Rust proxy artifacts
	$(call print_blue,Cleaning proxy...)
	@cd proxy && cargo clean
	$(call print_green,Proxy cleaned)

clean-patterns: ## Clean pattern binaries
	$(call print_blue,Cleaning patterns...)
	@rm -f patterns/memstore/memstore
	@rm -f patterns/memstore/coverage.out patterns/memstore/coverage.html
	@rm -f patterns/redis/redis
	@rm -f patterns/redis/coverage.out patterns/redis/coverage.html
	@rm -f patterns/nats/nats
	@rm -f patterns/nats/coverage.out patterns/nats/coverage.html
	@rm -f patterns/core/coverage.out patterns/core/coverage.html
	@rm -f tests/acceptance/interfaces/coverage.out tests/acceptance/interfaces/coverage.html
	@rm -f tests/acceptance/redis/coverage.out tests/acceptance/redis/coverage.html
	@rm -f tests/acceptance/nats/coverage.out tests/acceptance/nats/coverage.html
	@rm -f tests/integration/coverage.out tests/integration/coverage.html
	$(call print_green,Patterns cleaned)

clean-proto: ## Clean generated protobuf code
	$(call print_blue,Cleaning generated protobuf code...)
	@rm -rf patterns/core/gen
	$(call print_green,Generated protobuf code cleaned)

##@ Development

watch-proxy: ## Watch and rebuild proxy on changes (requires cargo-watch)
	@cd proxy && cargo watch -x build

watch-test: ## Watch and rerun tests on changes (requires cargo-watch)
	@cd proxy && cargo watch -x test

fmt: fmt-rust fmt-go ## Format all code

fmt-rust: ## Format Rust code
	$(call print_blue,Formatting Rust code...)
	@cd proxy && cargo fmt
	$(call print_green,Rust code formatted)

fmt-go: ## Format Go code
	$(call print_blue,Formatting Go code...)
	@cd patterns/memstore && go fmt ./...
	@cd patterns/redis && go fmt ./...
	@cd patterns/nats && go fmt ./...
	@cd patterns/core && go fmt ./...
	@cd tests/acceptance/interfaces && go fmt ./...
	@cd tests/acceptance/redis && go fmt ./...
	@cd tests/acceptance/nats && go fmt ./...
	@cd tests/integration && go fmt ./...
	$(call print_green,Go code formatted)

lint: lint-rust lint-go ## Lint all code

lint-rust: ## Lint Rust code with clippy
	$(call print_blue,Linting Rust code...)
	@cd proxy && cargo clippy -- -D warnings
	$(call print_green,Rust code linted)

lint-go: ## Lint Go code
	$(call print_blue,Linting Go code...)
	@cd patterns/memstore && go vet ./...
	@cd patterns/redis && go vet ./...
	@cd patterns/nats && go vet ./...
	@cd patterns/core && go vet ./...
	@cd tests/acceptance/interfaces && go vet ./...
	@cd tests/acceptance/redis && go vet ./...
	@cd tests/acceptance/nats && go vet ./...
	@cd tests/integration && go vet ./...
	$(call print_green,Go code linted)

##@ Docker

docker-up: ## Start local development services (Redis, NATS)
	$(call print_blue,Starting local development services...)
	@docker-compose up -d
	$(call print_green,Services started - Redis: localhost:6379, NATS: localhost:4222)

docker-down: ## Stop local development services
	$(call print_blue,Stopping local development services...)
	@docker-compose down
	$(call print_green,Services stopped)

docker-logs: ## Show logs from local services
	@docker-compose logs -f

docker-redis-cli: ## Open Redis CLI (requires docker-up)
	@docker-compose run --rm redis-cli

##@ Documentation

docs: docs-validate docs-build ## Validate and build documentation

docs-validate: ## Validate documentation with strict checks
	$(call print_blue,Validating documentation...)
	@uv run tooling/validate_docs.py
	$(call print_green,Documentation validated)

docs-build: ## Build Docusaurus documentation site
	$(call print_blue,Building documentation site...)
	@cd docusaurus && npm run build
	$(call print_green,Documentation built)

docs-serve: ## Serve documentation locally
	$(call print_blue,Starting documentation server...)
	@cd docusaurus && npm run serve

##@ Installation

install-tools: ## Install development tools
	$(call print_blue,Installing development tools...)
	$(call print_yellow,Installing Rust tools...)
	@rustup component add rustfmt clippy
	@cargo install cargo-watch || true
	$(call print_yellow,Installing Go tools...)
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	$(call print_yellow,Installing Python tools...)
	@curl -LsSf https://astral.sh/uv/install.sh | sh || true
	@uv sync
	$(call print_green,Development tools installed)

##@ CI/CD

ci-full: clean-proto proto-go lint test-all test-acceptance docs-validate docs-build ## Run complete CI pipeline (clean, proto, lint, all tests, docs validate+build)
	$(call print_green,Complete CI pipeline passed - codebase validated)

ci: lint test-all test-acceptance docs-validate ## Run full CI pipeline (lint, test, acceptance, validate docs)
	$(call print_green,CI pipeline passed)

pre-commit: fmt lint test ## Run pre-commit checks (format, lint, test)
	$(call print_green,Pre-commit checks passed)
