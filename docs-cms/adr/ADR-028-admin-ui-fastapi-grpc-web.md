---
title: "ADR-028: Admin UI with FastAPI and gRPC-Web"
status: Accepted
date: 2025-10-07
deciders: Core Team
tags: ['admin', 'ui', 'fastapi', 'grpc-web', 'frontend']
---

## Context

Prism Admin API (ADR-027) provides gRPC endpoints for administration. Need web-based UI for:
- Managing client configurations
- Monitoring active sessions
- Viewing backend health
- Namespace management
- Operational tasks

**Requirements:**
- Browser-accessible admin interface
- Communicate with gRPC backend
- Lightweight deployment
- Modern, responsive UI
- Production-grade security

## Decision

Build **Admin UI with FastAPI + gRPC-Web**:

1. **FastAPI backend**: Python service serving static files and gRPC-Web proxy
2. **gRPC-Web**: Protocol translation from browser to gRPC backend
3. **Vanilla JavaScript**: Simple, no-framework frontend
4. **CSS**: Tailwind or modern CSS for styling
5. **Single container**: All-in-one deployment

## Rationale

### Architecture

┌─────────────────────────────────────────────┐
│              Browser                        │
│                                             │
│  ┌────────────────────────────────────┐    │
│  │  Admin UI (HTML/CSS/JS)            │    │
│  │  - Configuration manager           │    │
│  │  - Session monitor                 │    │
│  │  - Health dashboard                │    │
│  └────────────┬───────────────────────┘    │
│               │ HTTP + gRPC-Web             │
└───────────────┼─────────────────────────────┘
                │
