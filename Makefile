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

build: build-proxy build-prismctl build-prism-admin build-pattern-launcher build-plugin-watcher build-patterns ## Build all components (excludes prism-loadtest due to module path issues)

build-proxy: ## Build Rust proxy
	$(call print_blue,Building Rust proxy...)
	@mkdir -p $(BINARIES_DIR)
	@cd prism-proxy && CARGO_TARGET_DIR=$(RUST_TARGET_DIR) cargo build --release
	@cp $(RUST_TARGET_DIR)/release/prism-proxy $(BINARIES_DIR)/prism-proxy
	$(call print_green,Proxy built: $(BINARIES_DIR)/prism-proxy)

build-prismctl: ## Build prismctl CLI
	$(call print_blue,Building prismctl CLI...)
	@mkdir -p $(BINARIES_DIR)
	@cd cmd/prismctl && go build -o $(BINARIES_DIR)/prismctl .
	$(call print_green,prismctl built: $(BINARIES_DIR)/prismctl)

build-prism-admin: ## Build prism-admin CLI
	$(call print_blue,Building prism-admin CLI...)
	@mkdir -p $(BINARIES_DIR)
	@cd cmd/prism-admin && go build -o $(BINARIES_DIR)/prism-admin .
	$(call print_green,prism-admin built: $(BINARIES_DIR)/prism-admin)

build-prism-loadtest: ## Build prism-loadtest CLI
	$(call print_blue,Building prism-loadtest CLI...)
	@mkdir -p $(BINARIES_DIR)
	@cd cmd/prism-loadtest && go build -o $(BINARIES_DIR)/prism-loadtest .
	$(call print_green,prism-loadtest built: $(BINARIES_DIR)/prism-loadtest)

build-pattern-launcher: ## Build pattern-launcher utility
	$(call print_blue,Building pattern-launcher...)
	@mkdir -p $(BINARIES_DIR)
	@cd cmd/pattern-launcher && go build -o $(BINARIES_DIR)/pattern-launcher .
	$(call print_green,pattern-launcher built: $(BINARIES_DIR)/pattern-launcher)

build-plugin-watcher: ## Build plugin-watcher utility
	$(call print_blue,Building plugin-watcher...)
	@mkdir -p $(BINARIES_DIR)
	@cd cmd/plugin-watcher && go build -o $(BINARIES_DIR)/plugin-watcher .
	$(call print_green,plugin-watcher built: $(BINARIES_DIR)/plugin-watcher)

build-consumer-runner: ## Build consumer pattern runner
	$(call print_blue,Building consumer-runner...)
	@mkdir -p $(BINARIES_DIR)
	@cd patterns/consumer/cmd/consumer-runner && go build -o $(BINARIES_DIR)/consumer-runner .
	$(call print_green,consumer-runner built: $(BINARIES_DIR)/consumer-runner)

build-producer-runner: ## Build producer pattern runner
	$(call print_blue,Building producer-runner...)
	@mkdir -p $(BINARIES_DIR)
	@cd patterns/producer/cmd/producer-runner && go build -o $(BINARIES_DIR)/producer-runner .
	$(call print_green,producer-runner built: $(BINARIES_DIR)/producer-runner)

build-multicast-registry-runner: ## Build multicast registry pattern runner
	$(call print_blue,Building multicast-registry-runner...)
	@mkdir -p $(BINARIES_DIR)
	@cd patterns/multicast_registry/cmd/multicast-registry-runner && go build -o $(BINARIES_DIR)/multicast-registry-runner .
	$(call print_green,multicast-registry-runner built: $(BINARIES_DIR)/multicast-registry-runner)

build-keyvalue-runner: ## Build keyvalue pattern runner
	$(call print_blue,Building keyvalue-runner...)
	@mkdir -p $(BINARIES_DIR)
	@cd patterns/keyvalue/cmd/keyvalue-runner && go build -o $(BINARIES_DIR)/keyvalue-runner .
	$(call print_green,keyvalue-runner built: $(BINARIES_DIR)/keyvalue-runner)

build-patterns: build-consumer-runner build-producer-runner build-multicast-registry-runner build-keyvalue-runner ## Build all pattern runners

build-dev: ## Build all components in debug mode (faster)
	$(call print_blue,Building in debug mode...)
	@mkdir -p $(BINARIES_DIR)
	@cd prism-proxy && CARGO_TARGET_DIR=$(RUST_TARGET_DIR) cargo build
	@cp $(RUST_TARGET_DIR)/debug/prism-proxy $(BINARIES_DIR)/prism-proxy-debug
	@cd cmd/prismctl && go build -o $(BINARIES_DIR)/prismctl-debug .
	@cd cmd/prism-admin && go build -o $(BINARIES_DIR)/prism-admin-debug .
	@cd cmd/prism-loadtest && go build -o $(BINARIES_DIR)/prism-loadtest-debug .
	@cd cmd/pattern-launcher && go build -o $(BINARIES_DIR)/pattern-launcher-debug .
	@cd cmd/plugin-watcher && go build -o $(BINARIES_DIR)/plugin-watcher-debug .
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

