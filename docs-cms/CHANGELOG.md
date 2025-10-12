---
title: "Documentation Change Log"
description: "Recent changes to Prism documentation with quick links"
sidebar_position: 1
---

# Documentation Change Log

Quick access to recently updated documentation. Changes listed in reverse chronological order (newest first).

## Recent Changes

### 2025-10-12

#### Test and Build Fixes (UPDATED)
**Commits**: 39f4992, 57f819d

**Summary**: Fixed critical test failures and lint issues preventing clean builds:

**Test Failure Fix** (39f4992):
- Removed non-existent `ttl_seconds` field from KeyValueBasicInterface test
- Issue: Test code referenced field not in proto definition
- SetRequest only has: key, value, tags (no ttl_seconds in basic interface)
- All tests now pass: Rust proxy (18 tests), Go patterns (all passed), acceptance tests (100+ tests)

**Protobuf Module Structure Fix** (57f819d):
- Fixed proto file organization mismatch between Makefile and Rust code
- Updated Makefile to use correct paths (prism/interfaces/ instead of prism/pattern/)
- Updated proxy/src/proto.rs to include both interfaces and interfaces.keyvalue modules
- Fixed all Rust imports from proto::pattern to proto::interfaces/interfaces::keyvalue
- Changed service names to match proto definitions (LifecycleInterface, KeyValueBasicInterface)
- Removed batch operations not in KeyValueBasicInterface
- Fixed clippy warning (removed useless .into() conversion)
- All lint checks now pass (Rust clippy + Go vet)

**Key Facts**: Root cause was proto file reorganization to prism/interfaces/ structure but Makefile and Rust code still referenced old prism/pattern/ paths. Both issues discovered during `make test` and `make lint` runs. Fixes enable clean CI builds.

**Impact**: Development can proceed with passing tests and lint. Build pipeline unblocked. Foundation for POC 1 implementation is stable.

---

#### POC 1 Infrastructure Analysis Documentation (NEW)
**Commit**: 48ee562
**Link**: [MEMO-013](/memos/memo-013)

**Summary**: Comprehensive analysis of Pattern SDK shared complexity and load testing framework evaluation:

**Documents Created**:
- **MEMO-014** (Pattern SDK): Pattern SDK Shared Complexity Analysis
- **RFC-029** (Load Testing): Load Testing Framework Evaluation and Strategy
- **MEMO-013**: POC 1 Infrastructure Analysis (synthesis document)

**Note**: Original numbering (MEMO-012, RFC-023) conflicted with existing documents. Renumbered to MEMO-014 and RFC-029 to maintain sequence integrity.

**Key Findings**:
- 38% code reduction potential by extracting shared complexity to Pattern SDK
- Two-tier load testing strategy: custom tool (pattern-level) + ghz (integration-level)
- 12 areas of duplication across POC 1 plugins (MemStore, Redis, Kafka)
- Recommended SDK enhancements: connection pool, TTL manager, health check framework

**Pattern SDK Analysis** (MEMO-014):
- Connection pool manager reduces Redis/Kafka code by ~270 lines
- TTL manager with heap-based expiration scales to 100K+ keys (vs per-key timers)
- Health check framework standardizes status reporting
- Implementation plan: 2-week sprint (5 days SDK + 2 days refactoring)
- Expected: 2100 LOC → 1300 LOC (38% reduction)

**Load Testing Evaluation** (RFC-029):
- Evaluated 5 frameworks: ghz (24/30), k6 (20/30), fortio (22/30), vegeta (disqualified), hey/bombardier (disqualified)
- Recommendation: Keep custom prism-loadtest + add ghz for integration testing
- Two-tier strategy: pattern-level (prism-loadtest) + integration-level (ghz)
- Custom tool validated by MEMO-010 (100 req/sec, precise rate limiting, thread-safe)

**POC 1 Infrastructure Synthesis** (MEMO-013):
- Combines SDK refactoring + load testing enhancements
- Timeline: 2-week sprint alongside POC 1 implementation
- Deliverables: Enhanced SDK packages, two-tier load testing, 38% code reduction
- Success metrics: coverage targets (85%+), performance baselines, reduced plugin LOC

**Key Facts**: Analysis based on RFC-021 POC 1 plugin designs. All three documents validated with `uv run tooling/validate_docs.py` (101 docs, 0 errors). Implementation can proceed in parallel with POC 1.

**Impact**: Provides clear roadmap for Pattern SDK enhancements. Establishes comprehensive load testing strategy. Sets foundation for maintainable, testable plugin implementations.

---

### 2025-10-11

#### MEMO-012: Developer Experience and Common Workflows (NEW)
**Link**: [MEMO-012](/memos/memo-012)

**Summary**: Practical guide documenting actual developer workflows, common commands, and testing patterns:
- **Core Commands**: Documentation validation, pattern builds, proxy runs, load testing
- **Mental Models**: Three-layer testing (unit → integration → load), TDD workflow (red → green → refactor)
- **Speed Optimization**: Skip full validation during iteration, parallel testing, incremental builds, reuse running backends
- **Common Shortcuts**: Bash aliases, Docker Compose profiles, Go test shortcuts
- **Integration Test Setup**: Multicast Registry example with Redis + NATS backends
- **Documentation Workflow**: Creating new docs with frontmatter templates, validation steps
- **Performance Testing**: Benchmark comparison, load test profiles (quick/standard/stress)
- **Debugging**: gRPC tracing, race detector, container logs
- **CI/CD**: Pre-commit checklist (tests, race detector, coverage, docs validation, builds)
- **Fast Iteration Loop**: Watch mode with auto-rebuild and continuous testing

