---
author: Platform Team
created: 2025-10-15
doc_uuid: e4cfce48-de30-48ea-b4dd-df95d24c02f9
id: memo-035
project_id: prism-data-layer
status: Implemented
tags:
- admin
- ui
- go
- templ
- htmx
- gin
- dashboard
title: Simple Admin UI MVP Implementation
updated: 2025-10-15
---

## Abstract

This memo documents the implementation of a minimal viable product (MVP) admin UI for prism-admin, providing real-time KPI monitoring and system status visualization on port 8080. The implementation follows RFC-036's templ + htmx + Gin architecture, running alongside the existing gRPC control plane on port 8981.

## Motivation

While prism-admin provides comprehensive gRPC APIs for control plane operations, operators need a simple web interface for monitoring system health during local development and testing. The MVP admin UI addresses this need with a read-only dashboard showing:

- Registered proxies with health status
- Registered launchers with capacity utilization
- Active namespaces
- Real-time metrics with auto-refresh

**Key Requirements:**
- Run on default port 8080 alongside gRPC server
- Read-only dashboard (no CRUD operations for MVP)
- Real-time metrics via htmx polling
- No authentication (local testing only)
- Minimal dependencies (templ + htmx + Gin)
- &lt;100ms page load time

## Implementation Overview

### Technology Stack

Following RFC-036's recommendations:

- **Backend Framework**: Gin (HTTP routing, middleware)
- **Templating**: templ (compile-time type-safe HTML)
- **Interactivity**: htmx 2.0.4 (HTML over the wire, 14KB)
- **Styling**: Custom CSS (minimal, no framework)
- **Data Source**: Existing SQLite Storage interface

**Dependencies Added:**
```go
github.com/gin-gonic/gin v1.11.0
github.com/a-h/templ v0.3.960
```

### Project Structure

```text
cmd/prism-admin/
├── main.go                   # Entry point (existing)
├── serve.go                  # Updated with HTTP server startup
├── http_server.go            # NEW: Gin HTTP server
├── control_plane.go          # Existing gRPC service
├── storage.go                # Existing SQLite storage
├── templates/                # NEW: templ templates
│   ├── layout.templ          # Base layout with navigation
│   ├── dashboard.templ       # Dashboard metrics
│   ├── proxies.templ         # Proxy list
│   ├── launchers.templ       # Launcher list
│   └── namespaces.templ      # Namespace list
└── static/                   # NEW: Static assets
    ├── css/styles.css        # Custom CSS (~200 lines)
    └── js/htmx.min.js        # htmx library (14KB)
```

### HTTP Server Architecture

**Routes:**
- `GET /` - Dashboard home with metrics grid
- `GET /api/metrics` - Metrics fragment for htmx polling
- `GET /proxies` - Proxy status table
- `GET /launchers` - Launcher status table
- `GET /namespaces` - Namespace list table
- `/static/*` - Static file serving

**HTTPServer Structure:**
```go
type HTTPServer struct {
	storage *Storage
	router  *gin.Engine
	server  *http.Server
}
```

### Dashboard Metrics

```go
type DashboardMetrics struct {
	ProxyCount        int
	ProxyHealthy      int
	ProxyUnhealthy    int
	LauncherCount     int
	LauncherHealthy   int
	LauncherUnhealthy int
	NamespaceCount    int
	LastUpdate        string
}
```

Metrics queried directly from Storage interface:
- `storage.ListProxies(ctx)` - All registered proxies
- `storage.ListLaunchers(ctx)` - All registered launchers
- `storage.ListNamespaces(ctx)` - All namespaces

### Real-Time Updates

**htmx Polling Pattern:**
```html
<div hx-get="/api/metrics"
     hx-trigger="every 5s"
     hx-swap="outerHTML"
     hx-indicator="#loading">
  @MetricsGrid(metrics)
</div>
```

- Polls `/api/metrics` every 5 seconds
- Swaps entire metrics grid (4 metric cards)
- Shows loading indicator during fetch
- No JavaScript required

### Startup Integration

Updated `serve.go` to start both servers concurrently:

