.PHONY: test test-docker test-podman clean-containers help

# Default target
help:
	@echo "Environment Validation Test Suite"
	@echo ""
	@echo "Targets:"
	@echo "  make test          - Run tests with current DOCKER_HOST (or Docker Desktop if unset)"
	@echo "  make test-docker   - Run tests using Docker Desktop"
	@echo "  make test-podman   - Run tests using Podman"
	@echo "  make clean-containers - Stop and remove all containers"
	@echo ""
	@echo "Current Environment:"
	@if [ -n "$$DOCKER_HOST" ]; then \
		echo "  DOCKER_HOST: $$DOCKER_HOST"; \
	else \
		echo "  DOCKER_HOST: (not set, will use Docker Desktop)"; \
	fi
	@echo ""

# Run tests with current environment
test:
	go test -v -timeout 60s

# Run tests using Docker Desktop (unset DOCKER_HOST)
test-docker:
	@echo "🐳 Testing with Docker Desktop..."
	@DOCKER_HOST="" go test -v -timeout 60s

# Run tests using Podman
test-podman:
	@echo "🦭 Testing with Podman..."
	@echo "Setting DOCKER_HOST to podman socket..."
	@DOCKER_HOST="$$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}' 2>/dev/null | sed 's#^#unix://#')" go test -v -timeout 60s || \
		(echo "❌ Podman tests failed. You may need to use Docker Desktop instead:" && \
		 echo "   make test-docker" && \
		 exit 1)

# Clean all containers
clean-containers:
	@echo "🧹 Cleaning containers..."
	@if command -v podman >/dev/null 2>&1; then \
		podman stop $$(podman ps -a -q) 2>/dev/null || true; \
		podman rm $$(podman ps -a -q) 2>/dev/null || true; \
		echo "✅ Podman containers cleaned"; \
	fi
	@if command -v docker >/dev/null 2>&1; then \
		docker stop $$(docker ps -a -q) 2>/dev/null || true; \
		docker rm $$(docker ps -a -q) 2>/dev/null || true; \
		echo "✅ Docker containers cleaned"; \
	fi