**Key Facts**: Covers only what exists in the codebase - no invented workflows. Includes actual commands from Makefiles, CLAUDE.md, and tooling scripts. Documents three-layer testing approach (MemStore/SQLite → Docker backends → full load tests) for speed optimization.

**Impact**: Provides single reference for new developers to understand actual development practices. Shows how to speed up multi-tier testing by reusing backends and running partial validations. Establishes consistent mental models for TDD and testing layers.

---

#### CI Validation Fixes - MDX Syntax and Broken Links (UPDATED)
**Links**: [MEMO-009](/memos/memo-009), [MEMO-010](/memos/memo-010), [RFC-029](/rfc/rfc-029)

**Summary**: Fixed MDX compilation errors and broken links identified by CI validation:
- **MEMO-009**: Escaped `<` character in line 87 (`<1KB` → `&lt;1KB`), added `text` language identifier to code fence on line 322, fixed broken link from `/pocs/poc-004-multicast-registry` to `/memos/memo-009` on line 369, updated relative path to absolute GitHub URL on line 372
- **MEMO-010**: Escaped all unescaped `<` characters in performance comparison tables (lines 22, 75, 97, 124, 135, 275, 322, 323) to `&lt;`
- **RFC-029**: Renamed from RFC-022 to RFC-029 (proper RFC numbering sequence)

**Key Facts**: All validation errors resolved. Code fences now have proper language identifiers (prevents "Unexpected FunctionDeclaration" MDX errors). HTML entities properly escaped (`<` → `&lt;`, `>` → `&gt;`). Links updated to use `/memos/` paths instead of deprecated `/pocs/` paths. Full validation passes with GitHub Pages build successful.

**Impact**: CI builds now pass successfully. MDX compilation errors eliminated. Documentation correctly renders in Docusaurus with proper code syntax highlighting. Users can navigate to correct memo pages without 404 errors.

---

#### Documentation Consistency Pass - Pattern SDK Terminology (UPDATED)
**Links**: [RFC-019](/rfc/rfc-019), [RFC-021](/rfc/rfc-021), [MEMO-008](/memos/memo-008), [MEMO-009](/memos/memo-009)

**Summary**: Completed comprehensive consistency pass to align all documentation with RFC-022 terminology change from "Plugin SDK" to "Pattern SDK":
- **RFC-019**: Updated title, module paths (`github.com/prism/plugin-sdk` → `github.com/prism/pattern-sdk`), directory references (`plugins/` → `patterns/`), and all references throughout
- **RFC-021**: Updated all "Plugin SDK" references to "Pattern SDK" and "plugins" to "patterns" in technology stack, work streams, and deliverables
- **MEMO-008**: Updated module path in code examples and directory references in Vault token exchange flow documentation
- **MEMO-009**: Updated cross-reference link to RFC-019 with correct short-form path

**Key Facts**: All 4 documents now use consistent "Pattern SDK" terminology. Cross-references updated to use short-form paths (`/rfc/rfc-019` instead of `/rfc/rfc-019-plugin-sdk-authorization-layer`). Validation passed with no errors. Revision history entries added to all updated documents.

**Impact**: Eliminates terminology confusion between the Go-based Pattern SDK (for backend patterns) and Rust-based plugin SDK (for proxy plugins). Ensures developers reading documentation see consistent terminology across all RFCs and memos. All documentation now accurately reflects the architectural sophistication of the pattern layer.

---

### 2025-10-09

#### RFC-022: Core Pattern SDK - Build System and Tooling Added (MAJOR UPDATE)
**Link**: [RFC-022](/rfc/rfc-022)

**Summary**: Major update transforming RFC-022 from physical code layout to comprehensive build system and tooling guide:
- **Terminology Update**: Renamed from "Plugin SDK" to "Pattern SDK" to reflect pattern layer sophistication
- **Module Path Change**: `github.com/prism/plugin-sdk` → `github.com/prism/pattern-sdk`
- **Directory Structure**: `examples/` → `patterns/` to emphasize pattern implementations
- **Comprehensive Makefile System**: Hierarchical Makefiles for SDK and individual patterns
  - Root Makefile: `all`, `build`, `test`, `test-unit`, `test-integration`, `lint`, `proto`, `clean`, `coverage`, `validate`, `fmt`
  - Pattern-specific Makefiles: Build, test, lint, run, docker, clean targets
  - Build targets reference table with usage guidance
- **Compile-Time Validation**: Interface implementation checks, pattern interface validation, slot configuration validation
  - `interfaces/assertions.go`: Compile-time type assertions for all interfaces
  - `tools/validate-interfaces.sh`: Validates all patterns compile successfully
  - `tools/validate-slots/main.go`: YAML-based slot configuration validator
- **Linting Configuration**: Complete `.golangci.yml` with 12+ enabled linters
  - errcheck, gosimple, govet, ineffassign, staticcheck, typecheck, unused, gofmt, goimports, misspell, goconst, gocyclo, lll, dupl, gosec, revive
  - Test file exclusions, generated file exclusions
  - Pre-commit hook: `.githooks/pre-commit` runs format, lint, validate, test-unit