test-patterns: test-memstore test-redis test-nats test-kafka test-postgres test-core test-prismctl ## Run all Go pattern tests

test-memstore: ## Run MemStore tests
	$(call print_blue,Running MemStore tests...)
	@cd pkg/drivers/memstore && go test -v -cover ./...
	$(call print_green,MemStore tests passed)

test-redis: ## Run Redis tests
	$(call print_blue,Running Redis tests...)
	@cd pkg/drivers/redis && go test -v -cover ./...
	$(call print_green,Redis tests passed)

test-nats: ## Run NATS tests
	$(call print_blue,Running NATS tests...)
	@cd pkg/drivers/nats && go test -v -cover ./...
	$(call print_green,NATS tests passed)

test-kafka: ## Run Kafka tests
	$(call print_blue,Running Kafka tests...)
	@cd pkg/drivers/kafka && go test -v -cover ./...
	$(call print_green,Kafka tests passed)

test-postgres: ## Run PostgreSQL tests
	$(call print_blue,Running PostgreSQL tests...)
	@cd pkg/drivers/postgres && go test -v -cover ./...
	$(call print_green,PostgreSQL tests passed)

test-core: ## Run Core SDK tests
	$(call print_blue,Running Core SDK tests...)
	@cd pkg/plugin && go test -v -cover ./...
	$(call print_green,Core SDK tests passed)

test-prismctl: ## Run prismctl tests
	$(call print_blue,Running prismctl tests...)
	@cd cmd/prismctl && go test -v -cover ./...
	$(call print_green,prismctl tests passed)

test-integration: build-memstore ## Run integration tests (requires built MemStore binary)
	$(call print_blue,Running integration tests...)
	@cd prism-proxy && cargo test --test integration_test -- --ignored --nocapture
	$(call print_green,Integration tests passed)

test-all: test test-integration test-integration-go test-acceptance ## Run all tests (unit, integration, acceptance)
	$(call print_green,All tests (unit + integration + acceptance) passed)

test-acceptance: test-acceptance-interfaces test-acceptance-redis test-acceptance-nats test-acceptance-kafka test-acceptance-postgres ## Run all acceptance tests with testcontainers
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

test-acceptance-kafka: ## Run Kafka acceptance tests with testcontainers
	$(call print_blue,Running Kafka acceptance tests...)
	@cd tests/acceptance && go test -v -timeout 10m ./kafka/...
	$(call print_green,Kafka acceptance tests passed)

test-acceptance-postgres: ## Run PostgreSQL acceptance tests with testcontainers
	$(call print_blue,Running PostgreSQL acceptance tests...)
	@cd tests/acceptance && go test -v -timeout 10m ./postgres/...
	$(call print_green,PostgreSQL acceptance tests passed)

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

test-acceptance-unified: ## Run unified acceptance tests (dynamic interface discovery)
	$(call print_blue,Running unified acceptance tests...)
	@cd tests/acceptance && go test -v -timeout 10m -run TestUnifiedPattern
	$(call print_green,Unified acceptance tests passed)

test-acceptance-discover: ## Discover interfaces supported by a pattern at PATTERN_ADDR
	$(call print_blue,Discovering pattern interfaces at $(PATTERN_ADDR)...)
	@cd tests/acceptance && go test -v -run TestDiscoverInterfaces
	$(call print_green,Interface discovery complete)

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

coverage-patterns: coverage-memstore coverage-redis coverage-nats coverage-kafka coverage-postgres coverage-core ## Generate coverage for all patterns

coverage-memstore: ## Generate MemStore coverage report
	$(call print_blue,Generating MemStore coverage...)
	@mkdir -p $(COVERAGE_DIR)/memstore
	@cd pkg/drivers/memstore && go test -coverprofile=../../build/coverage/memstore/coverage.out ./...
	@cd pkg/drivers/memstore && go tool cover -func=../../build/coverage/memstore/coverage.out | grep total
	@cd pkg/drivers/memstore && go tool cover -html=../../build/coverage/memstore/coverage.out -o ../../build/coverage/memstore/coverage.html
	$(call print_green,MemStore coverage: $(COVERAGE_DIR)/memstore/coverage.html)

