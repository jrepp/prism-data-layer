---
author: Platform Team
created: 2025-10-15
doc_uuid: f7a3c8e5-2d9f-4a16-b8e2-1c3d4e5f6a7b
id: rfc-036
project_id: prism-data-layer
status: Proposed
tags:
- admin
- ui
- go
- templ
- htmx
- web
title: Minimalist Web Framework for Prism Admin UI
updated: 2025-10-15
---

## Abstract

This RFC proposes using **templ + htmx + Gin** as an alternative web framework for the Prism Admin UI, replacing the FastAPI + gRPC-Web + Vanilla JavaScript stack proposed in ADR-028. This approach provides server-side rendering with progressive enhancement, eliminates the need for a separate Python service, and aligns the admin UI with our Go-first ecosystem while maintaining simplicity and avoiding heavy JavaScript frameworks.

## Motivation

The current admin UI design (ADR-028) uses FastAPI (Python) as a gRPC-Web proxy serving static files. While functional, it introduces several challenges that this RFC aims to address.

**Current Challenges:**
1. **Language Fragmentation**: Python service alongside Go proxy, plugins, and CLI
2. **Deployment Complexity**: Separate container for admin UI (100-150MB vs 15-20MB Go binary)
3. **Runtime Dependencies**: Python interpreter, pip packages, uvicorn
4. **Maintenance Overhead**: Two languages for admin functionality (prismctl in Go + UI in Python)
5. **Performance**: 1-2 second startup time vs <50ms for Go binary

**Goals:**
- Consolidate admin UI into Go ecosystem (same language as proxy/plugins/CLI)
- Eliminate Python dependency for admin UI
- Maintain simplicity (no React/Vue/Angular complexity)
- Preserve progressive enhancement approach (HTML over the wire)
- Enable rapid UI development with compile-time type safety
- Reduce deployment footprint (single Go binary vs Python + deps)

**Non-Goals:**
- Replace prismctl (Go CLI remains primary admin tool)
- Build rich SPA-style interactions (admin UI is primarily CRUD)
- Support offline-first or complex client-side state management
- Real-time collaborative editing features

## Proposed Design

### Core Concept

**Server-side rendering with progressive enhancement**: Return HTML fragments from Go handlers, use htmx to swap them into the DOM. No JSON APIs between UI and handlers, no JavaScript build step, no client-side state management.

### Architecture Overview