- **Testing Infrastructure**: Comprehensive test organization and coverage gates
  - Unit tests vs integration tests (build tags)
  - Testcontainers integration (`testing/containers.go`)
  - 80% coverage threshold enforcement
  - Benchmark tests with pattern examples
  - Extended CI/CD workflow with validation, lint, unit, integration, coverage gates

**Key Innovation**: Build system treats patterns as first-class sophisticated implementations, not simple "plugins". Comprehensive tooling ensures quality gates (lint, validate, test, coverage) are enforced at every stage. Makefile-based workflow enables fast iteration with incremental builds and caching. Compile-time validation catches configuration errors before runtime.

**Impact**: Establishes production-grade build infrastructure for Pattern SDK. Pattern authors get consistent Makefile targets, automated validation, and quality gates. Pre-commit hooks prevent broken code from being committed. Coverage gates ensure test quality. Testcontainers enable realistic integration testing. Complements RFC-025 (pattern architecture) by focusing on build system and tooling rather than concurrency primitives.

---

#### MEMO-009: Topaz Local Authorizer Configuration for Development and Integration Testing (NEW)
**Link**: [MEMO-009](/memos/memo-009)

**Summary**: Comprehensive guide for configuring Topaz as local authorizer across two scenarios:
- **Development Iteration**: Fast, lightweight authorization during local development with Docker Compose
- **Integration Testing**: Realistic authorization testing in CI/CD with testcontainers
- Local infrastructure layer: Reusable components (Topaz, Dex, Vault, Signoz) running independently
- Seed data setup with bootstrap script creating test users (dev@local.prism, admin@local.prism) and groups
- Policy files for main authorization (prism.rego) and namespace isolation
- Developer workflow: Docker Compose startup, directory bootstrap, policy hot-reload
- Integration testing: Go testcontainers setup, GitHub Actions CI/CD configuration
- Troubleshooting guide: 4 common issues with diagnosis and solutions
- Pattern SDK integration: Configuration for local Topaz with enforcement modes
- Comparison table: Development vs Integration Testing vs Production configurations

**Key Innovation**: Topaz as local infrastructure layer component enables fast development iteration (&lt;3s startup) and realistic integration testing (&lt;5s per test suite) without external dependencies. Bootstrap script provides reproducible test data. Policy hot-reload eliminates restart cycles.

**Impact**: Completes local development stack for authorization testing. Patterns can develop against realistic authorization without cloud services. testcontainers integration ensures CI/CD tests match production behavior. Establishes reusable local infrastructure pattern for other services (Dex, Vault, Signoz).

---

#### RFC-025: Pattern SDK Architecture - Pattern Lifecycle Management Added (MAJOR UPDATE)
**Link**: [RFC-025](/rfc/rfc-025-pattern-sdk-architecture)

**Summary**: Major expansion adding comprehensive pattern lifecycle management to Pattern SDK architecture:
- **Slot Matching via Config**: Backends validated against union of required interfaces at pattern slots
  - SlotConfig defines interface requirements (keyvalue_basic + keyvalue_scan)
  - SlotMatcher validates backends implement ALL required interfaces before assignment
  - Fail-fast validation: Pattern won't start if slots improperly configured
  - Optional slots supported (e.g., durability slot for event replay)
- **Lifecycle Isolation**: Pattern main separated from program main
  - SDK handles: config loading, backend initialization, slot validation, signal handling
  - Pattern implements: Initialize, Start, Shutdown, HealthCheck
  - Simple cmd/main.go just calls lifecycle.Run(pattern)
- **Graceful Shutdown with Bounded Timeout**: Configurable cleanup timeouts
  - graceful_timeout: 30s (pattern drains in-flight requests)
  - shutdown_timeout: 35s (hard deadline for forced exit)
  - Pattern drains worker pools, closes connections, waits for background goroutines
  - Exit codes: 0 (clean), 1 (errors), 2 (timeout forced)
- **Signal Handling at SDK Level**: OS signals intercepted by SDK
  - SIGTERM/SIGINT → SDK creates shutdown context → calls pattern.Shutdown(ctx)
  - Pattern isolated from signal complexity
  - Consistent signal handling across all patterns
- **Complete Example**: Multicast Registry pattern showing full lifecycle integration
  - Initialize: Extracts validated backends from slots, creates concurrency primitives
  - Start: Launches worker pool, health check loop, blocks until stop signal
  - Shutdown: Drains workers, closes backends, bounded by context timeout
  - HealthCheck: Circuit breaker-protected backend health verification

**Key Innovation**: Slot-based configuration with interface validation eliminates runtime errors from misconfigured backends. Lifecycle isolation keeps patterns focused on business logic while SDK handles cross-cutting concerns. Bounded graceful shutdown ensures clean deployments in Kubernetes (pod termination respects shutdown_timeout).

**Impact**: Patterns become significantly simpler to implement (no signal handling, config parsing, slot validation). Slot matcher prevents "backend doesn't support X interface" runtime errors. Graceful shutdown with hard timeout prevents hung deployments. Foundation for production-grade pattern implementations in POC phases.

---

### 2025-10-09 (Earlier)