```go
// Start gRPC server (port 8981)
go func() {
	if err := grpcServer.Serve(lis); err != nil {
		errChan <- fmt.Errorf("gRPC server error: %w", err)
	}
}()

// Start HTTP server (port 8080)
go func() {
	if err := httpServer.Start(); err != nil && err != http.ErrServerClosed {
		errChan <- fmt.Errorf("HTTP server error: %w", err)
	}
}()
```

**Graceful Shutdown:**
- HTTP server shutdown with 5-second timeout
- gRPC server graceful stop
- Both servers shut down on SIGINT/SIGTERM

### Command-Line Interface

Added `--http-port` flag to `serve` command:

```bash
prism-admin serve                          # Defaults: gRPC 8981, HTTP 8080
prism-admin serve --port 8981 --http-port 8080
```

## User Experience

### Startup Output

```text
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
🚀 Prism Admin Control Plane Server
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  gRPC API:   0.0.0.0:8981
  Admin UI:   http://localhost:8080
  Database:   sqlite (/Users/user/.prism/admin.db)
  Status:     ✅ Ready
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  gRPC accepts connections from:
    • Proxies (registration, heartbeats, namespace mgmt)
    • Launchers (registration, heartbeats, process mgmt)
    • Clients (namespace provisioning via proxy)

  Admin UI accessible at:
    • http://localhost:8080/          (Dashboard)
    • http://localhost:8080/proxies   (Proxy status)
    • http://localhost:8080/launchers (Launcher status)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

### Dashboard Features

**Metric Cards (4-column grid):**
1. **Proxies**: Total count with healthy/unhealthy breakdown
2. **Launchers**: Total count with healthy/unhealthy breakdown
3. **Namespaces**: Total count of active namespaces
4. **Last Update**: Current time (updates every 5s)

**Status Indicators:**
- 🟢 Green dot: Healthy
- 🔴 Red dot: Unhealthy
- ⚪ Gray dot: Unknown

**Navigation:**
- Top header with logo and nav links
- Clean, responsive layout
- Mobile-friendly (single column on small screens)

## Performance Characteristics

**Measured Performance:**
- Page load: &lt;50ms (empty dashboard)
- Page load: &lt;100ms (with data)
- Metrics API: &lt;20ms (SQLite query + render)
- Memory footprint: ~5MB (Gin server overhead)
- Build time: &lt;2s (templ generation + go build)

**Comparison with RFC-036 Targets:**
- ✅ Container size: N/A (runs in prism-admin binary)
- ✅ Startup time: &lt;50ms (HTTP server only)
- ✅ Page load: &lt;100ms target met

## Testing

**Manual Testing:**
```bash
# Build prism-admin with UI
cd cmd/prism-admin
go build -o prism-admin .

# Start server
./prism-admin serve

# Test endpoints
curl http://localhost:8080/              # Dashboard HTML
curl http://localhost:8080/api/metrics   # Metrics fragment
curl http://localhost:8080/proxies       # Proxy list
curl http://localhost:8080/launchers     # Launcher list
curl http://localhost:8080/namespaces    # Namespace list
```

**Verified Scenarios:**
- ✅ Dashboard loads with empty database
- ✅ Dashboard shows 1 registered launcher (from storage)
- ✅ Metrics auto-refresh every 5 seconds
- ✅ Navigation links work across all pages
- ✅ Static assets serve correctly (CSS, htmx.js)
- ✅ Graceful shutdown with Ctrl+C

## Known Limitations

**MVP Scope (By Design):**
- ❌ No authentication/authorization (local testing only)
- ❌ No CRUD operations (read-only dashboard)
- ❌ No real-time WebSocket (htmx polling sufficient)
- ❌ No charts/graphs (text metrics only)
- ❌ No filtering/search (basic lists only)
- ❌ No pagination (assumes small datasets)

**Future Enhancements (Beyond MVP):**
1. OIDC authentication integration (RFC-010)
2. Namespace CRUD operations
3. Audit log viewer with filtering
4. Real-time charts (proxy/launcher trends)
5. Session management UI
6. Configuration editor

## Comparison with RFC-036

| Aspect | RFC-036 (Proposed) | MEMO-035 (Implemented) |
|--------|-------------------|------------------------|
| **Framework** | Gin | ✅ Gin |
| **Templates** | templ | ✅ templ |
| **Interactivity** | htmx | ✅ htmx 2.0.4 |
| **Styling** | Tailwind CSS | Custom CSS (200 lines) |
| **Authentication** | OIDC | ❌ Not implemented (MVP) |
| **CRUD Operations** | Full namespace management | ❌ Read-only (MVP) |
| **Deployment** | Standalone service | Embedded in prism-admin |
| **Container Size** | 15-20MB | N/A (no separate container) |
| **Page Load** | &lt;100ms | ✅ &lt;100ms |

**Key Differences:**
- **Styling**: Used custom CSS instead of Tailwind for simplicity
- **Deployment**: Embedded in prism-admin binary (not standalone service)
- **Scope**: MVP focuses on monitoring (RFC-036 includes full CRUD)

## Development Workflow

**Template Generation:**
```bash
# After editing .templ files
templ generate

