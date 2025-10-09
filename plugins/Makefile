# Prism Backend Plugins Makefile
# Based on ADR-025 (Container Plugin Model) and ADR-026 (Distroless Images)
#
# Build all plugins with Podman for minimal, secure containers

.PHONY: all build clean test help
.PHONY: postgres kafka redis
.PHONY: postgres-debug kafka-debug redis-debug
.PHONY: push push-debug run-postgres run-kafka run-redis

# Configuration
REGISTRY ?= prism
VERSION ?= latest
PODMAN := podman

# Plugin list
PLUGINS := postgres kafka redis

# Default target
all: build

# Help target
help:
	@echo "Prism Backend Plugins Build System"
	@echo "==================================="
	@echo ""
	@echo "Targets:"
	@echo "  all                Build all plugins (production images)"
	@echo "  build              Build all production images"
	@echo "  build-debug        Build all debug images"
	@echo "  clean              Remove all images"
	@echo "  test               Run tests for all plugins"
	@echo ""
	@echo "Individual plugins:"
	@echo "  postgres           Build postgres plugin (production)"
	@echo "  postgres-debug     Build postgres plugin (debug)"
	@echo "  kafka              Build kafka plugin (production)"
	@echo "  kafka-debug        Build kafka plugin (debug)"
	@echo "  redis              Build redis plugin (production)"
	@echo "  redis-debug        Build redis plugin (debug)"
	@echo ""
	@echo "Operations:"
	@echo "  push               Push all production images to registry"
	@echo "  push-debug         Push all debug images to registry"
	@echo "  run-postgres       Run postgres plugin locally"
	@echo "  run-kafka          Run kafka plugin locally"
	@echo "  run-redis          Run redis plugin locally"
	@echo ""
	@echo "Variables:"
	@echo "  REGISTRY=$(REGISTRY)    Container registry"
	@echo "  VERSION=$(VERSION)      Image version tag"
	@echo ""
	@echo "Examples:"
	@echo "  make build                    # Build all production images"
	@echo "  make postgres postgres-debug  # Build both variants of postgres"
	@echo "  make VERSION=v1.0.0 build     # Build with specific version"

# Build all production images
build: $(PLUGINS)
	@echo "✓ All plugins built successfully"

# Build all debug images
build-debug: $(addsuffix -debug,$(PLUGINS))
	@echo "✓ All debug plugins built successfully"

# Build core package (required by all plugins)
.build-core-timestamp:
	@echo "Building core package..."
	@cd core && go mod download
	@touch .build-core-timestamp

# PostgreSQL Plugin
postgres: .build-core-timestamp
	@echo "Building postgres plugin (production)..."
	@cd postgres && $(PODMAN) build \
		--target production \
		-t $(REGISTRY)/postgres-plugin:$(VERSION) \
		-t $(REGISTRY)/postgres-plugin:latest \
		-f Dockerfile \
		..
	@echo "✓ postgres plugin built: $(REGISTRY)/postgres-plugin:$(VERSION)"

postgres-debug: .build-core-timestamp
	@echo "Building postgres plugin (debug)..."
	@cd postgres && $(PODMAN) build \
		--target debug \
		-t $(REGISTRY)/postgres-plugin:$(VERSION)-debug \
		-t $(REGISTRY)/postgres-plugin:debug \
		-f Dockerfile \
		..
	@echo "✓ postgres debug plugin built: $(REGISTRY)/postgres-plugin:$(VERSION)-debug"

# Kafka Plugin
kafka: .build-core-timestamp
	@echo "Building kafka plugin (production)..."
	@cd kafka && $(PODMAN) build \
		--target production \
		-t $(REGISTRY)/kafka-plugin:$(VERSION) \
		-t $(REGISTRY)/kafka-plugin:latest \
		-f Dockerfile \
		..
	@echo "✓ kafka plugin built: $(REGISTRY)/kafka-plugin:$(VERSION)"

kafka-debug: .build-core-timestamp
	@echo "Building kafka plugin (debug)..."
	@cd kafka && $(PODMAN) build \
		--target debug \
		-t $(REGISTRY)/kafka-plugin:$(VERSION)-debug \
		-t $(REGISTRY)/kafka-plugin:debug \
		-f Dockerfile \
		..
	@echo "✓ kafka debug plugin built: $(REGISTRY)/kafka-plugin:$(VERSION)-debug"

# Redis Plugin
redis: .build-core-timestamp
	@echo "Building redis plugin (production)..."
	@cd redis && $(PODMAN) build \
		--target production \
		-t $(REGISTRY)/redis-plugin:$(VERSION) \
		-t $(REGISTRY)/redis-plugin:latest \
		-f Dockerfile \
		..
	@echo "✓ redis plugin built: $(REGISTRY)/redis-plugin:$(VERSION)"