#### RFC-019: Plugin SDK Authorization Layer - Token Validation Pushed to Plugins with Vault Integration (ARCHITECTURAL UPDATE)
**Link**: [RFC-019](/rfc/rfc-019-plugin-sdk-authorization-layer)

**Summary**: Major architectural update reflecting critical design decision to push token validation and credential exchange to plugins (not proxy):
- **Architectural Rationale**: Token validation is high-latency (~10-50ms) per-session operation, not per-request
- **Proxy Role Change**: Proxy now passes tokens through without validation (stateless forwarding)
- **Plugin-Side Validation**: Plugins validate tokens once per session, then cache validation result
- **Vault Integration**: Complete implementation of token exchange for per-session backend credentials
  - Plugins exchange validated user JWT for Vault token
  - Vault token used to fetch dynamic backend credentials (username/password)
  - Per-session credentials enable user-specific audit trails in backend logs
  - Automatic credential renewal every lease_duration/2 (background goroutine)
- **VaultClient Implementation**: Complete Go SDK code for JWT login, credential fetching, lease renewal
- **Credential Lifecycle**: Mermaid diagram showing session setup → token exchange → credential renewal → session teardown
- **Configuration Examples**: YAML showing Vault address, JWT auth path, secret path, renewal intervals
- **Vault Policy Examples**: HCL policy for plugin access to database credentials and lease renewal
- **Benefits**: Per-user audit trails, fine-grained ACLs, automatic rotation, rate limiting per user

**Key Innovation**: Token validation amortized over session lifetime (validate once, reuse claims). Vault provides dynamic, short-lived credentials (1h TTL) with user-specific ACLs generated on-demand. Backend logs show which user accessed what data (not just "plugin user"). Zero shared long-lived credentials - breach of one session doesn't compromise others.

**Impact**: Enables true zero-trust architecture with per-session credential isolation. Backend databases can enforce row-level security using Vault-generated credentials. Plugin-side validation creates defense-in-depth even if proxy bypassed. Vault manages entire credential lifecycle (generation, renewal, revocation). Foundation for multi-tenant data access with user attribution.

---

#### RFC-002: Data Layer Interface Specification - Code Fence Formatting Fixes (UPDATED)
**Link**: [RFC-002](/rfc/rfc-002-data-layer-interface)

**Summary**: Fixed 4 MDX code fence validation errors identified by documentation validation tooling:
- Line 1156: Fixed closing fence with ```go → ``` (removed language identifier from closing fence)
- Line 1162: Fixed opening fence missing language → added ```text
- Line 1168: Fixed closing fence with ```bash → ``` (removed language identifier from closing fence)
- Line 1177: Fixed opening fence missing language → added ```text
- Applied state machine-based Python script to distinguish opening fences (require language) from closing fences (must be plain)
- All 4 errors resolved, documentation now passes MDX compilation

**Impact**: RFC-002 now compiles correctly in Docusaurus build. Fixes broken GitHub Pages deployment. Ensures code examples display properly with correct syntax highlighting.

---

#### RFC-023: Publish Snapshotter Plugin - Write-Only Event Buffering with Pagination (NEW)
**Link**: [RFC-023](/rfc/rfc-023-publish-snapshotter-plugin)

**Summary**: Comprehensive RFC defining publish snapshotter plugin architecture for write-only event capture with intelligent buffering:
- **Write-Only API**: Satisfies PubSub publish interface only (no subscription)
- Intelligent buffering with configurable thresholds (event count, size, age)
- Page-based commits to object storage (S3, MinIO, local filesystem)
- Index publishing to KeyValue/TimeSeries/Document backends for discovery
- Session disconnect handling with guaranteed zero data loss
- Format flexibility: Protobuf or NDJSON serialization with optional compression (gzip/zstd)
- Two backend slots: storage_object (new interface) + index backend (KeyValue/TimeSeries/Document)
- Complete page lifecycle: buffer → serialize → compress → write → index → clear
- Query and replay capabilities using index metadata
- Performance characteristics: 10,000 events/page, 300s max age, &lt;1GB RAM per 1000 writers
- Configuration examples: development (MemStore + local filesystem), production (Redis + MinIO), large scale (ClickHouse + S3)

**Key Innovation**: Write-only event capture decouples data producers from consumers, enabling durable event archival without active subscribers. Two-slot architecture separates storage (object storage) from indexing (KeyValue/TimeSeries) for flexibility. Page-based commits provide efficient large-file writes while maintaining discoverability through side-channel index.

**Impact**: Enables audit logging, event archival, data lake ingestion, session recording, and metrics collection patterns without requiring active consumers. Zero data loss guarantee even on disconnects. Object storage economics (cheap, durable) combined with queryable index. Adds new storage_object interface to MEMO-006 catalog.

---

#### RFC-022: Core Plugin SDK Physical Code Layout (NEW)
**Link**: [RFC-022](/rfc/rfc-022-core-plugin-sdk-code-layout)

