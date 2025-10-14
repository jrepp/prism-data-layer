.PHONY: all build clean test help proto
.DEFAULT_GOAL := all

# Environment setup
SHELL := /bin/bash
export PATH := $(HOME)/.cargo/bin:$(shell go env GOPATH)/bin:$(PATH)

# Docker/Podman setup for testcontainers (ADR-049: We use Podman instead of Docker Desktop)
# testcontainers-go needs DOCKER_HOST to find the Podman socket for acceptance tests
ifndef DOCKER_HOST
  PODMAN_SOCKET := $(shell podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}' 2>/dev/null)
  ifneq ($(PODMAN_SOCKET),)
    export DOCKER_HOST := unix://$(PODMAN_SOCKET)
  endif
endif

# Build directories (hygienic out-of-source builds)
BUILD_DIR := $(CURDIR)/build
BINARIES_DIR := $(BUILD_DIR)/binaries
COVERAGE_DIR := $(BUILD_DIR)/coverage
TEST_LOGS_DIR := $(BUILD_DIR)/test-logs
DOCS_BUILD_DIR := $(BUILD_DIR)/docs
RUST_TARGET_DIR := $(BUILD_DIR)/rust/target

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
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9_-]+:.*##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

env: ## Show build environment variables
	$(call print_blue,Build Environment:)
	@echo "  SHELL:       $(SHELL)"
	@echo "  PATH:        $(PATH)"
	@echo "  DOCKER_HOST: $(DOCKER_HOST)"
	@echo "  BUILD_DIR:   $(BUILD_DIR)"
	@echo "  GO version:  $(shell go version)"
	@echo "  Rust:        $(shell rustc --version 2>/dev/null || echo 'not found')"
	@echo "  Podman:      $(shell podman --version 2>/dev/null || echo 'not found')"

all: proto build ## Build all components (default target)
	$(call print_green,All components built successfully)

##@ Build

build: build-proxy build-patterns build-prismctl ## Build all components

build-proxy: ## Build Rust proxy
	$(call print_blue,Building Rust proxy...)
	@mkdir -p $(BINARIES_DIR)
	@cd prism-proxy && CARGO_TARGET_DIR=$(RUST_TARGET_DIR) cargo build --release
	@cp $(RUST_TARGET_DIR)/release/prism-proxy $(BINARIES_DIR)/prism-proxy
	$(call print_green,Proxy built: $(BINARIES_DIR)/prism-proxy)

build-patterns: build-memstore build-redis build-nats ## Build all Go patterns

# Pattern: To add a new pattern, add three targets:
#   build-<pattern>: Build the pattern binary
#   test-<pattern>: Run pattern unit tests
#   coverage-<pattern>: Generate coverage report
# Then add to: build-patterns, test-patterns, coverage-patterns, clean-patterns, fmt-go, lint-go

build-memstore: ## Build MemStore pattern
	$(call print_blue,Building MemStore pattern...)
	@mkdir -p $(BINARIES_DIR)
	@cd patterns/memstore && go build -o $(BINARIES_DIR)/memstore cmd/memstore/main.go
	$(call print_green,MemStore built: $(BINARIES_DIR)/memstore)

build-redis: ## Build Redis pattern
	$(call print_blue,Building Redis pattern...)
	@mkdir -p $(BINARIES_DIR)
	@cd patterns/redis && go build -o $(BINARIES_DIR)/redis cmd/redis/main.go
	$(call print_green,Redis built: $(BINARIES_DIR)/redis)

build-nats: ## Build NATS pattern
	$(call print_blue,Building NATS pattern...)
	@mkdir -p $(BINARIES_DIR)
	@cd patterns/nats && go build -o $(BINARIES_DIR)/nats cmd/nats/main.go
	$(call print_green,NATS built: $(BINARIES_DIR)/nats)

build-prismctl: ## Build prismctl CLI
	$(call print_blue,Building prismctl CLI...)
	@mkdir -p $(BINARIES_DIR)
	@cd prismctl && go build -o $(BINARIES_DIR)/prismctl .
	$(call print_green,prismctl built: $(BINARIES_DIR)/prismctl)