coverage-redis: ## Generate Redis coverage report
	$(call print_blue,Generating Redis coverage...)
	@mkdir -p $(COVERAGE_DIR)/redis
	@cd pkg/drivers/redis && go test -coverprofile=../../build/coverage/redis/coverage.out ./...
	@cd pkg/drivers/redis && go tool cover -func=../../build/coverage/redis/coverage.out | grep total
	@cd pkg/drivers/redis && go tool cover -html=../../build/coverage/redis/coverage.out -o ../../build/coverage/redis/coverage.html
	$(call print_green,Redis coverage: $(COVERAGE_DIR)/redis/coverage.html)

coverage-nats: ## Generate NATS coverage report
	$(call print_blue,Generating NATS coverage...)
	@mkdir -p $(COVERAGE_DIR)/nats
	@cd pkg/drivers/nats && go test -coverprofile=../../build/coverage/nats/coverage.out ./...
	@cd pkg/drivers/nats && go tool cover -func=../../build/coverage/nats/coverage.out | grep total
	@cd pkg/drivers/nats && go tool cover -html=../../build/coverage/nats/coverage.out -o ../../build/coverage/nats/coverage.html
	$(call print_green,NATS coverage: $(COVERAGE_DIR)/nats/coverage.html)

coverage-kafka: ## Generate Kafka coverage report
	$(call print_blue,Generating Kafka coverage...)
	@mkdir -p $(COVERAGE_DIR)/kafka
	@cd pkg/drivers/kafka && go test -coverprofile=../../build/coverage/kafka/coverage.out ./...
	@cd pkg/drivers/kafka && go tool cover -func=../../build/coverage/kafka/coverage.out | grep total
	@cd pkg/drivers/kafka && go tool cover -html=../../build/coverage/kafka/coverage.out -o ../../build/coverage/kafka/coverage.html
	$(call print_green,Kafka coverage: $(COVERAGE_DIR)/kafka/coverage.html)

coverage-postgres: ## Generate PostgreSQL coverage report
	$(call print_blue,Generating PostgreSQL coverage...)
	@mkdir -p $(COVERAGE_DIR)/postgres
	@cd pkg/drivers/postgres && go test -coverprofile=../../build/coverage/postgres/coverage.out ./...
	@cd pkg/drivers/postgres && go tool cover -func=../../build/coverage/postgres/coverage.out | grep total
	@cd pkg/drivers/postgres && go tool cover -html=../../build/coverage/postgres/coverage.out -o ../../build/coverage/postgres/coverage.html
	$(call print_green,PostgreSQL coverage: $(COVERAGE_DIR)/postgres/coverage.html)

coverage-core: ## Generate Core SDK coverage report
	$(call print_blue,Generating Core SDK coverage...)
	@mkdir -p $(COVERAGE_DIR)/core
	@cd pkg/plugin && go test -coverprofile=../../build/coverage/core/coverage.out ./...
	@cd pkg/plugin && go tool cover -func=../../build/coverage/core/coverage.out | grep total
	@cd pkg/plugin && go tool cover -html=../../build/coverage/core/coverage.out -o ../../build/coverage/core/coverage.html
	$(call print_green,Core SDK coverage: $(COVERAGE_DIR)/core/coverage.html)

coverage-acceptance: coverage-acceptance-interfaces coverage-acceptance-redis coverage-acceptance-nats coverage-acceptance-kafka coverage-acceptance-postgres ## Generate coverage for acceptance tests

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

coverage-acceptance-kafka: ## Generate Kafka acceptance test coverage
	$(call print_blue,Generating Kafka acceptance test coverage...)
	@mkdir -p $(COVERAGE_DIR)/acceptance/kafka
	@cd tests/acceptance && go test -coverprofile=../../build/coverage/acceptance/kafka/coverage.out -timeout 10m ./kafka/...
	@cd tests/acceptance && go tool cover -func=../../build/coverage/acceptance/kafka/coverage.out | grep total
	@cd tests/acceptance && go tool cover -html=../../build/coverage/acceptance/kafka/coverage.out -o ../../build/coverage/acceptance/kafka/coverage.html
	$(call print_green,Kafka acceptance coverage: $(COVERAGE_DIR)/acceptance/kafka/coverage.html)