**Summary**: Comprehensive RFC defining physical code layout for publishable Go SDK (`github.com/prism/plugin-sdk`) for building backend plugins:
- **Package Structure**: 9 packages (auth, authz, audit, plugin, interfaces, storage, observability, testing, errors)
- Clean separation: Authentication (JWT/OIDC), authorization (Topaz), audit logging, lifecycle management
- Interface contracts matching protobuf service definitions (KeyValue, PubSub, Stream, Queue, etc.)
- Observability built-in: structured logging (Zap), Prometheus metrics, OpenTelemetry tracing
- Testing utilities: mock implementations for auth/authz/audit, test server helpers
- Minimal external dependencies: gRPC, protobuf, JWT libraries, Topaz SDK, Zap, Prometheus, OTel
- Semantic versioning strategy with Go modules (v0.x.x pre-1.0, v1.x.x stable, v2.x.x breaking)
- Complete example: MemStore plugin using SDK (150 lines vs 500+ without SDK)
- Automated releases with GitHub Actions
- godoc-friendly documentation with usage examples per package

**Key Innovation**: Batteries-included SDK enables plugin authors to focus on backend-specific logic while SDK handles cross-cutting concerns (auth, authz, audit, observability, lifecycle). Defense-in-depth authorization built into SDK following RFC-019 patterns. Reusable abstractions eliminate code duplication across plugins.

**Impact**: Accelerates plugin development with consistent patterns. Ensures all plugins enforce authorization, emit audit logs, and expose observability metrics. Reduces security vulnerabilities through centralized auth logic. Single SDK version bump propagates improvements to all plugins. Foundation for POC 1 implementation (RFC-021).

---

#### RFC-021: POC 1 - Three Minimal Plugins Implementation Plan (COMPLETE REWRITE)
**Link**: [RFC-021](/rfc/rfc-021-poc1-three-plugins-implementation)

**Summary**: Complete rewrite of POC 1 implementation plan based on user feedback for focused, test-driven approach:
- **Scope Changes**: Removed Admin API (use prismctl CLI), removed Python client library, split into 3 minimal plugins
- Three focused plugins: MemStore (in-memory, 6 interfaces), Redis (external, 8 interfaces), Kafka (streaming, 7 interfaces)
- Core Plugin SDK skeleton: Reusable Go library from RFC-022 with auth/authz/audit stubs
- Load testing tool: Go CLI (prism-load) for parallel request generation with configurable concurrency, duration, RPS
- Optimized builds: Static linking (`CGO_ENABLED=0`), scratch-based Docker images (&lt;10MB target)
- TDD workflow: Write tests FIRST, achieve 80%+ code coverage (SDK: 85%+, plugins: 80%+)
- Go module caching: Shared GOMODCACHE and GOCACHE across monorepo to avoid duplicate builds
- Plugin lifecycle diagram: 4 phases (startup, request handling, health checks, shutdown)
- 8 work streams with dependencies: Protobuf (1 day), SDK (2 days), Proxy (3 days), 3 plugins (2 days each), Load tester (1 day), Build optimization (1 day)
- Timeline: 2 weeks (10 working days) with parallelizable work streams
- Success criteria: All tests pass, 80%+ coverage, &lt;5ms P99 latency, &lt;10MB Docker images

**Key Innovation**: Walking Skeleton approach proves architecture end-to-end with minimal scope. Three focused plugins demonstrate SDK reusability and different backend patterns (in-process, external cache, external streaming). TDD workflow with mandatory code coverage gates ensures quality from day one. Load testing validates performance claims early.

**Impact**: Clear, achievable POC scope replacing original overly-complex plan. SDK skeleton provides foundation for all future plugins. Static linking enables lightweight deployments. TDD discipline establishes engineering culture. Load tester enables continuous performance validation. Coverage thresholds prevent quality regressions.

---

#### RFC-015: Plugin Acceptance Test Framework - Interface-Based Testing (COMPLETE REWRITE)
**Link**: [RFC-015](/rfc/rfc-015-plugin-acceptance-test-framework)

**Summary**: Complete rewrite aligning with MEMO-006 interface decomposition principles, shifting from backend-type testing to interface-based testing:
- **Interface Compliance**: 45 interface test suites (one per interface from MEMO-006 catalog)
- Cross-backend test reuse: Same test suite validates multiple backends implementing same interface
- Registry-driven testing: Backends declare interfaces in `registry/backends/*.yaml`, tests verify claims
- Compliance matrix: Automated validation that backends implement all declared interfaces
- Test organization: `tests/acceptance/interfaces/{datamodel}/{interface}_test.go` structure
- testcontainers integration: Real backend instances (Redis, Postgres, Kafka) for integration testing
- Example test suites: KeyValueBasicTestSuite (10 tests), KeyValueScanTestSuite (6 tests)
- GitHub Actions CI: Matrix strategy runs interface × backend combinations (45 interfaces × 4 backends = 180 jobs)
- Backend registry loading: `LoadBackendRegistry()` reads YAML declarations, `FindBackendsImplementing()` filters by interface
- Makefile targets: `test-compliance`, `test-compliance-redis`, `test-interface INTERFACE=keyvalue_basic`

**Key Innovation**: Interface-based testing enables test code reuse across backends. Single `KeyValueBasicTestSuite` validates Redis, PostgreSQL, DynamoDB, MemStore - reduces 1500 lines (duplicated) to 100 lines (shared). Registry-driven execution ensures only declared interfaces are tested (no false failures).

**Impact**: Dramatically reduces test maintenance burden. Establishes clear interface contracts through executable specifications. Backend registry becomes single source of truth for capabilities. Compliance matrix provides confidence that backends satisfy declared interfaces. Foundation for plugin acceptance testing in CI/CD pipeline.

---