\`\`\`text
┌─────────────────────────────────────────────────────────┐
│                      Browser                            │
│  ┌───────────────────────────────────────────────────┐ │
│  │  HTML Pages (rendered by templ)                   │ │
│  │  - Namespace management                           │ │
│  │  - Session monitoring                             │ │
│  │  - Backend health dashboard                       │ │
│  └────────────────┬──────────────────────────────────┘ │
│                   │ HTML/HTTP (htmx AJAX)               │
└───────────────────┼─────────────────────────────────────┘
                    │
┌───────────────────▼─────────────────────────────────────┐
│  Prism Admin UI Service (Gin + templ) (:8000)          │
│                                                          │
│  ┌────────────────────────────────────────────────────┐ │
│  │  Gin HTTP Handlers                                 │ │
│  │  - Serve HTML pages (templ components)            │ │
│  │  - Return HTML fragments (htmx responses)         │ │
│  │  - OIDC authentication middleware                 │ │
│  └────────────────┬───────────────────────────────────┘ │
│                   │ gRPC                                 │
└───────────────────┼──────────────────────────────────────┘
                    │
┌───────────────────▼──────────────────────────────────────┐
│  Prism Proxy Admin API (gRPC) (:8981)                   │
│  - prism.admin.v1.AdminService                          │
│  - Namespace, Session, Backend, Operational APIs        │
└─────────────────────────────────────────────────────────┘
\`\`\`

This RFC is structured as both a proposal (Sections 1-5) and implementation guide (Appendix).

## Technology Stack and Rationale

### 1. templ - Type-Safe HTML Templates

[templ](https://templ.guide) provides compile-time type-safe HTML templating in Go.

**Example - Namespace Card Component:**

\`\`\`go
// templates/namespace.templ
package templates

import "prism/proto/admin/v1"

templ NamespaceCard(ns *adminv1.Namespace) {
  <div class="card p-4 bg-white rounded shadow" id={"namespace-" + ns.Name}>
    <h3 class="text-lg font-semibold">{ns.Name}</h3>
    <p class="text-sm text-gray-600">{ns.Description}</p>
    <button
      hx-delete={"/admin/namespaces/" + ns.Name}
      hx-target={"#namespace-" + ns.Name}
      hx-swap="outerHTML swap:1s"
      hx-confirm="Delete namespace?"
      class="btn-danger mt-2">
      Delete
    </button>
  </div>
}
\`\`\`

**Key Benefits:**
- **Compile-Time Safety**: Typos in field names caught at build time, not runtime
- **IDE Support**: Full autocomplete for Go struct fields
- **Automatic Escaping**: XSS protection by default
- **Component Composition**: Reusable components with type-safe props

**Rationale**: templ provides the type safety and developer experience of React components, but server-side. Unlike `html/template` (Go stdlib), templ validates templates at compile time, preventing runtime errors.

### 2. htmx - HTML Over the Wire

[htmx](https://htmx.org) enables declarative AJAX without JavaScript:

\`\`\`html
<!-- Search with live filtering -->
<input type="search"
       hx-get="/admin/namespaces/search"
       hx-trigger="keyup changed delay:300ms"
       hx-target="#namespace-list"/>

<!-- Delete with confirmation -->
<button hx-delete="/admin/namespaces/analytics"
        hx-target="#namespace-analytics"
        hx-swap="outerHTML"
        hx-confirm="Delete this namespace?">
  Delete
</button>
\`\`\`

**Key Benefits:**
- **No JavaScript Required**: Declarative attributes handle AJAX
- **Progressive Enhancement**: Works without htmx (graceful degradation)
- **Small Footprint**: 14KB gzipped (vs 100KB+ for frameworks)
- **Server-Authoritative**: UI state lives on server, not client

**Rationale**: htmx eliminates the need for a JavaScript framework while providing modern UX patterns. Admin UI requirements (CRUD, forms, filtering) are perfectly suited to htmx's capabilities.

### 3. Gin - Go HTTP Framework

[Gin](https://gin-gonic.com/) provides routing, middleware, and HTTP utilities:

\`\`\`go
func ListNamespaces(c *gin.Context) {
    client := getAdminClient(c)
    resp, err := client.ListNamespaces(c.Request.Context(), &adminv1.ListNamespacesRequest{})
    if err != nil {
        renderError(c, err)
        return
    }

    // Return full page or fragment based on HX-Request header
    if c.GetHeader("HX-Request") == "true" {
        templates.NamespaceList(resp.Namespaces).Render(c.Request.Context(), c.Writer)
    } else {
        templates.NamespacePage(resp.Namespaces).Render(c.Request.Context(), c.Writer)
    }
}
\`\`\`

**Rationale**: Gin is mature, performant, and widely used in the Go ecosystem. Its middleware system integrates well with OIDC auth and htmx detection.

## Comparison with ADR-028

| Aspect | templ+htmx+Gin (Proposed) | FastAPI+gRPC-Web (ADR-028) |
|--------|---------------------------|----------------------------|
| **Language** | Go only | Python + JavaScript |
| **Type Safety** | Full (compile-time) | Partial (runtime validation) |
| **Build Step** | `templ generate` | None (but slower dev iteration) |
| **Dependencies** | Go binary | Python + uvicorn + grpcio |
| **Container Size** | 15-20MB (scratch+binary) | 100-150MB (python:3.11-slim) |
| **Startup Time** | <50ms | 1-2 seconds |
| **Memory Usage** | 20-30MB | 50-100MB |
| **Consistency** | Matches Go ecosystem | Separate Python stack |
| **Admin API Access** | Native gRPC | gRPC-Web (protocol translation) |
| **Development UX** | `templ watch` + `air` | `uvicorn --reload` |
| **Testing** | Standard Go testing | Python pytest |

**Key Advantages:**
1. **85-87% smaller container** (15-20MB vs 100-150MB)
2. **20-40x faster startup** (<50ms vs 1-2s)
3. **Language consolidation** (Go for all admin tooling)
4. **Type safety** (compile-time validation vs runtime)
5. **Direct gRPC access** (no protocol translation overhead)

## Project Structure

\`\`\`text
cmd/prism-admin-ui/
├── main.go              # Entry point, Gin setup
├── handlers/
│   ├── namespace.go     # Namespace CRUD
│   ├── session.go       # Session monitoring
│   ├── health.go        # Backend health
│   └── auth.go          # OIDC login/logout
├── templates/
│   ├── layout.templ     # Base layout with nav
│   ├── namespace.templ  # Namespace components
│   ├── session.templ    # Session components
│   └── health.templ     # Health components
├── static/
│   ├── css/styles.css   # Tailwind CSS
│   └── js/htmx.min.js   # htmx library (14KB)
└── middleware/
    ├── auth.go          # OIDC token validation
    ├── htmx.go          # HX-Request detection
    └── logging.go       # Request logging
\`\`\`

## Authentication Integration

Reuse OIDC infrastructure from prismctl (RFC-010):

\`\`\`go
// middleware/auth.go
func OIDCAuth(validator *auth.JwtValidator) gin.HandlerFunc {
    return func(c *gin.Context) {
        sessionToken, err := c.Cookie("prism_session")
        if err == nil && sessionToken != "" {
            claims, err := validator.ValidateToken(sessionToken)
            if err == nil {
                c.Set("claims", claims)
                c.Next()
                return
            }
        }

        c.Redirect(http.StatusFound, "/admin/login")
        c.Abort()
    }
}
\`\`\`

**Benefits:**
- Shared JWT validation logic with prismctl
- Consistent OIDC configuration
- No duplicate authentication code

## Security Considerations

### XSS Protection

templ automatically escapes all variables:

\`\`\`go
templ UserInput(input string) {
  <p>{input}</p>  // Automatically escaped
}

// input = "<script>alert('xss')</script>"
// Renders: &lt;script&gt;alert('xss')&lt;/script&gt;
\`\`\`

**Manual override** (use sparingly):

\`\`\`go
templ TrustedHTML(html string) {
  <div>{templ.Raw(html)}</div>  // Explicit opt-in
}
\`\`\`

### CSRF Protection

Use Gin middleware:

\`\`\`go
import "github.com/utrack/gin-csrf"

r.Use(csrf.Middleware(csrf.Options{
    Secret: os.Getenv("CSRF_SECRET"),
}))
\`\`\`

## Deployment

### Standalone Service

\`\`\`yaml
# docker-compose.yml
services:
  prism-proxy:
    image: prism/proxy:latest
    ports:
      - "8980:8980"  # Data plane
      - "8981:8981"  # Admin API

  prism-admin-ui:
    image: prism/admin-ui:latest
    ports:
      - "8000:8000"
    environment:
      PRISM_ADMIN_ENDPOINT: prism-proxy:8981
      OIDC_ISSUER: https://idp.example.com
      OIDC_AUDIENCE: prism-admin-ui
\`\`\`

### Dockerfile

\`\`\`dockerfile
FROM golang:1.21 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go install github.com/a-h/templ/cmd/templ@latest
RUN templ generate
RUN CGO_ENABLED=0 go build -o prism-admin-ui ./cmd/prism-admin-ui

FROM scratch
COPY --from=builder /app/prism-admin-ui /prism-admin-ui
COPY --from=builder /app/cmd/prism-admin-ui/static /static
EXPOSE 8000
ENTRYPOINT ["/prism-admin-ui"]
\`\`\`

**Result**: 15-20MB image

## Migration Path from ADR-028

If FastAPI admin UI already exists:

### Phase 1: Parallel Deployment (Week 1-2)
- Deploy templ+htmx UI on port 8001
- Keep FastAPI UI on port 8000
- A/B test both versions

### Phase 2: Feature Parity (Week 2-4)
- Implement all FastAPI features in templ+htmx
- Migrate users incrementally

### Phase 3: Sunset FastAPI (Week 4-6)
- Switch default to templ+htmx (port 8000)
- Deprecate FastAPI UI

### Phase 4: Optimization (Week 6-8)
- Bundle admin UI into proxy binary (optional)
- Server-side caching
- Template rendering optimization

## Testing Strategy

### Unit Tests

\`\`\`go
func TestNamespaceCard(t *testing.T) {
    ns := &adminv1.Namespace{
        Name:        "test-namespace",
        Description: "Test description",
    }

    var buf bytes.Buffer
    err := templates.NamespaceCard(ns).Render(context.Background(), &buf)
    require.NoError(t, err)

    html := buf.String()
    assert.Contains(t, html, "test-namespace")
    assert.Contains(t, html, `id="namespace-test-namespace"`)
}
\`\`\`

### Integration Tests

\`\`\`go
func TestNamespaceCRUD(t *testing.T) {
    mockAdmin := startMockAdminAPI(t)
    defer mockAdmin.Close()

    adminUI := startAdminUI(t, mockAdmin.Address())
    defer adminUI.Close()

    resp, err := http.PostForm(adminUI.URL()+"/admin/namespaces", url.Values{
        "name":        {"test-ns"},
        "description": {"Test"},
    })
    require.NoError(t, err)
    assert.Equal(t, http.StatusOK, resp.StatusCode)
}
\`\`\`

## Alternatives Considered

### Alternative 1: Keep FastAPI (ADR-028)

**Pros**: Already designed, familiar to Python developers

**Cons**: Language fragmentation, larger footprint (100-150MB vs 15-20MB), separate maintenance

**Decision**: Propose templ+htmx for Go consolidation

### Alternative 2: React/Vue SPA

**Pros**: Rich interactions, large ecosystem

**Cons**: Build complexity, large bundle, overkill for CRUD

**Decision**: Rejected - admin UI doesn't need SPA complexity

### Alternative 3: html/template (Go stdlib)

**Pros**: No dependencies, standard library

**Cons**: No type safety, no compile-time validation

**Decision**: Rejected - templ's type safety is critical

## Open Questions

1. **Embedded vs Standalone**: Embed in proxy or separate service?
   - **Recommendation**: Start standalone, consider embedding in Phase 4

2. **CSS Framework**: Tailwind (utility-first) or custom CSS?
   - **Recommendation**: Tailwind for rapid development

3. **Real-time Updates**: WebSocket for live session monitoring?
   - **Recommendation**: Start with htmx polling, add WebSocket if needed

## Implementation Roadmap

If this RFC is accepted:

### Week 1-2: Foundation
- Set up `cmd/prism-admin-ui` directory structure
- Implement base layout and navigation (templ)
- Add OIDC authentication middleware
- Deploy parallel to FastAPI UI (port 8001)

### Week 3-4: Core Features
- Namespace CRUD (full feature parity with FastAPI)
- Session monitoring dashboard
- Backend health checks
- User testing and feedback

### Week 5-6: Polish and Migration
- Address feedback from user testing
- Performance optimization
- Switch default to templ+htmx (port 8000)
- Deprecate FastAPI UI

### Week 7-8: Production Readiness
- Security audit
- Load testing
- Documentation
- Optional: Embed in proxy binary

## References

- [templ Documentation](https://templ.guide)
- [htmx Documentation](https://htmx.org)
- [Gin Web Framework](https://gin-gonic.com)
- ADR-028: Admin UI with FastAPI and gRPC-Web
- ADR-040: Go Binary for Admin CLI (prismctl)
- RFC-003: Admin Interface for Prism
- RFC-010: Admin Protocol with OIDC Authentication

## Appendix: Implementation Guide

This appendix serves as a practical reference for implementing templ+htmx patterns in Prism Admin UI.

### Common Patterns

#### Pattern 1: Full Page Render

**When:** Initial page load, navigation

\`\`\`go
templ NamespacePage(namespaces []*adminv1.Namespace) {
  <!DOCTYPE html>
  <html>
    <head>
      <script src="/static/js/htmx.min.js"></script>
      <link rel="stylesheet" href="/static/css/styles.css"/>
    </head>
    <body>
      @NamespaceList(namespaces)
    </body>
  </html>
}
\`\`\`

#### Pattern 2: Partial/Fragment Render

**When:** htmx requests

\`\`\`go
func ListNamespaces(c *gin.Context) {
  namespaces := getNamespaces()

  if c.GetHeader("HX-Request") == "true" {
    templates.NamespaceList(namespaces).Render(c.Request.Context(), c.Writer)
  } else {
    templates.NamespacePage(namespaces).Render(c.Request.Context(), c.Writer)
  }
}
\`\`\`

#### Pattern 3: Forms

**When:** Create/Update operations

\`\`\`go
templ NamespaceForm(ns *adminv1.Namespace) {
  <form hx-post="/admin/namespaces"
        hx-target="#namespace-list"
        hx-swap="afterbegin">
    <input name="name" value={ns.Name} required/>
    <button type="submit">Create</button>
  </form>
}
\`\`\`

#### Pattern 4: Search/Filter

**When:** Real-time filtering

\`\`\`go
templ SearchBox() {
  <input type="search"
         hx-get="/admin/namespaces/search"
         hx-trigger="keyup changed delay:300ms"
         hx-target="#namespace-list"/>
}
\`\`\`

#### Pattern 5: Optimistic Updates

**When:** Better UX for slow operations

\`\`\`html
<button hx-delete="/admin/namespaces/analytics"
        hx-target="#namespace-analytics"
        hx-swap="outerHTML swap:1s"
        hx-indicator="#spinner">
  Delete
</button>
<div id="spinner" class="htmx-indicator">Deleting...</div>
\`\`\`

### htmx Attribute Reference

\`\`\`text
Core:
  hx-get="/url"              GET request
  hx-post="/url"             POST request
  hx-delete="/url"           DELETE request

Targeting:
  hx-target="#id"            Where to put response
  hx-swap="innerHTML"        How to swap

Triggers:
  hx-trigger="click"         When to fire
  hx-trigger="keyup changed delay:300ms"  Debounced

State:
  hx-indicator="#spinner"    Show during request
  hx-confirm="Are you sure?" Confirm before request
\`\`\`

### Best Practices

1. **Component Composition**: Break templates into small, reusable components
2. **Type Safety**: Use Go structs, let templ validate at compile time
3. **Predictable IDs**: Use consistent naming (`id={"namespace-" + ns.Name}`)
4. **Loading States**: Always provide `hx-indicator` feedback
5. **Error Handling**: Return proper HTTP status codes with error templates

### Common Gotchas

1. **Templates Not Regenerating**: Run `templ generate --watch` during development
2. **URL Encoding**: Use `templ.URL()` for URLs with query params
3. **XSS Protection**: templ escapes by default; use `templ.Raw()` sparingly
4. **CSRF Tokens**: Add middleware and include tokens in all forms
5. **Browser Caching**: Disable cache for htmx requests (`Cache-Control: no-store`)

## Revision History

- 2025-10-15: Initial RFC proposing templ+htmx+Gin as alternative to ADR-028
