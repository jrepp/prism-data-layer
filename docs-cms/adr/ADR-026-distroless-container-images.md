---
title: "ADR-026: Distroless Base Images for Container Components"
status: Accepted
date: 2025-10-07
deciders: Core Team
tags: ['containers', 'security', 'deployment', 'docker']
---

## Context

Prism deploys multiple container components:
- Proxy core (Rust)
- Backend plugins (Rust, potentially Go)
- Tooling utilities

Container base images impact:
- **Security**: Attack surface from included packages
- **Image size**: Download time, storage, cost
- **Vulnerabilities**: CVEs in OS packages
- **Debugging**: Available tools for troubleshooting

**Requirements:**
- Minimal attack surface
- Small image size
- Fast build and deployment
- Security scanning compliance
- Sufficient tools for debugging when needed

## Decision

Use **Google Distroless base images** for all Prism container components:

1. **Production images**: Distroless (minimal, no shell, no package manager)
2. **Debug variant**: Distroless debug (includes busybox for troubleshooting)
3. **Multi-stage builds**: Build in full image, run in distroless
4. **Static binaries**: Compile to static linking where possible
5. **Runtime dependencies only**: Only include what's needed to run

## Rationale

### Why Distroless

**Security benefits:**
- No shell (prevents shell-based attacks)
- No package manager (can't install malware)
- Minimal packages (reduced CVE exposure)
- Small attack surface (fewer binaries to exploit)

**Image size:**
- Base image: ~20MB (vs. debian:slim ~80MB, ubuntu:22.04 ~77MB)
- Final images: 30-50MB (application + distroless)
- Faster pulls, lower bandwidth, less storage

**Vulnerability scanning:**
- Fewer packages = fewer CVEs
- Google maintains and patches base images
- Easier compliance with security policies

### Distroless Variants

**Available variants:**

1. **`static-debian12`**: Static binaries (Go, Rust static)
   - Size: ~2MB
   - Contains: CA certs, tzdata, /etc/passwd
   - No libc

2. **`cc-debian12`**: C runtime (Rust dynamic)
   - Size: ~20MB
   - Contains: glibc, libssl, CA certs
   - For dynamically-linked binaries

3. **`static-debian12:debug`**: Static + busybox
   - Size: ~5MB
   - Includes: sh, cat, ls, netstat
   - For debugging

4. **`cc-debian12:debug`**: CC + busybox
   - Size: ~22MB
   - For debugging dynamically-linked apps

### Rust Applications

Most Prism components are Rust:

```dockerfile
# Build stage - full Rust environment
FROM rust:1.75 as builder

WORKDIR /app
COPY . .

# Build with static linking where possible
RUN cargo build --release --bin prism-proxy

# Runtime stage - distroless
FROM gcr.io/distroless/cc-debian12:nonroot

COPY --from=builder /app/target/release/prism-proxy /usr/local/bin/

# Non-root user (UID 65532)
USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/prism-proxy"]
```

**Why `cc-debian12` for Rust:**
- Most Rust crates link dynamically to system libs (OpenSSL, etc.)
- Fully static build requires `musl` target (more complex)
- `cc-debian12` provides glibc and common C libraries

### Go Applications (Tooling)

Go tooling can use fully static images:

```dockerfile
# Build stage
FROM golang:1.22 as builder

WORKDIR /app
COPY . .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o prism-cli cmd/prism-cli/main.go

# Runtime stage - fully static
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /app/prism-cli /usr/local/bin/

USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/prism-cli"]
```

**Why `static-debian12` for Go:**
- Go easily builds fully static binaries with `CGO_ENABLED=0`
- No C dependencies needed
- Smallest possible image

### Debug Images

For troubleshooting, build debug variant:

```dockerfile
# Runtime stage - distroless debug
FROM gcr.io/distroless/cc-debian12:debug-nonroot

COPY --from=builder /app/target/release/prism-proxy /usr/local/bin/

USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/prism-proxy"]
```

**Access debug shell:**
```bash
# Override entrypoint to get shell
docker run -it --entrypoint /busybox/sh prism/proxy:debug

# Or in Kubernetes
kubectl exec -it prism-proxy-pod -- /busybox/sh
```

**Debug tools available:**
- `sh` (shell)
- `ls`, `cat`, `grep`, `ps`
- `netstat`, `ping`, `wget`
- `vi` (basic editor)

### Example: Complete Multi-Stage Build

```dockerfile
# Dockerfile.proxy
# Build stage - full Rust toolchain
FROM rust:1.75 as builder

WORKDIR /app

# Copy dependency manifests first (cache layer)
COPY Cargo.toml Cargo.lock ./
COPY proxy/Cargo.toml proxy/
RUN mkdir proxy/src && echo "fn main() {}" > proxy/src/main.rs
RUN cargo build --release
RUN rm -rf proxy/src

# Copy source and build
COPY proxy/src proxy/src
RUN cargo build --release --bin prism-proxy

# Production runtime - distroless cc (for glibc/openssl)
FROM gcr.io/distroless/cc-debian12:nonroot as production

COPY --from=builder /app/target/release/prism-proxy /usr/local/bin/prism-proxy

# Use non-root user
USER nonroot:nonroot

# Health check metadata (not executed by distroless)
EXPOSE 8980 9090

ENTRYPOINT ["/usr/local/bin/prism-proxy"]

# Debug runtime - includes busybox
FROM gcr.io/distroless/cc-debian12:debug-nonroot as debug

COPY --from=builder /app/target/release/prism-proxy /usr/local/bin/prism-proxy

USER nonroot:nonroot

EXPOSE 8980 9090

ENTRYPOINT ["/usr/local/bin/prism-proxy"]
```

**Build both variants:**
```bash
# Production
docker build --target production -t prism/proxy:latest .

# Debug
docker build --target debug -t prism/proxy:debug .
```

### Plugin Containers

Each plugin follows same pattern:

```dockerfile
# Dockerfile.kafka-publisher
FROM rust:1.75 as builder

WORKDIR /app
COPY . .
RUN cargo build --release --bin kafka-publisher

FROM gcr.io/distroless/cc-debian12:nonroot

COPY --from=builder /app/target/release/kafka-publisher /usr/local/bin/

USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/kafka-publisher"]
```

### Security Hardening

**Non-root user:**
- Distroless images include `nonroot` user (UID 65532)
- Never run as root

**Read-only filesystem:**
```yaml
# Kubernetes pod spec
securityContext:
  runAsNonRoot: true
  runAsUser: 65532
  readOnlyRootFilesystem: true
  allowPrivilegeEscalation: false
  capabilities:
    drop: ["ALL"]
```

**No shell or package manager:**
- Prevents remote code execution via shell
- Can't install malware or backdoors

### CI/CD Integration

**Build pipeline:**
```yaml
# .github/workflows/docker-build.yml
- name: Build production image
  run: |
    docker build --target production \
      -t ghcr.io/prism/proxy:${{ github.sha }} \
      -t ghcr.io/prism/proxy:latest \
      .

- name: Build debug image
  run: |
    docker build --target debug \
      -t ghcr.io/prism/proxy:${{ github.sha }}-debug \
      -t ghcr.io/prism/proxy:debug \
      .

- name: Scan images
  run: |
    trivy image ghcr.io/prism/proxy:latest
```

### Alternatives Considered

1. **Alpine Linux**
   - Pros: Small (~5MB base), familiar, has package manager
   - Cons: musl libc (compatibility issues), still has shell/packages
   - Rejected: More attack surface than distroless

2. **Debian Slim**
   - Pros: Familiar, good docs, standard glibc
   - Cons: Large (~80MB), includes shell, package manager, many CVEs
   - Rejected: Too large, unnecessary packages

3. **Ubuntu**
   - Pros: Very familiar, enterprise support available
   - Cons: Large (77MB+), many packages, high CVE count
   - Rejected: Too large for minimal services

4. **Scratch (empty)**
   - Pros: Absolutely minimal (0 bytes)
   - Cons: No CA certs, no timezone data, hard to debug
   - Rejected: Too minimal, missing essential files

5. **Chainguard Images**
   - Pros: Similar to distroless, daily rebuilds, minimal CVEs
   - Cons: Requires subscription for some images
   - Deferred: Evaluate later if Google distroless insufficient

## Consequences

### Positive

- **Minimal attack surface**: No shell, no package manager
- **Small images**: 30-50MB vs 200-300MB with full OS
- **Fewer CVEs**: Minimal packages mean fewer vulnerabilities
- **Fast deployments**: Smaller images pull faster
- **Security compliance**: Easier to pass security audits
- **Industry standard**: Google's recommended practice

### Negative

- **No debugging in production**: Can't SSH and install tools
- **Must use debug variant**: Need separate image for troubleshooting
- **Learning curve**: Different from traditional Docker images
- **Static linking complexity**: Some Rust crates harder to statically link

### Neutral

- **Build time**: Multi-stage builds add complexity but cache well
- **Observability**: Must rely on external logging/metrics (good practice anyway)

## Implementation Notes

### Image Naming Convention

prism/proxy:latest              # Production
prism/proxy:v1.2.3              # Specific version (production)
prism/proxy:debug               # Debug variant (latest)
prism/proxy:v1.2.3-debug        # Debug variant (specific version)
```text

### File Structure

prism/
├── proxy/
│   ├── Dockerfile              # Proxy image (multi-stage)
│   └── src/
├── containers/
│   ├── kafka-publisher/
│   │   ├── Dockerfile
│   │   └── src/
│   ├── kafka-consumer/
│   │   ├── Dockerfile
│   │   └── src/
│   └── mailbox-listener/
│       ├── Dockerfile
│       └── src/
└── tools/
    └── cmd/
        ├── prism-cli/
        │   └── Dockerfile
        └── prism-migrate/
            └── Dockerfile
```

### Required Files in Image

**Always include:**
- Application binary
- CA certificates (for TLS)
- Timezone data (if using timestamps)

**Distroless provides:**
- `/etc/passwd` (nonroot user)
- `/etc/ssl/certs/ca-certificates.crt`
- `/usr/share/zoneinfo/`

**Never include:**
- Config files (use environment variables)
- Secrets (inject at runtime)
- Temporary files

### Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prism-proxy
spec:
  template:
    spec:
      containers:
      - name: proxy
        image: prism/proxy:latest
        securityContext:
          runAsNonRoot: true
          runAsUser: 65532
          readOnlyRootFilesystem: true
          allowPrivilegeEscalation: false
          capabilities:
            drop: ["ALL"]
        env:
        - name: RUST_LOG
          value: info
        ports:
        - containerPort: 8980
          name: grpc
        - containerPort: 9090
          name: metrics
```

### Debugging Workflow

1. **Production issue occurs**
2. **Deploy debug image** to separate environment or pod
3. **Reproduce issue** with debug image
4. **Access shell**: `kubectl exec -it pod -- /busybox/sh`
5. **Investigate**: Use busybox tools to diagnose
6. **Fix and redeploy** production image

**Never deploy debug image to production**

## References

- [Distroless GitHub](https://github.com/GoogleContainerTools/distroless)
- [Distroless Best Practices](https://github.com/GoogleContainerTools/distroless#distroless-container-images)
- [Docker Multi-Stage Builds](https://docs.docker.com/build/building/multi-stage/)
- ADR-025: Container Plugin Model
- ADR-008: Observability Strategy

## Revision History

- 2025-10-07: Initial draft and acceptance