#### RFC-020: Streaming HTTP Listener - API-Specific Adapter Pattern (NEW)
**Link**: [RFC-020](/rfc/rfc-020-streaming-http-listener-api-adapter)

**Summary**: Comprehensive RFC defining streaming HTTP listener architecture that bridges external HTTP/JSON protocols to Prism's internal gRPC/Protobuf layer:
- **API-Specific Adapters**: Each adapter implements ONE external API contract (MCP, Agent-to-Agent, custom APIs)
- Thin translation layer with no business logic (pure protocol translation)
- Streaming support: SSE (Server-Sent Events), WebSocket, HTTP chunked encoding
- Three deployment options: sidecar, separate service, or serverless (AWS Lambda)
- MCP backend interface decomposition: 5 interfaces across 3 data models (KeyValue, Queue, Stream)
- New MCP interfaces: mcp_tool (tool calling), mcp_resource (resource access), mcp_prompt (prompt templates)
- AI tool orchestration pattern: Combines MCP backend + execution queue + result stream
- Performance: &lt;3ms P95 adapter overhead, 30,000 RPS with HTTP/JSON translation
- Complete Go implementation examples with protocol translation helpers
- Configuration examples for MCP tool server, SSE event streaming, and agent-to-agent coordination

**Key Innovation**: API-specific adapters satisfy external protocol contracts while transparently mapping to internal gRPC primitives. MCP treated as backend plugin with decomposed interfaces following MEMO-006 principles. Enables AI tool calling, resource access, and prompt management via HTTP/JSON while leveraging Prism's backend flexibility.

**Impact**: Enables seamless integration of HTTP-based APIs (MCP, A2A) with Prism's gRPC core without modifying proxy. Easy adapter authoring pattern for new protocols. MCP backend decomposition provides foundation for AI tool orchestration with queue-based execution and result streaming.

---

#### ADR-050: Topaz for Policy-Based Authorization (NEW)
**Link**: [ADR-050](/adr/adr-050-topaz-policy-authorization)

**Summary**: Architecture decision to use Topaz by Aserto for fine-grained policy-based authorization:
- **Topaz Selection**: Evaluated OPA alone, cloud IAM, Zanzibar systems (SpiceDB, Ory Keto) - Topaz chosen for best balance
- Relationship-Based Access Control (ReBAC) inspired by Google Zanzibar
- Sidecar deployment pattern for &lt;5ms P99 local authorization checks
- Complete integration examples: Rust proxy middleware, Python CLI, FastAPI admin UI
- Directory schema modeling users, groups, namespaces, backends with relationships
- Three example policies: namespace isolation, time-based maintenance windows, PII protection
- Performance: P50 &lt;0.5ms, P95 &lt;2ms, P99 &lt;5ms for local sidecar checks
- 10,000+ authorization checks per second capacity
- Docker Compose for local dev, Kubernetes sidecar for production
- Fail-closed by default with opt-in fail-open per namespace

**Key Innovation**: Local sidecar authorization combines OPA's policy expressiveness with Zanzibar-style relationship modeling. Real-time policy updates without proxy restarts. Centralized policy management (Git) with decentralized enforcement (local sidecars).

**Impact**: Enables multi-tenancy isolation, role-based access control, attribute-based policies, and resource-level permissions with production-ready performance. Foundation for defense-in-depth security across proxy and plugin layers.

---

#### RFC-019: Plugin SDK Authorization Layer (NEW)
**Link**: [RFC-019](/rfc/rfc-019-plugin-sdk-authorization-layer)

**Summary**: Standardized authorization layer in Prism core plugin SDK enabling backend plugins to validate tokens and enforce policies:
- **Defense-in-Depth**: Authorization enforced at proxy AND plugin layers
- Three core components: TokenValidator (JWT/OIDC), TopazClient (policy queries), AuditLogger (structured logging)
- Complete Go SDK implementation with Authorizer interface orchestrating all components
- gRPC interceptors for automatic authorization on all plugin methods
- Token validation with JWKS caching (&lt;1ms with caching)
- Topaz policy checks with 5s decision caching (&lt;1ms P99 cache hit)
- Async audit logging with buffered events (&lt;0.1ms overhead)
- Fail-closed by default with configurable fail-open for local testing
- Configuration examples: production (token + policy enabled) vs local dev (disabled with audit)
- Plugin integration patterns: automatic (gRPC interceptor) vs manual (fine-grained control)

**Key Innovation**: Backend plugins validate authorization independently of proxy, creating defense-in-depth security. SDK provides reusable authorization primitives so plugins just call SDK (no reimplementation). Authorization overhead &lt;3ms P99 with caching enabled.

**Impact**: Eliminates plugin-level security vulnerabilities. Prevents bypassing proxy authorization by connecting directly to plugins. Consistent policy enforcement across all backends. Complete audit trail of data access at plugin layer. Enables zero-trust architecture.

---

#### MEMO-006: Backend Interface Decomposition and Schema Registry (NEW)
**Link**: [MEMO-006](/memos/memo-006-backend-interface-decomposition-schema-registry)