coverage-acceptance-postgres: ## Generate PostgreSQL acceptance test coverage
	$(call print_blue,Generating PostgreSQL acceptance test coverage...)
	@mkdir -p $(COVERAGE_DIR)/acceptance/postgres
	@cd tests/acceptance && go test -coverprofile=../../build/coverage/acceptance/postgres/coverage.out -timeout 10m ./postgres/...
	@cd tests/acceptance && go tool cover -func=../../build/coverage/acceptance/postgres/coverage.out | grep total
	@cd tests/acceptance && go tool cover -html=../../build/coverage/acceptance/postgres/coverage.out -o ../../build/coverage/acceptance/postgres/coverage.html
	$(call print_green,PostgreSQL acceptance coverage: $(COVERAGE_DIR)/acceptance/postgres/coverage.html)

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
	@mkdir -p pkg/plugin/gen
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
	@rm -f pkg/drivers/memstore/memstore pkg/drivers/memstore/coverage.out pkg/drivers/memstore/coverage.html
	@rm -f pkg/drivers/redis/redis pkg/drivers/redis/coverage.out pkg/drivers/redis/coverage.html
	@rm -f pkg/drivers/nats/nats pkg/drivers/nats/coverage.out pkg/drivers/nats/coverage.html
	@rm -f pkg/drivers/kafka/kafka pkg/drivers/kafka/coverage.out pkg/drivers/kafka/coverage.html
	@rm -f pkg/drivers/postgres/postgres pkg/drivers/postgres/coverage.out pkg/drivers/postgres/coverage.html
	@rm -f pkg/plugin/coverage.out pkg/plugin/coverage.html
	@rm -f tests/acceptance/interfaces/coverage.out tests/acceptance/interfaces/coverage.html
	@rm -f tests/acceptance/redis/coverage.out tests/acceptance/redis/coverage.html
	@rm -f tests/acceptance/nats/coverage.out tests/acceptance/nats/coverage.html
	@rm -f tests/integration/coverage.out tests/integration/coverage.html
	@rm -rf test-logs/ prism-proxy/target/
	$(call print_green,Legacy artifacts cleaned)

clean-proto: ## Clean generated protobuf code
	$(call print_blue,Cleaning generated protobuf code...)
	@rm -rf pkg/plugin/gen
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
	@cd pkg/drivers/memstore && go fmt ./...
	@cd pkg/drivers/redis && go fmt ./...
	@cd pkg/drivers/nats && go fmt ./...
	@cd pkg/drivers/kafka && go fmt ./...
	@cd pkg/drivers/postgres && go fmt ./...
	@cd pkg/plugin && go fmt ./...
	@cd cmd/prismctl && go fmt ./...
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

##@ Quick Development Iteration

quick-check: lint-parallel-critical build test-unit-fast ## Quick pre-commit check (critical lint + build + fast unit tests) ~30s
	$(call print_green,Quick check complete - ready to commit!)

test-unit-fast: ## Run only unit tests (no acceptance, no testcontainers) ~10s
	$(call print_blue,Running fast unit tests...)
	@uv run tooling/parallel_test.py --categories unit,lint --log-dir $(TEST_LOGS_DIR)
	$(call print_green,Fast unit tests complete)

quick-pattern: ## Quick test for current pattern directory (auto-detects pattern)
	$(call print_blue,Quick pattern test...)
	@if [ -f "go.mod" ]; then \
		go test -v -short -timeout 30s ./...; \
	elif [ -f "../../go.mod" ] && [ -d "cmd" ]; then \
		cd cmd && go build && cd .. && go test -v -short -timeout 30s ./...; \
	else \
		echo "Not in a pattern directory"; \
		exit 1; \
	fi
	$(call print_green,Pattern quick test complete)

quick-run-pattern: ## Quick build and run current pattern runner
	$(call print_blue,Building and running pattern...)
	@if [ -d "cmd" ]; then \
		RUNNER_NAME=$$(basename $(CURDIR))-runner; \
		echo "Building $$RUNNER_NAME..."; \
		cd cmd/$$RUNNER_NAME && go build -o $(BINARIES_DIR)/$$RUNNER_NAME . && \
		echo "✓ Built: $(BINARIES_DIR)/$$RUNNER_NAME" && \
		echo "Run with: ./$(BINARIES_DIR)/$$RUNNER_NAME --help"; \
	else \
		echo "Not in a pattern directory with cmd/"; \
		exit 1; \
	fi

test-this: ## Test current directory (auto-detects Go, Rust, or Python)
	$(call print_blue,Testing current directory...)
	@if [ -f "Cargo.toml" ]; then \
		cargo test; \
	elif [ -f "go.mod" ]; then \
		go test -v -timeout 30s ./...; \
	elif [ -f "pyproject.toml" ]; then \
		uv run pytest; \
	else \
		echo "No recognized test framework in current directory"; \
		exit 1; \
	fi

build-this: ## Build current directory (auto-detects Go, Rust)
	$(call print_blue,Building current directory...)
	@if [ -f "Cargo.toml" ]; then \
		cargo build; \
	elif [ -f "go.mod" ]; then \
		go build ./...; \
	elif [ -d "cmd" ] && [ -f "../go.mod" ]; then \
		for d in cmd/*; do \
			if [ -d "$$d" ]; then \
				echo "Building $$d..."; \
				cd $$d && go build && cd ../..; \
			fi \
		done; \
	else \
		echo "No recognized build system in current directory"; \
		exit 1; \
	fi

quick-verify-pattern: quick-pattern quick-run-pattern ## Verify pattern (test + build + show how to run)
	$(call print_green,Pattern verified and ready!)

