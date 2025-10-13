.PHONY: all clean test lint format typecheck install

# Colors
GREEN  := \033[0;32m
YELLOW := \033[0;33m
NC     := \033[0m

all: format lint typecheck test

install: ## Install development dependencies
	@echo "$(GREEN)Installing development dependencies...$(NC)"
	@uv sync --extra dev

format: ## Format code with ruff
	@echo "$(GREEN)Formatting code...$(NC)"
	@uv run ruff format prismctl/ tests/
	@echo "$(GREEN)✓ Code formatted$(NC)"

lint: ## Lint code with ruff
	@echo "$(GREEN)Linting code...$(NC)"
	@uv run ruff check prismctl/ tests/
	@echo "$(GREEN)✓ Linting passed$(NC)"

lint-fix: ## Auto-fix linting issues
	@echo "$(GREEN)Auto-fixing linting issues...$(NC)"
	@uv run ruff check --fix prismctl/ tests/
	@uv run ruff format prismctl/ tests/
	@echo "$(GREEN)✓ Auto-fix complete$(NC)"

typecheck: ## Run mypy type checker
	@echo "$(GREEN)Running type checker...$(NC)"
	@uv run mypy prismctl/
	@echo "$(GREEN)✓ Type checking passed$(NC)"

test: ## Run tests with coverage
	@echo "$(GREEN)Running tests...$(NC)"
	@uv run pytest
	@echo "$(GREEN)✓ Tests passed$(NC)"

test-verbose: ## Run tests with verbose output
	@echo "$(GREEN)Running tests (verbose)...$(NC)"
	@uv run pytest -vv

clean: ## Clean build artifacts
	@echo "$(GREEN)Cleaning build artifacts...$(NC)"
	@rm -rf build/
	@rm -rf dist/
	@rm -rf *.egg-info
	@rm -rf .pytest_cache
	@rm -rf .mypy_cache
	@rm -rf .ruff_cache
	@rm -rf htmlcov/
	@rm -rf .coverage
	@find . -type d -name __pycache__ -exec rm -rf {} +
	@find . -type f -name '*.pyc' -delete
	@echo "$(GREEN)✓ Cleaned$(NC)"

build: ## Build package
	@echo "$(GREEN)Building package...$(NC)"
	@uv build
	@echo "$(GREEN)✓ Package built$(NC)"

install-local: ## Install package locally in editable mode
	@echo "$(GREEN)Installing package locally...$(NC)"
	@uv pip install -e .
	@echo "$(GREEN)✓ Package installed$(NC)"

help: ## Display this help message
	@awk 'BEGIN {FS = ":.*##"; printf "\\nUsage:\\n  make \\033[36m<target>\\033[0m\\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \\033[36m%-15s\\033[0m %s\\n", $$1, $$2 } /^##@/ { printf "\\n\\033[1m%s\\033[0m\\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