**Summary**: Comprehensive architectural guide for decomposing backends into thin, composable proto service interfaces and establishing schema registry for patterns and slots:
- **Design Decision**: Use explicit interface flavors (not capability flags) for type safety
- 45 thin interfaces across 10 data models (KeyValue, PubSub, Stream, Queue, List, Set, SortedSet, TimeSeries, Graph, Document)
- Each data model has multiple interfaces: Basic (required), Scan, TTL, Transactional, Batch, etc.
- Backend implementation matrix showing interface composition (Redis: 16 interfaces, Postgres: 16 different mix, MemStore: 2 minimal)
- Pattern schemas with slots requiring specific interface combinations
- Schema registry filesystem layout (registry/interfaces/, registry/backends/, registry/patterns/)
- Configuration generator workflow with validation
- Examples: Redis implements keyvalue_basic + keyvalue_scan + keyvalue_ttl + keyvalue_transactional + keyvalue_batch
- Capabilities expressed through interface presence (TTL support = implements keyvalue_ttl interface)

**Key Innovation**: Thin interfaces enable type-safe backend composition. Pattern slots specify required interfaces (e.g., Multicast Registry needs keyvalue_basic + keyvalue_scan for registry slot). No runtime capability checks - compiler enforces contracts.

**Impact**: Enables straightforward config generation, backend substitutability, and clear contracts for what each backend supports. Foundation for schema-driven pattern composition.

---

#### MEMO-005: Client Protocol Design Philosophy - Composition vs Use-Case Specificity (NEW)
**Link**: [MEMO-005](/memos/memo-005-client-protocol-design-philosophy)

**Summary**: Comprehensive memo resolving the architectural tension between composable primitives (RFC-014) and use-case-specific protocols (RFC-017), covering:
- Context comparison: RFC-014 composable primitives vs RFC-017 use-case patterns
- Four design principles (push complexity down, developer comprehension, schema evolution, keep proxy small)
- Proposed layered API architecture: Layer 1 (generic primitives) + Layer 2 (use-case patterns)
- Pattern coordinators as plugins (not core proxy) for independent evolution
- Configuration examples showing per-namespace choice of primitives vs patterns
- Decision matrix comparing primitives-only, patterns-only, and layered approaches
- Implementation roadmap aligned with RFC-018 POCs
- Success metrics for developer experience, system complexity, and pattern adoption

**Key Innovation**: Applications choose per-namespace between Layer 1 (generic KeyValue, PubSub) for maximum control or Layer 2 (ergonomic Multicast Registry, Saga) for rapid development. Pattern coordinators are optional plugins that compose Layer 1 primitives, keeping core proxy small (~10k LOC) while providing self-documenting APIs for common use cases.

**Impact**: Resolves "composition vs use-case" design question with both layers, addressing developer simplicity (Layer 2), proxy size (plugins), and flexibility (Layer 1).

---

#### MEMO-003: Documentation-First Development Approach (NEW)
**Link**: [MEMO-003](/memos/memo-003-documentation-first-development)

**Summary**: Comprehensive memo defining the documentation-first development approach used in Prism, covering:
- Definition and core principles (Design in Documentation → Review → Implement → Validate)
- Notable improvements over code-first workflows with concrete examples
- Expected outcomes (faster reviews, better designs, reduced rework)
- Strategies for success (blocking requirements, design tool, living documentation)
- Validation and quality assurance (tooling/validate_docs.py)
- Metrics and success criteria (documentation coverage, build success rate, review velocity)
- Proposed improvements (code example validation, decision graph visualization, RFC-driven task generation)

**Impact**: Establishes documentation-first as the core development methodology, with validation tooling as a blocking requirement before commits.

---

#### RFC-011: Data Proxy Authentication - Secrets Provider Abstraction (EXPANDED)
**Link**: [RFC-011](/rfc/rfc-011-data-proxy-authentication)

**Summary**: Major expansion adding comprehensive secrets provider abstraction:
- Pluggable SecretsProvider trait supporting multiple secret management services
- Four provider implementations: HashiCorp Vault, AWS Secrets Manager, Google Secret Manager, Azure Key Vault
- Provider comparison matrix (dynamic credentials, auto-rotation, versioning, audit logging, cost)
- Multi-provider hybrid cloud deployment patterns
- Configuration examples for each provider
- Credential management with automatic caching and renewal

**Impact**: Enables secure credential management across cloud providers and on-premises deployments with consistent abstraction layer.

---

#### RFC-006: Admin CLI - OIDC Authentication (EXPANDED)
**Link**: [RFC-006](/rfc/rfc-006-python-admin-cli)

**Summary**: Added comprehensive OIDC authentication section covering:
- Device code flow (OAuth 2.0) for command-line SSO authentication
- Mermaid sequence diagram showing complete authentication flow
- Login/logout commands with token caching (~/.prism/token)
- Token storage security (file permissions 0600, automatic refresh)
- Authentication modes (interactive, service account, local Dex, custom issuer)
- Go implementation examples for token management
- Local development with Dex (references ADR-046)
- Principal column added to session list output
- Shadow traffic example updated to Postgres version upgrade (14 → 16) use case

**Impact**: Complete CLI authentication specification enabling secure admin access with OIDC integration and local testing support.

---

#### ADR-046: Dex IDP for Local Identity Testing (NEW)
**Link**: [ADR-046](/adr/adr-046-dex-idp-local-testing)

**Summary**: New ADR proposing Dex as the local OIDC provider for development and testing:
- Self-hosted OIDC provider for local development (no cloud dependencies)
- Docker Compose integration with test user configuration
- Full OIDC spec support including device code flow
- Integration with prismctl for local authentication
- Testing workflow with realistic OIDC flows