┌───────────────▼─────────────────────────────┐
│  FastAPI Service (:8000)                    │
│                                             │
│  ┌─────────────────────────────────────┐   │
│  │  Static File Server                 │   │
│  │  GET /  → index.html                │   │
│  │  GET /static/* → CSS/JS             │   │
│  └─────────────────────────────────────┘   │
│                                             │
│  ┌─────────────────────────────────────┐   │
│  │  gRPC-Web Proxy                     │   │
│  │  POST /prism.admin.v1.AdminService  │   │
│  └──────────────┬──────────────────────┘   │
└─────────────────┼───────────────────────────┘
                  │ gRPC
┌─────────────────▼───────────────────────────┐
│  Prism Admin API (:8981)                    │
│  - prism.admin.v1.AdminService              │
└─────────────────────────────────────────────┘
```text

### Why FastAPI

**Pros:**
- Modern Python web framework
- Async support (perfect for gRPC proxy)
- Built-in OpenAPI/Swagger docs
- Easy static file serving
- Production-ready with Uvicorn

**Cons:**
- Python dependency (but we already use Python for tooling)

### Why gRPC-Web

**Browser limitation**: Browsers can't speak native gRPC (no HTTP/2 trailers support)

**gRPC-Web solution:**
- HTTP/1.1 or HTTP/2 compatible
- Protobuf encoding preserved
- Generated JavaScript clients
- Transparent proxy to gRPC backend

### Frontend Stack

**Vanilla JavaScript** (no framework):
- **Pros**: No build step, no dependencies, fast load, simple
- **Cons**: Manual DOM manipulation, no reactivity

**Modern CSS** (Tailwind or custom):
- **Pros**: Responsive, modern look, utility-first
- **Cons**: Larger CSS file (but can be minified)

**Generated gRPC-Web client:**
```
# Generate JavaScript client from proto
protoc --js_out=import_style=commonjs,binary:./admin-ui/static/js \
       --grpc-web_out=import_style=commonjs,mode=grpcwebtext:./admin-ui/static/js \
       proto/prism/admin/v1/admin.proto
```text

### FastAPI Implementation

```
# admin-ui/main.py
from fastapi import FastAPI
from fastapi.staticfiles import StaticFiles
from fastapi.responses import FileResponse
import grpc
from grpc_web import grpc_web_server

app = FastAPI(title="Prism Admin UI")

# Serve static files
app.mount("/static", StaticFiles(directory="static"), name="static")

# Serve index.html for root
@app.get("/")
async def read_root():
    return FileResponse("static/index.html")

# gRPC-Web proxy
@app.post("/prism.admin.v1.AdminService/{method}")
async def grpc_proxy(method: str, request: bytes):
    """Proxy gRPC-Web requests to gRPC backend"""
    channel = grpc.aio.insecure_channel("prism-proxy:8981")
    # Forward request to gRPC backend
    # Handle response and convert to gRPC-Web format
    pass

# Health check
@app.get("/health")
async def health():
    return {"status": "healthy"}
```text

### Frontend Structure

admin-ui/
├── main.py                 # FastAPI app
├── requirements.txt        # Python deps
├── Dockerfile             # Container image
└── static/
    ├── index.html         # Main page
    ├── css/
    │   └── styles.css     # Tailwind or custom CSS
    ├── js/
    │   ├── admin_grpc_web_pb.js  # Generated gRPC-Web client
    │   ├── config.js      # Config management
    │   ├── sessions.js    # Session monitoring
    │   └── health.js      # Health dashboard
    └── lib/
        └── grpc-web.js    # gRPC-Web runtime
```

### HTML Template

```html
<!-- admin-ui/static/index.html -->
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Prism Admin</title>
    <link rel="stylesheet" href="/static/css/styles.css">
</head>
<body class="bg-gray-100">
    <div class="container mx-auto px-4 py-8">
        <header class="mb-8">
            <h1 class="text-3xl font-bold">Prism Admin</h1>
            <nav class="mt-4">
                <button data-page="configs" class="px-4 py-2 bg-blue-500 text-white rounded">Configs</button>
                <button data-page="sessions" class="px-4 py-2 bg-blue-500 text-white rounded">Sessions</button>
                <button data-page="health" class="px-4 py-2 bg-blue-500 text-white rounded">Health</button>
            </nav>
        </header>

        <main id="content">
            <!-- Dynamic content loaded here -->
        </main>
    </div>

    <script type="module" src="/static/js/admin_grpc_web_pb.js"></script>
    <script type="module" src="/static/js/config.js"></script>
    <script type="module" src="/static/js/sessions.js"></script>
    <script type="module" src="/static/js/health.js"></script>
</body>
</html>
```

### JavaScript gRPC-Web Client

```javascript
// admin-ui/static/js/config.js
import {AdminServiceClient} from './admin_grpc_web_pb.js';
import {ListConfigsRequest} from './admin_grpc_web_pb.js';

const client = new AdminServiceClient('http://localhost:8000', null, null);

async function loadConfigs() {
    const request = new ListConfigsRequest();

    client.listConfigs(request, {'x-admin-token': getAdminToken()}, (err, response) => {
        if (err) {
            console.error('Error loading configs:', err);
            return;
        }

        const configs = response.getConfigsList();
        renderConfigs(configs);
    });
}

function renderConfigs(configs) {
    const html = configs.map(config => `
        <div class="bg-white p-4 rounded shadow mb-4">
            <h3 class="font-bold">${config.getName()}</h3>
            <p class="text-sm text-gray-600">Pattern: ${config.getPattern()}</p>
            <p class="text-sm text-gray-600">Backend: ${config.getBackend().getType()}</p>
        </div>
    `).join('');

    document.getElementById('content').innerHTML = html;
}

// Export functions
export {loadConfigs};
```

### Deployment

```dockerfile
# admin-ui/Dockerfile
FROM python:3.11-slim

WORKDIR /app

# Install dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy application
COPY main.py .
COPY static/ static/

# Expose port
EXPOSE 8000

# Run FastAPI with Uvicorn
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]
```

```yaml
# docker-compose.yml
services:
  prism-proxy:
    image: prism/proxy:latest
    ports:
      - "8980:8980"  # Data plane
      - "8981:8981"  # Admin API

  admin-ui:
    image: prism/admin-ui:latest
    ports:
      - "8000:8000"
    environment:
      - PRISM_ADMIN_ENDPOINT=prism-proxy:8981
      - ADMIN_TOKEN_SECRET=your-secret-key
    depends_on:
      - prism-proxy
```

### Security

**Authentication:**
```python
from fastapi import Header, HTTPException

async def verify_admin_token(x_admin_token: str = Header(...)):
    if not is_valid_admin_token(x_admin_token):
        raise HTTPException(status_code=401, detail="Invalid admin token")
    return x_admin_token

@app.post("/prism.admin.v1.AdminService/{method}")
async def grpc_proxy(
    method: str,
    request: bytes,
    admin_token: str = Depends(verify_admin_token)
):
    # Forward with admin token
    metadata = [('x-admin-token', admin_token)]
    # ... proxy request
```

**CORS** (if needed):
```python
from fastapi.middleware.cors import CORSMiddleware

app.add_middleware(
    CORSMiddleware,
    allow_origins=["https://" + "admin.example.com"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)
```

### Alternative: Envoy gRPC-Web Proxy

Instead of FastAPI, use Envoy:

```yaml
# envoy.yaml
static_resources:
  listeners:
  - name: listener_0
    address:
      socket_address:
        address: 0.0.0.0
        port_value: 8000
    filter_chains:
    - filters:
      - name: envoy.filters.network.http_connection_manager
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
          codec_type: AUTO
          stat_prefix: ingress_http
          route_config:
            name: local_route
            virtual_hosts:
            - name: backend
              domains: ["*"]
              routes:
              - match:
                  prefix: "/prism.admin.v1"
                route:
                  cluster: grpc_backend
          http_filters:
          - name: envoy.filters.http.grpc_web
          - name: envoy.filters.http.cors
          - name: envoy.filters.http.router

  clusters:
  - name: grpc_backend
    connect_timeout: 0.25s
    type: LOGICAL_DNS
    lb_policy: ROUND_ROBIN
    typed_extension_protocol_options:
      envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
        "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions
        explicit_http_config:
          http2_protocol_options: {}
    load_assignment:
      cluster_name: grpc_backend
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: prism-proxy
                port_value: 8981
```

**Pros:** Production-grade, feature-rich
**Cons:** More complex, separate process

### Alternatives Considered

1. **React/Vue/Angular SPA**
   - Pros: Rich UI, reactive, component-based
   - Cons: Build step, bundle size, complexity
   - Rejected: Vanilla JS sufficient for admin UI

2. **Server-side rendering (Jinja2)**
   - Pros: No JavaScript needed, SEO-friendly
   - Cons: Full page reloads, less interactive
   - Rejected: Admin UI needs interactivity

3. **Separate Ember.js app (as originally planned)**
   - Pros: Full-featured framework, ember-data
   - Cons: Large bundle, build complexity, overkill
   - Rejected: Too heavy for admin UI

4. **grpcurl-based CLI only**
   - Pros: Simple, no UI needed
   - Cons: Not user-friendly for non-technical admins
   - Rejected: Web UI provides better UX

## Consequences

### Positive

- **Simple deployment**: Single container with FastAPI
- **No build step**: Vanilla JS loads directly
- **gRPC compatible**: Uses gRPC-Web protocol
- **Lightweight**: Minimal dependencies
- **Fast development**: Python + simple JS

### Negative

- **Manual DOM updates**: No framework reactivity
- **Limited UI features**: Vanilla JS less powerful than frameworks
- **Python dependency**: Adds Python to stack (but already used for tooling)

### Neutral

- **gRPC-Web limitation**: Requires proxy (but handled by FastAPI)
- **Browser compatibility**: Modern browsers only (ES6+)

## Implementation Notes

### Development Workflow

```bash
# Generate gRPC-Web client
buf generate --template admin-ui/buf.gen.grpc-web.yaml

# Run FastAPI dev server
cd admin-ui
uvicorn main:app --reload --port 8000

# Open browser
open http://localhost:8000
```

### Production Build

```bash
# Minify CSS
npx tailwindcss -i static/css/styles.css -o static/css/styles.min.css --minify

# Minify JS (optional)
npx terser static/js/config.js -o static/js/config.min.js

# Build Docker image
docker build -t prism/admin-ui:latest ./admin-ui
```

### Requirements

```txt
# admin-ui/requirements.txt
fastapi==0.104.1
uvicorn[standard]==0.24.0
grpcio==1.59.0
grpcio-tools==1.59.0
```

## References

- ADR-027: Admin API via gRPC
- [gRPC-Web](https://github.com/grpc/grpc-web)
- [FastAPI](https://fastapi.tiangolo.com/)
- [Tailwind CSS](https://tailwindcss.com/)

## Revision History

- 2025-10-07: Initial draft and acceptance