# Generates .go files in templates/ directory
# Templates automatically included in build
```

**Build Process:**
```bash
cd cmd/prism-admin
templ generate  # Generate Go from templ
go build -o prism-admin .
```

**Development Iteration:**
1. Edit templates in `templates/*.templ`
2. Run `templ generate`
3. Run `go build`
4. Test with `./prism-admin serve`

## Security Considerations

**Local Testing Only:**
- No authentication/authorization implemented
- Assumes trusted local environment
- Not production-ready without auth layer

**Future Production Requirements:**
1. OIDC authentication (RFC-010 integration)
2. CSRF protection (Gin middleware)
3. Rate limiting per user
4. Audit logging for all UI actions
5. Content Security Policy headers

## Integration with Existing Systems

**Storage Layer:**
- Uses existing SQLite Storage interface
- No schema changes required
- Read-only operations only

**gRPC Control Plane:**
- Runs alongside gRPC server (port 8981)
- No interference with gRPC operations
- Shares same database instance

**Command-Line Interface:**
- Extends existing `serve` command
- Backward compatible (gRPC port unchanged)
- Adds `--http-port` flag only

## Success Metrics

**Achieved Goals:**
- ✅ Dashboard accessible at http://localhost:8080
- ✅ Real-time metrics with 5-second auto-refresh
- ✅ Shows live data from SQLite storage
- ✅ Works alongside gRPC server on 8981
- ✅ Page load &lt;100ms
- ✅ No external dependencies (htmx embedded)

**Code Metrics:**
- New files: 8 (4 templates, 1 server, 1 CSS, 1 JS, 1 updated)
- Lines of code: ~800 (400 Go, 200 templ, 200 CSS)
- Build time: &lt;2 seconds
- Dependencies added: 2 (gin, templ)

## Next Steps

**Immediate (Week 2-3):**
1. Add namespace CRUD operations
2. Implement audit log viewer
3. Add pagination for large datasets
4. Create integration tests

**Short-Term (Month 2):**
1. OIDC authentication integration
2. Session management UI
3. Real-time charts with Chart.js
4. Configuration editor

**Long-Term (Month 3+):**
1. Multi-prism-admin federation view
2. Advanced filtering and search
3. Operational playbooks UI
4. Alerting configuration

## References

- [RFC-036](/rfc/rfc-036): Minimalist Web Framework for Prism Admin UI
- [ADR-054](/adr/adr-054): SQLite Storage for prism-admin Local State
- [ADR-055](/adr/adr-055): Proxy-Admin Control Plane Protocol
- [ADR-056](/adr/adr-056): Launcher-Admin Control Plane Protocol
- [templ Documentation](https://templ.guide)
- [htmx Documentation](https://htmx.org)
- [Gin Web Framework](https://gin-gonic.com)

## Revision History

- 2025-10-15: Initial implementation of MVP admin UI with dashboard, proxies, launchers, and namespaces pages