**Impact**: Enables local development and testing of authentication features without external OIDC provider dependencies.

---

#### RFC-014: Layered Data Access Patterns - Client Pattern Catalog (EXPANDED)
**Link**: [RFC-014](/rfc/rfc-014-layered-data-access-patterns)

**Summary**: New RFC defining how Prism separates client API from backend implementation through pattern composition. Covers:
- Three-layer architecture (Client API, Pattern Composition, Backend Execution)
- Publisher with Claim Check pattern implementation
- Pattern layering and compatibility matrix
- Proxy internal structure with mermaid diagrams
- Authentication and authorization flow diagrams
- Pattern routing and execution strategies

**Impact**: Provides foundation for composable reliability patterns without client code changes.

---

#### RFC-011: Data Proxy Authentication - Open Questions Expanded
**Link**: [RFC-011](/rfc/rfc-011-data-proxy-authentication)

**Summary**: Added comprehensive feedback to open questions:
- Certificate Authority: Use Vault for certificate management
- Credential Caching: 24-hour default, configurable with refresh tokens
- Connection Pooling: Per-credential pooling for multi-tenancy isolation
- Fallback Auth: Fail closed with configurable grace period
- Observability: Detailed metrics for credential events and session establishment

**Impact**: Clarifies authentication implementation decisions with practical recommendations.

---

#### RFC-010: Admin Protocol with OIDC - Multi-Provider Support
**Link**: [RFC-010](/rfc/rfc-010-admin-protocol-oidc)

**Summary**: Expanded open questions with detailed answers:
- OIDC Provider Support: AWS Cognito, Azure AD, Google, Okta, Auth0, Dex
- Token Caching: 24-hour default with JWKS caching and refresh token support
- Offline Access: JWT validation with cached JWKS, security trade-offs
- Multi-Tenancy: Four mapping options (group-based, claim-based, OPA policy, tenant-scoped)
- Service Accounts: Four approaches with comparison table and best practices

**Impact**: Production-ready guidance for OIDC integration across multiple identity providers.

---

#### RFC-009: Distributed Reliability Patterns - Change Notification Graph
**Link**: [RFC-009](/rfc/rfc-009-distributed-reliability-patterns)

**Summary**: Added change notification flow diagram to CDC pattern showing:
- Change type classification (INSERT, UPDATE, DELETE, SCHEMA)
- Notification consumers (Cache Invalidator, Search Indexer, Analytics Loader, Webhook Notifier, Audit Logger)
- Data flow from PostgreSQL WAL through Kafka to downstream systems
- Key notification patterns and use cases

**Impact**: Visual guide for implementing CDC-based change notification architectures.

---

## Older Changes

### 2025-10-08

#### RFC-009: Distributed Reliability Patterns (INITIAL)
**Link**: [RFC-009](/rfc/rfc-009-distributed-reliability-patterns)

**Summary**: Initial RFC documenting 7 distributed reliability patterns:
1. Tiered Storage - Hot/warm/cold data lifecycle
2. Write-Ahead Log - Durable, fast writes
3. Claim Check - Large payload handling in messaging
4. Event Sourcing - Immutable event log as source of truth
5. Change Data Capture - Database replication without dual writes
6. CQRS - Separate read/write models
7. Outbox - Transactional messaging

**Impact**: Establishes pattern catalog for building reliable distributed systems.

---

### 2025-10-07

#### RFC-001: Prism Architecture (INITIAL)
**Link**: [RFC-001](/rfc/rfc-001-prism-architecture)

**Summary**: Foundational architecture RFC defining:
- System components and layered interface hierarchy
- Client configuration system (server vs client config)
- Session management lifecycle
- Five interface layers (Queue, PubSub, Reader, Transact, Config)
- Container plugin model for backend-specific logic
- Performance targets (P99 &lt;10ms, 10k+ RPS)

**Impact**: Core architectural vision for Prism data access gateway.

---

#### RFC-002: Data Layer Interface Specification (INITIAL)
**Link**: [RFC-002](/rfc/rfc-002-data-layer-interface)

**Summary**: Complete gRPC interface specification covering:
- Session Service (authentication, heartbeat, lifecycle)
- Queue Service (Kafka-style operations)
- PubSub Service (NATS-style wildcards)
- Reader Service (database-style paged reading)
- Transact Service (two-table transactional writes)
- Error handling and backward compatibility

**Impact**: Stable, versioned API contracts for all client interactions.

---

## How to Use This Log

1. **Quick Navigation**: Click any link to jump directly to the updated document
2. **Impact Assessment**: Each entry includes an "Impact" section explaining significance
3. **Reverse Chronological**: Newest changes at the top for easy discovery
4. **Detailed Summaries**: Key changes summarized without needing to read full docs

## Contributing Changes

When updating documentation:

1. Add entry to "Recent Changes" section (top)
2. Include: Date, Document title, Link, Summary, Impact
3. Move entries older than 30 days to "Older Changes"
4. Keep most recent 10-15 entries in "Recent Changes"

## Change Categories

- **NEW**: Brand new documentation
- **EXPANDED**: Significant additions to existing docs
- **UPDATED**: Modifications or clarifications
- **DEPRECATED**: Marked as outdated or superseded
- **REMOVED**: Deleted or consolidated