build-dev: ## Build all components in debug mode (faster)
	$(call print_blue,Building in debug mode...)
	@mkdir -p $(BINARIES_DIR)
	@cd prism-proxy && CARGO_TARGET_DIR=$(RUST_TARGET_DIR) cargo build
	@cp $(RUST_TARGET_DIR)/debug/prism-proxy $(BINARIES_DIR)/prism-proxy-debug
	@cd patterns/memstore && go build -o $(BINARIES_DIR)/memstore-debug cmd/memstore/main.go
	@cd patterns/redis && go build -o $(BINARIES_DIR)/redis-debug cmd/redis/main.go
	@cd patterns/nats && go build -o $(BINARIES_DIR)/nats-debug cmd/nats/main.go
	$(call print_green,Debug builds complete: $(BINARIES_DIR)/*-debug)

##@ Testing

test: test-proxy test-patterns test-acceptance test-integration-go ## Run all tests (unit, acceptance, integration)
	$(call print_green,All tests passed)

test-parallel: ## Run all tests in parallel (fast!)
	$(call print_blue,Running tests in parallel...)
	@uv run tooling/parallel_test.py --log-dir $(TEST_LOGS_DIR)
	$(call print_green,Parallel tests complete)

test-parallel-fast: ## Run fast tests only in parallel (skip acceptance)
	$(call print_blue,Running fast tests in parallel...)
	@uv run tooling/parallel_test.py --fast --log-dir $(TEST_LOGS_DIR)
	$(call print_green,Fast parallel tests complete)

test-parallel-fail-fast: ## Run tests in parallel with fail-fast
	$(call print_blue,Running tests in parallel with fail-fast...)
	@uv run tooling/parallel_test.py --fail-fast --log-dir $(TEST_LOGS_DIR)
	$(call print_green,Parallel tests complete)

test-proxy: ## Run Rust proxy unit tests
	$(call print_blue,Running Rust proxy tests...)
	@cd prism-proxy && cargo test --lib
	$(call print_green,Proxy unit tests passed)

test-patterns: test-memstore test-redis test-nats test-core test-prismctl ## Run all Go pattern tests

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

test-prismctl: ## Run prismctl tests
	$(call print_blue,Running prismctl tests...)
	@cd prismctl && go test -v -cover ./...
	$(call print_green,prismctl tests passed)

test-integration: build-memstore ## Run integration tests (requires built MemStore binary)
	$(call print_blue,Running integration tests...)
	@cd prism-proxy && cargo test --test integration_test -- --ignored --nocapture
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

test-acceptance-parallel: ## Run acceptance tests in parallel with matrix report
	$(call print_blue,Running acceptance tests in parallel...)
	@mkdir -p $(BUILD_DIR)/reports
	@uv run tooling/parallel_acceptance_test.py
	$(call print_green,Parallel acceptance tests complete)

test-acceptance-parallel-report: ## Run acceptance tests in parallel and save matrix report
	$(call print_blue,Running acceptance tests and generating reports...)
	@mkdir -p $(BUILD_DIR)/reports
	@uv run tooling/parallel_acceptance_test.py --format markdown --output $(BUILD_DIR)/reports/acceptance-matrix.md
	@uv run tooling/parallel_acceptance_test.py --format json --output $(BUILD_DIR)/reports/acceptance-results.json
	$(call print_green,Reports saved: $(BUILD_DIR)/reports/acceptance-matrix.md, $(BUILD_DIR)/reports/acceptance-results.json)

test-acceptance-parallel-backends: ## Run acceptance tests for specific backends (use BACKENDS=MemStore,Redis)
	$(call print_blue,Running acceptance tests for backends: $(BACKENDS)...)
	@uv run tooling/parallel_acceptance_test.py --backends $(BACKENDS)
	$(call print_green,Backend acceptance tests complete)

test-acceptance-parallel-patterns: ## Run acceptance tests for specific patterns (use PATTERNS=KeyValueBasic,KeyValueTTL)
	$(call print_blue,Running acceptance tests for patterns: $(PATTERNS)...)
	@uv run tooling/parallel_acceptance_test.py --patterns $(PATTERNS)
	$(call print_green,Pattern acceptance tests complete)

test-integration-go: ## Run Go integration tests (proxy-pattern lifecycle)
	$(call print_blue,Running Go integration tests...)
	@cd tests/integration && go test -v -timeout 5m ./...
	$(call print_green,Go integration tests passed)

test-prismctl-integration: podman-start ## Run prismctl OIDC integration tests with Dex
	$(call print_blue,Starting Dex test server...)
	@cd cli/tests/integration && $(COMPOSE) -f docker-compose.dex.yml up -d
	@sleep 5
	$(call print_blue,Running prismctl integration tests...)
	@cd cli && uv run pytest tests/integration/ -v --cov=prismctl.auth --cov-report=term-missing || (cd tests/integration && $(COMPOSE) -f docker-compose.dex.yml down && exit 1)
	$(call print_blue,Stopping Dex test server...)
	@cd cli/tests/integration && $(COMPOSE) -f docker-compose.dex.yml down
	$(call print_green,Prismctl integration tests passed)

##@ Code Coverage

coverage: coverage-proxy coverage-patterns ## Generate coverage reports for all components

coverage-proxy: ## Generate Rust proxy coverage report
	$(call print_blue,Generating proxy coverage...)
	@cd prism-proxy && cargo test --lib -- --test-threads=1
	$(call print_green,Proxy coverage report generated)

coverage-patterns: coverage-memstore coverage-redis coverage-nats coverage-core ## Generate coverage for all patterns

coverage-memstore: ## Generate MemStore coverage report
	$(call print_blue,Generating MemStore coverage...)
	@mkdir -p $(COVERAGE_DIR)/memstore
	@cd patterns/memstore && go test -coverprofile=../../build/coverage/memstore/coverage.out ./...
	@cd patterns/memstore && go tool cover -func=../../build/coverage/memstore/coverage.out | grep total
	@cd patterns/memstore && go tool cover -html=../../build/coverage/memstore/coverage.out -o ../../build/coverage/memstore/coverage.html
	$(call print_green,MemStore coverage: $(COVERAGE_DIR)/memstore/coverage.html)

coverage-redis: ## Generate Redis coverage report
	$(call print_blue,Generating Redis coverage...)
	@mkdir -p $(COVERAGE_DIR)/redis
	@cd patterns/redis && go test -coverprofile=../../build/coverage/redis/coverage.out ./...
	@cd patterns/redis && go tool cover -func=../../build/coverage/redis/coverage.out | grep total
	@cd patterns/redis && go tool cover -html=../../build/coverage/redis/coverage.out -o ../../build/coverage/redis/coverage.html
	$(call print_green,Redis coverage: $(COVERAGE_DIR)/redis/coverage.html)

coverage-nats: ## Generate NATS coverage report
	$(call print_blue,Generating NATS coverage...)
	@mkdir -p $(COVERAGE_DIR)/nats
	@cd patterns/nats && go test -coverprofile=../../build/coverage/nats/coverage.out ./...
	@cd patterns/nats && go tool cover -func=../../build/coverage/nats/coverage.out | grep total
	@cd patterns/nats && go tool cover -html=../../build/coverage/nats/coverage.out -o ../../build/coverage/nats/coverage.html
	$(call print_green,NATS coverage: $(COVERAGE_DIR)/nats/coverage.html)

coverage-core: ## Generate Core SDK coverage report
	$(call print_blue,Generating Core SDK coverage...)
	@mkdir -p $(COVERAGE_DIR)/core
	@cd patterns/core && go test -coverprofile=../../build/coverage/core/coverage.out ./...
	@cd patterns/core && go tool cover -func=../../build/coverage/core/coverage.out | grep total
	@cd patterns/core && go tool cover -html=../../build/coverage/core/coverage.out -o ../../build/coverage/core/coverage.html
	$(call print_green,Core SDK coverage: $(COVERAGE_DIR)/core/coverage.html)

coverage-acceptance: coverage-acceptance-interfaces coverage-acceptance-redis coverage-acceptance-nats ## Generate coverage for acceptance tests

coverage-acceptance-interfaces: ## Generate interface-based acceptance test coverage
	$(call print_blue,Generating interface-based acceptance test coverage...)
	@mkdir -p $(COVERAGE_DIR)/acceptance/interfaces
	@cd tests/acceptance/interfaces && go test -coverprofile=../../../build/coverage/acceptance/interfaces/coverage.out -timeout 10m ./...
	@cd tests/acceptance/interfaces && go tool cover -func=../../../build/coverage/acceptance/interfaces/coverage.out | grep total
	@cd tests/acceptance/interfaces && go tool cover -html=../../../build/coverage/acceptance/interfaces/coverage.out -o ../../../build/coverage/acceptance/interfaces/coverage.html
	$(call print_green,Interface acceptance coverage: $(COVERAGE_DIR)/acceptance/interfaces/coverage.html)

coverage-acceptance-redis: ## Generate Redis acceptance test coverage
	$(call print_blue,Generating Redis acceptance test coverage...)
	@mkdir -p $(COVERAGE_DIR)/acceptance/redis
	@cd tests/acceptance/redis && go test -coverprofile=../../../build/coverage/acceptance/redis/coverage.out -timeout 10m ./...
	@cd tests/acceptance/redis && go tool cover -func=../../../build/coverage/acceptance/redis/coverage.out | grep total
	@cd tests/acceptance/redis && go tool cover -html=../../../build/coverage/acceptance/redis/coverage.out -o ../../../build/coverage/acceptance/redis/coverage.html
	$(call print_green,Redis acceptance coverage: $(COVERAGE_DIR)/acceptance/redis/coverage.html)

coverage-acceptance-nats: ## Generate NATS acceptance test coverage
	$(call print_blue,Generating NATS acceptance test coverage...)
	@mkdir -p $(COVERAGE_DIR)/acceptance/nats
	@cd tests/acceptance/nats && go test -coverprofile=../../../build/coverage/acceptance/nats/coverage.out -timeout 10m ./...
	@cd tests/acceptance/nats && go tool cover -func=../../../build/coverage/acceptance/nats/coverage.out | grep total
	@cd tests/acceptance/nats && go tool cover -html=../../../build/coverage/acceptance/nats/coverage.out -o ../../../build/coverage/acceptance/nats/coverage.html
	$(call print_green,NATS acceptance coverage: $(COVERAGE_DIR)/acceptance/nats/coverage.html)

coverage-integration: ## Generate Go integration test coverage
	$(call print_blue,Generating Go integration test coverage...)
	@mkdir -p $(COVERAGE_DIR)/integration
	@cd tests/integration && go test -coverprofile=../../build/coverage/integration/coverage.out -timeout 5m ./...
	@cd tests/integration && go tool cover -func=../../build/coverage/integration/coverage.out | grep total
	@cd tests/integration && go tool cover -html=../../build/coverage/integration/coverage.out -o ../../build/coverage/integration/coverage.html
	$(call print_green,Integration coverage: $(COVERAGE_DIR)/integration/coverage.html)

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

clean: clean-build clean-proto ## Clean all build artifacts
	$(call print_green,All artifacts cleaned)

clean-build: ## Clean build directory (recommended)
	$(call print_blue,Cleaning build directory...)
	@rm -rf $(BUILD_DIR)
	$(call print_green,Build directory cleaned: $(BUILD_DIR))

clean-legacy: ## Clean legacy in-source build artifacts (deprecated)
	$(call print_blue,Cleaning legacy build artifacts...)
	@rm -f patterns/memstore/memstore patterns/memstore/coverage.out patterns/memstore/coverage.html
	@rm -f patterns/redis/redis patterns/redis/coverage.out patterns/redis/coverage.html
	@rm -f patterns/nats/nats patterns/nats/coverage.out patterns/nats/coverage.html
	@rm -f patterns/core/coverage.out patterns/core/coverage.html
	@rm -f tests/acceptance/interfaces/coverage.out tests/acceptance/interfaces/coverage.html
	@rm -f tests/acceptance/redis/coverage.out tests/acceptance/redis/coverage.html
	@rm -f tests/acceptance/nats/coverage.out tests/acceptance/nats/coverage.html
	@rm -f tests/integration/coverage.out tests/integration/coverage.html
	@rm -rf test-logs/ prism-proxy/target/
	$(call print_green,Legacy artifacts cleaned)

clean-proto: ## Clean generated protobuf code
	$(call print_blue,Cleaning generated protobuf code...)
	@rm -rf patterns/core/gen
	$(call print_green,Generated protobuf code cleaned)

##@ Development

watch-proxy: ## Watch and rebuild proxy on changes (requires cargo-watch)
	@cd prism-proxy && cargo watch -x build

watch-test: ## Watch and rerun tests on changes (requires cargo-watch)
	@cd prism-proxy && cargo watch -x test

fmt: fmt-rust fmt-go fmt-python ## Format all code

fmt-rust: ## Format Rust code
	$(call print_blue,Formatting Rust code...)
	@cd prism-proxy && cargo fmt
	$(call print_green,Rust code formatted)

fmt-go: ## Format Go code
	$(call print_blue,Formatting Go code...)
	@cd patterns/memstore && go fmt ./...
	@cd patterns/redis && go fmt ./...
	@cd patterns/nats && go fmt ./...
	@cd patterns/core && go fmt ./...
	@cd prismctl && go fmt ./...
	@cd tests/acceptance/interfaces && go fmt ./...
	@cd tests/acceptance/redis && go fmt ./...
	@cd tests/acceptance/nats && go fmt ./...
	@cd tests/integration && go fmt ./...
	$(call print_green,Go code formatted)

fmt-python: ## Format Python code with ruff
	$(call print_blue,Formatting Python code...)
	@uv run ruff format tooling/
	$(call print_green,Python code formatted)

lint: lint-rust lint-go lint-python lint-workflows ## Lint all code and workflows

lint-rust: ## Lint Rust code with clippy
	$(call print_blue,Linting Rust code...)
	@cd prism-proxy && cargo clippy -- -D warnings
	$(call print_green,Rust code linted)

lint-python: lint-python-ruff lint-python-pylint ## Lint Python code with ruff and pylint

lint-python-ruff: ## Lint Python code with ruff (includes format check)
	$(call print_blue,Linting Python code with ruff...)
	@uv run ruff check tooling/
	@uv run ruff format --check tooling/
	$(call print_green,Ruff checks passed)

lint-python-pylint: ## Lint Python code with pylint
	$(call print_blue,Linting Python code with pylint...)
	@uv run pylint tooling/*.py
	$(call print_green,Pylint checks passed)

lint-python-fix: ## Auto-fix Python linting issues with ruff
	$(call print_blue,Auto-fixing Python linting issues...)
	@uv run ruff check --fix tooling/
	$(call print_green,Python auto-fix complete)

lint-go: ## Lint Go code with golangci-lint (comprehensive)
	$(call print_blue,Linting Go code with golangci-lint...)
	@golangci-lint run ./...
	$(call print_green,Go code linted with golangci-lint)

lint-go-fast: ## Lint Go code with golangci-lint (fast - critical linters only)
	$(call print_blue,Running critical linters only...)
	@golangci-lint run --enable-only=errcheck,gosimple,govet,ineffassign,staticcheck,typecheck,unused ./...
	$(call print_green,Critical linters passed)

lint-parallel: lint-rust lint-python ## Lint all code in parallel categories (fastest!)
	$(call print_blue,Linting in parallel...)
	@uv run tooling/parallel_lint.py
	$(call print_green,Parallel linting complete)

lint-parallel-critical: lint-rust lint-python ## Lint critical categories only in parallel
	$(call print_blue,Running critical linters in parallel...)
	@uv run tooling/parallel_lint.py --categories critical,security
	$(call print_green,Critical parallel linting complete)

lint-parallel-list: ## List all available linter categories
	@uv run tooling/parallel_lint.py --list

lint-workflows: ## Lint GitHub Actions workflows with actionlint
	$(call print_blue,Linting GitHub Actions workflows...)
	@command -v actionlint >/dev/null 2>&1 || { echo "⚠️  actionlint not installed. Install with: brew install actionlint"; exit 1; }
	@actionlint .github/workflows/*.yml
	$(call print_green,Workflow linting complete)

lint-fix: ## Auto-fix linting issues where possible
	$(call print_blue,Auto-fixing linting issues...)
	@golangci-lint run --fix ./...
	@cd prism-proxy && cargo fmt
	@cd prism-proxy && cargo clippy --fix --allow-dirty -- -D warnings
	@uv run ruff check --fix tooling/
	@uv run ruff format tooling/
	$(call print_green,Auto-fix complete)

##@ Podman & Compose

# Compose command - uses Podman instead of Docker Desktop (ADR-049)
COMPOSE := podman compose

podman-start: ## Start Podman machine if not running
	@if ! podman machine inspect --format '{{.State}}' 2>/dev/null | grep -q running; then \
		printf "$(BLUE)Starting Podman machine...$(NC)\n"; \
		podman machine start; \
		printf "$(GREEN)✓ Podman machine started$(NC)\n"; \
	else \
		printf "$(GREEN)✓ Podman machine already running$(NC)\n"; \
	fi

podman-stop: ## Stop Podman machine
	$(call print_blue,Stopping Podman machine...)
	@podman machine stop
	$(call print_green,Podman machine stopped)

podman-status: ## Show Podman machine status
	@podman machine list

##@ Local Compose Deployments

up-dev: podman-start ## Start dev services (Redis, NATS) from docker-compose.yml
	$(call print_blue,Starting dev services...)
	@$(COMPOSE) up -d
	$(call print_green,Dev services started - Redis: localhost:6379, NATS: localhost:4222)

down-dev: ## Stop dev services
	$(call print_blue,Stopping dev services...)
	@$(COMPOSE) down
	$(call print_green,Dev services stopped)

logs-dev: ## Show logs from dev services
	@$(COMPOSE) logs -f

up-test: podman-start ## Start test infrastructure (Postgres, Kafka, NATS, LocalStack)
	$(call print_blue,Starting test infrastructure...)
	@$(COMPOSE) -f docker-compose.test.yml up -d
	$(call print_green,Test infrastructure started - Postgres: localhost:5432, Kafka: localhost:9092, NATS: localhost:4222, LocalStack: localhost:4566)

down-test: ## Stop test infrastructure
	$(call print_blue,Stopping test infrastructure...)
	@$(COMPOSE) -f docker-compose.test.yml down
	$(call print_green,Test infrastructure stopped)

logs-test: ## Show logs from test infrastructure
	@$(COMPOSE) -f docker-compose.test.yml logs -f

up-dex: podman-start ## Start Dex IdP for local auth
	$(call print_blue,Starting Dex IdP...)
	@$(COMPOSE) -f local-dev/docker-compose.dex.yml up -d
	$(call print_green,Dex IdP started - OIDC: http://localhost:5556)

down-dex: ## Stop Dex IdP
	$(call print_blue,Stopping Dex IdP...)
	@$(COMPOSE) -f local-dev/docker-compose.dex.yml down
	$(call print_green,Dex IdP stopped)

logs-dex: ## Show logs from Dex IdP
	@$(COMPOSE) -f local-dev/docker-compose.dex.yml logs -f

up-poc4: podman-start ## Start POC4 multicast registry deployment
	$(call print_blue,Starting POC4 deployment...)
	@$(COMPOSE) -f deployments/poc4-multicast-registry/docker-compose.yml up -d
	$(call print_green,POC4 deployment started - Redis: localhost:6379, NATS: localhost:4222)

down-poc4: ## Stop POC4 multicast registry deployment
	$(call print_blue,Stopping POC4 deployment...)
	@$(COMPOSE) -f deployments/poc4-multicast-registry/docker-compose.yml down
	$(call print_green,POC4 deployment stopped)

logs-poc4: ## Show logs from POC4 deployment
	@$(COMPOSE) -f deployments/poc4-multicast-registry/docker-compose.yml logs -f

up-all: up-dev up-dex ## Start all local services (dev + dex)
	$(call print_green,All local services started)

down-all: down-poc4 down-dex down-test down-dev ## Stop all compose deployments
	$(call print_green,All compose deployments stopped)

# Legacy aliases (deprecated - use up-dev/down-dev)
docker-up: up-dev ## [DEPRECATED] Use up-dev instead
docker-down: down-dev ## [DEPRECATED] Use down-dev instead
docker-logs: logs-dev ## [DEPRECATED] Use logs-dev instead

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
