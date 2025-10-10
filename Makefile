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

build-patterns: build-memstore build-redis ## Build all Go patterns

build-memstore: ## Build MemStore pattern
	$(call print_blue,Building MemStore pattern...)
	@cd patterns/memstore && go build -o memstore cmd/memstore/main.go
	$(call print_green,MemStore built)

build-redis: ## Build Redis pattern
	$(call print_blue,Building Redis pattern...)
	@cd patterns/redis && go build -o redis cmd/redis/main.go
	$(call print_green,Redis built)

build-dev: ## Build all components in debug mode (faster)
	$(call print_blue,Building in debug mode...)
	@cd proxy && cargo build
	@cd patterns/memstore && go build -o memstore cmd/memstore/main.go
	@cd patterns/redis && go build -o redis cmd/redis/main.go
	$(call print_green,Debug builds complete)

##@ Testing

test: test-proxy test-patterns ## Run all tests
	$(call print_green,All tests passed)

test-proxy: ## Run Rust proxy unit tests
	$(call print_blue,Running Rust proxy tests...)
	@cd proxy && cargo test --lib
	$(call print_green,Proxy unit tests passed)

test-patterns: test-memstore test-redis test-core ## Run all Go pattern tests

test-memstore: ## Run MemStore tests
	$(call print_blue,Running MemStore tests...)
	@cd patterns/memstore && go test -v -cover ./...
	$(call print_green,MemStore tests passed)

test-redis: ## Run Redis tests
	$(call print_blue,Running Redis tests...)
	@cd patterns/redis && go test -v -cover ./...
	$(call print_green,Redis tests passed)

test-core: ## Run Core SDK tests
	$(call print_blue,Running Core SDK tests...)
	@cd patterns/core && go test -v -cover ./...
	$(call print_green,Core SDK tests passed)

test-integration: build-memstore ## Run integration tests (requires built MemStore binary)
	$(call print_blue,Running integration tests...)
	@cd proxy && cargo test --test integration_test -- --ignored --nocapture
	$(call print_green,Integration tests passed)

test-all: test test-integration ## Run all tests including integration tests
	$(call print_green,All tests (unit + integration) passed)

##@ Code Coverage

coverage: coverage-proxy coverage-patterns ## Generate coverage reports for all components

coverage-proxy: ## Generate Rust proxy coverage report
	$(call print_blue,Generating proxy coverage...)
	@cd proxy && cargo test --lib -- --test-threads=1
	$(call print_green,Proxy coverage report generated)

coverage-patterns: coverage-memstore coverage-redis coverage-core ## Generate coverage for all patterns

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

coverage-core: ## Generate Core SDK coverage report
	$(call print_blue,Generating Core SDK coverage...)
	@cd patterns/core && go test -coverprofile=coverage.out ./...
	@cd patterns/core && go tool cover -func=coverage.out | grep total
	@cd patterns/core && go tool cover -html=coverage.out -o coverage.html
	$(call print_green,Core SDK coverage: patterns/core/coverage.html)

##@ Protobuf

proto: proto-go proto-rust ## Generate protobuf code for all languages

proto-go: ## Generate Go protobuf code
	$(call print_blue,Generating Go protobuf code...)
	@mkdir -p patterns/core/gen
	@PATH="$$PATH:$$(go env GOPATH)/bin" protoc \
		--go_out=patterns/core/gen \
		--go_opt=paths=source_relative \
		--go-grpc_out=patterns/core/gen \
		--go-grpc_opt=paths=source_relative \
		--proto_path=proto \
		proto/prism/pattern/lifecycle.proto \
		proto/prism/common/types.proto
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
	@rm -f patterns/core/coverage.out patterns/core/coverage.html
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
	@cd patterns/core && go fmt ./...
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
	@cd patterns/core && go vet ./...
	$(call print_green,Go code linted)

##@ Docker

docker-up: ## Start local development services (Redis)
	$(call print_blue,Starting local development services...)
	@docker-compose up -d
	$(call print_green,Services started - Redis available at localhost:6379)

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

ci: lint test-all docs-validate ## Run full CI pipeline (lint, test, validate docs)
	$(call print_green,CI pipeline passed)

pre-commit: fmt lint test ## Run pre-commit checks (format, lint, test)
	$(call print_green,Pre-commit checks passed)