redis-debug: .build-core-timestamp
	@echo "Building redis plugin (debug)..."
	@cd redis && $(PODMAN) build \
		--target debug \
		-t $(REGISTRY)/redis-plugin:$(VERSION)-debug \
		-t $(REGISTRY)/redis-plugin:debug \
		-f Dockerfile \
		..
	@echo "✓ redis debug plugin built: $(REGISTRY)/redis-plugin:$(VERSION)-debug"

# Push images to registry
push:
	@echo "Pushing production images to $(REGISTRY)..."
	@$(PODMAN) push $(REGISTRY)/postgres-plugin:$(VERSION)
	@$(PODMAN) push $(REGISTRY)/postgres-plugin:latest
	@$(PODMAN) push $(REGISTRY)/kafka-plugin:$(VERSION)
	@$(PODMAN) push $(REGISTRY)/kafka-plugin:latest
	@$(PODMAN) push $(REGISTRY)/redis-plugin:$(VERSION)
	@$(PODMAN) push $(REGISTRY)/redis-plugin:latest
	@echo "✓ All production images pushed"

push-debug:
	@echo "Pushing debug images to $(REGISTRY)..."
	@$(PODMAN) push $(REGISTRY)/postgres-plugin:$(VERSION)-debug
	@$(PODMAN) push $(REGISTRY)/postgres-plugin:debug
	@$(PODMAN) push $(REGISTRY)/kafka-plugin:$(VERSION)-debug
	@$(PODMAN) push $(REGISTRY)/kafka-plugin:debug
	@$(PODMAN) push $(REGISTRY)/redis-plugin:$(VERSION)-debug
	@$(PODMAN) push $(REGISTRY)/redis-plugin:debug
	@echo "✓ All debug images pushed"

# Run plugins locally (for testing)
run-postgres: postgres
	@echo "Running postgres plugin..."
	@$(PODMAN) run --rm -it \
		--name postgres-plugin \
		-e DATABASE_URL="postgres://prism:password@host.containers.internal:5432/prism" \
		-e PRISM_LOG_LEVEL=debug \
		-p 9090:9090 \
		$(REGISTRY)/postgres-plugin:latest

run-kafka: kafka
	@echo "Running kafka plugin..."
	@$(PODMAN) run --rm -it \
		--name kafka-plugin \
		-e KAFKA_BROKERS="host.containers.internal:9092" \
		-e KAFKA_TOPIC="events" \
		-e PRISM_LOG_LEVEL=debug \
		-p 9091:9091 \
		$(REGISTRY)/kafka-plugin:latest

run-redis: redis
	@echo "Running redis plugin..."
	@$(PODMAN) run --rm -it \
		--name redis-plugin \
		-e REDIS_ADDRESS="host.containers.internal:6379" \
		-e PRISM_LOG_LEVEL=debug \
		-p 9092:9092 \
		$(REGISTRY)/redis-plugin:latest

# Test plugins
test:
	@echo "Running tests..."
	@cd core && go test -v ./...
	@cd postgres && go test -v ./...
	@cd kafka && go test -v ./...
	@cd redis && go test -v ./...
	@echo "✓ All tests passed"

# Clean all images
clean:
	@echo "Cleaning images..."
	-@$(PODMAN) rmi $(REGISTRY)/postgres-plugin:$(VERSION) 2>/dev/null || true
	-@$(PODMAN) rmi $(REGISTRY)/postgres-plugin:latest 2>/dev/null || true
	-@$(PODMAN) rmi $(REGISTRY)/postgres-plugin:$(VERSION)-debug 2>/dev/null || true
	-@$(PODMAN) rmi $(REGISTRY)/postgres-plugin:debug 2>/dev/null || true
	-@$(PODMAN) rmi $(REGISTRY)/kafka-plugin:$(VERSION) 2>/dev/null || true
	-@$(PODMAN) rmi $(REGISTRY)/kafka-plugin:latest 2>/dev/null || true
	-@$(PODMAN) rmi $(REGISTRY)/kafka-plugin:$(VERSION)-debug 2>/dev/null || true
	-@$(PODMAN) rmi $(REGISTRY)/kafka-plugin:debug 2>/dev/null || true
	-@$(PODMAN) rmi $(REGISTRY)/redis-plugin:$(VERSION) 2>/dev/null || true
	-@$(PODMAN) rmi $(REGISTRY)/redis-plugin:latest 2>/dev/null || true
	-@$(PODMAN) rmi $(REGISTRY)/redis-plugin:$(VERSION)-debug 2>/dev/null || true
	-@$(PODMAN) rmi $(REGISTRY)/redis-plugin:debug 2>/dev/null || true
	@rm -f .build-core-timestamp
	@echo "✓ All images cleaned"

# List all built images
list:
	@echo "Built plugin images:"
	@$(PODMAN) images | grep "$(REGISTRY).*plugin" || echo "No plugin images found"

# Show image sizes
sizes:
	@echo "Image sizes:"
	@$(PODMAN) images --format "table {{.Repository}}:{{.Tag}}\t{{.Size}}" | grep "$(REGISTRY).*plugin" || echo "No plugin images found"
