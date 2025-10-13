---
date: 2025-10-08
deciders: System
doc_uuid: a59c9c6c-a815-4a64-90a2-de2ce7e9362d
id: adr-037
project_id: prism-data-layer
status: Proposed
tags:
- operations
- deployment
- kubernetes
- automation
- dx
title: Kubernetes Operator with Custom Resource Definitions
---

## Context

Managing Prism deployments at scale requires automation for:
- **Namespace Lifecycle**: Creating, updating, deleting namespaces across multiple Prism instances
- **Shard Management**: Deploying product/feature-based shards (ADR-034)
- **Plugin Installation**: Distributing plugins across instances
- **Configuration Sync**: Keeping namespace configs consistent across replicas
- **Resource Management**: CPU/memory limits, autoscaling, health checks

### Manual Management Pain Points

Without automation:
- **YAML Hell**: Manually maintaining hundreds of namespace config files
- **Deployment Complexity**: kubectl apply across multiple files, error-prone
- **Inconsistency**: Config drift between Prism instances
- **No GitOps**: Can't declaratively manage Prism infrastructure as code
- **Slow Iteration**: Namespace changes require manual updates to multiple instances

### Kubernetes Operator Pattern

**Operators** extend Kubernetes with custom logic to manage applications:
- **CRDs** (Custom Resource Definitions): Define custom resources (e.g., `PrismNamespace`)
- **Controller**: Watches CRDs, reconciles desired state → actual state
- **Declarative**: Describe what you want, operator figures out how

**Examples**: PostgreSQL Operator, Kafka Operator, Istio Operator

## Decision

**Build a Prism Kubernetes Operator** that manages Prism deployments via Custom Resource Definitions (CRDs).

### Custom Resources

```yaml
# CRD 1: PrismNamespace
apiVersion: prism.io/v1alpha1
kind: PrismNamespace
metadata:
  name: user-profiles
spec:
  backend: postgres
  pattern: keyvalue
  consistency: strong
  backendConfig:
    connection_string: postgres://db:5432/profiles
    pool_size: 20
  caching:
    enabled: true
    ttl: 300s
  rateLimit:
    rps: 10000
  shard:  # Optional: assign to specific shard
    product: playback
    slaTier: p99_10ms

status:
  state: Active
  prismInstances:
    - prism-playback-0
    - prism-playback-1
  health: Healthy
  metrics:
    rps: 1234
    p99Latency: 8ms
```

```yaml
# CRD 2: PrismShard (from ADR-034)
apiVersion: prism.io/v1alpha1
kind: PrismShard
metadata:
  name: playback-live
spec:
  product: playback
  feature: live
  slaTier: p99_10ms
  replicas: 5
  backends:
    - postgres
    - redis
  resources:
    requests:
      cpu: "4"
      memory: "8Gi"
    limits:
      cpu: "8"
      memory: "16Gi"
  plugins:
    - name: postgres
      version: "1.2.0"
      deployment: sidecar
    - name: redis
      version: "2.1.3"
      deployment: in-process
  autoscaling:
    enabled: true
    minReplicas: 3
    maxReplicas: 20
    targetRPS: 5000

status:
  readyReplicas: 5
  namespaces: 12
  aggregateMetrics:
    totalRPS: 24567
    avgP99Latency: 9ms
```

```yaml
# CRD 3: PrismPlugin
apiVersion: prism.io/v1alpha1
kind: PrismPlugin
metadata:
  name: mongodb
spec:
  version: "1.0.0"
  source:
    registry: ghcr.io/prism/plugins
    image: mongodb-plugin:1.0.0
  deployment:
    type: sidecar  # or in-process, remote
    resources:
      requests:
        cpu: "500m"
        memory: "1Gi"
      limits:
        cpu: "2"
        memory: "4Gi"
  healthCheck:
    grpc:
      port: 50100
      service: prism.plugin.BackendPlugin
    interval: 30s
    timeout: 5s

status:
  installed: true
  shards:
    - playback-live
    - analytics-batch
  namespacesUsing: 15
```

### Operator Architecture

┌──────────────────────────────────────────────────────────┐
│                  Kubernetes Cluster                       │
│                                                           │
│  ┌─────────────────────────────────────────────────────┐ │
│  │             Prism Operator (Controller)             │ │
│  │                                                     │ │
│  │  Watches:                                           │ │
│  │  - PrismNamespace CRDs                              │ │
│  │  - PrismShard CRDs                                  │ │
│  │  - PrismPlugin CRDs                                 │ │
│  │                                                     │ │
│  │  Reconciles:                                        │ │
│  │  1. Creates/updates Prism Deployments              │ │
│  │  2. Provisions PVCs for SQLite config (ADR-036)    │ │
│  │  3. Deploys plugin sidecars                        │ │
│  │  4. Updates Services, Ingress                      │ │
│  │  5. Syncs namespace config to Prism instances       │ │
│  └─────────────────────────────────────────────────────┘ │
│           │                              │               │
│           ▼                              ▼               │
│  ┌──────────────────┐        ┌──────────────────┐       │
│  │  PrismShard:     │        │  PrismShard:     │       │
│  │  playback-live   │        │  analytics-batch │       │
│  │                  │        │                  │       │
│  │  ┌────────────┐  │        │  ┌────────────┐  │       │
│  │  │ Prism Pod  │  │        │  │ Prism Pod  │  │       │
│  │  │ replicas:5 │  │        │  │ replicas:3 │  │       │
│  │  └────────────┘  │        │  └────────────┘  │       │
│  │  ┌────────────┐  │        │  ┌────────────┐  │       │
│  │  │  Plugins   │  │        │  │  Plugins   │  │       │
│  │  │ (sidecars) │  │        │  │ (sidecars) │  │       │
│  │  └────────────┘  │        │  └────────────┘  │       │
│  └──────────────────┘        └──────────────────┘       │
└──────────────────────────────────────────────────────────┘
```text

### Reconciliation Logic

**When a PrismNamespace is created**:
1. Operator determines which shard should host it (based on shard selector)
2. Updates Prism instance's SQLite config database (ADR-036) via Admin API
3. Verifies namespace is active and healthy
4. Updates `PrismNamespace.status` with assigned shard and metrics

**When a PrismShard is created**:
1. Creates Deployment with specified replicas
2. Creates PersistentVolumeClaim for each replica (SQLite storage)
3. Creates Service (ClusterIP for internal, LoadBalancer if exposed)
4. Deploys plugin sidecars as specified
5. Initializes SQLite databases on each replica
6. Waits for all replicas to be ready

**When a PrismPlugin is updated**:
1. Pulls new plugin image
2. For each shard using the plugin:
   - Performs rolling update of plugin sidecars
   - Verifies health after each update
   - Rolls back on failure

## Rationale

### Why Custom Operator vs Raw Kubernetes?

**Without Operator** (raw Kubernetes manifests):
```
# Must manually define:
- Deployment for each shard
- StatefulSet for SQLite persistence
- Services for each shard
- ConfigMaps for namespace configs (must sync manually!)
- Plugin sidecar injection (manual, error-prone)
```text

**With Operator**:
```
# Just define:
apiVersion: prism.io/v1alpha1
kind: PrismNamespace
metadata:
  name: my-namespace
spec:
  backend: postgres
  pattern: keyvalue
# Operator handles the rest!
```text

### Compared to Alternatives

**vs Helm Charts**:
- ✅ Operator is dynamic (watches for changes, reconciles)
- ✅ Operator can query Prism API for current state
- ❌ Helm is static (install/upgrade only)
- **Use both**: Operator installed via Helm, then manages CRDs

**vs Manual kubectl**:
- ✅ Operator enforces best practices
- ✅ Operator handles complex workflows (rolling updates, health checks)
- ❌ kubectl requires manual orchestration
- **Operator wins** for production deployments

**vs External Tool (Ansible, Terraform)**:
- ✅ Operator is Kubernetes-native (no external dependencies)
- ✅ Operator continuously reconciles (self-healing)
- ❌ External tools are one-shot (no continuous reconciliation)
- **Operator preferred** for Kubernetes environments

## Alternatives Considered

### 1. Helm Charts Only

- **Pros**: Simpler, no custom code
- **Cons**: No dynamic reconciliation, can't query Prism state
- **Rejected because**: Doesn't scale operationally (manual config sync)

### 2. GitOps (ArgoCD/Flux) Without Operator

- **Pros**: Declarative, Git as source of truth
- **Cons**: Still need to manage low-level Kubernetes resources manually
- **Partially accepted**: Use GitOps + Operator (ArgoCD applies CRDs, operator reconciles)

### 3. Serverless Functions (AWS Lambda, CloudRun)

- **Pros**: No Kubernetes needed
- **Cons**: Stateful config management harder, no standard API
- **Rejected because**: Prism is Kubernetes-native, operator pattern is standard

## Consequences

### Positive

- **Declarative Management**: `kubectl apply namespace.yaml` creates namespace across all shards
- **GitOps Ready**: CRDs in Git → ArgoCD applies → Operator reconciles
- **Self-Healing**: Operator detects drift and corrects it
- **Reduced Ops Burden**: No manual config sync, deployment orchestration
- **Type Safety**: CRDs are schema-validated by Kubernetes API server
- **Extensibility**: Easy to add new CRDs (e.g., `PrismMigration` for shadow traffic automation)

### Negative

- **Operator Complexity**: Must maintain operator code (Rust + kube-rs or Go + controller-runtime)
- **Kubernetes Dependency**: Prism is now tightly coupled to Kubernetes (but can still run standalone)
- **Learning Curve**: Operators require understanding of reconciliation loops, watches, caching

### Neutral

- **CRD Versioning**: Must handle API versioning (v1alpha1 → v1beta1 → v1) over time
- **RBAC**: Operator needs permissions to create/update Deployments, Services, etc.
- **Observability**: Operator needs its own metrics, logging, tracing

## Implementation Notes

### Technology Stack

**Language**: Rust (kube-rs) or Go (controller-runtime/operator-sdk)
- **Rust**: Better type safety, performance
- **Go**: More mature operator ecosystem, examples

**Recommendation**: **Go with operator-sdk** (faster development, better docs)

### Project Structure

prism-operator/
├── Dockerfile
├── Makefile
├── go.mod
├── main.go                     # Operator entry point
├── api/
│   └── v1alpha1/
│       ├── prismnamespace_types.go
│       ├── prismshard_types.go
│       └── prismplugin_types.go
├── controllers/
│   ├── prismnamespace_controller.go
│   ├── prismshard_controller.go
│   └── prismplugin_controller.go
├── config/
│   ├── crd/                    # Generated CRD YAML
│   ├── rbac/                   # RBAC manifests
│   ├── manager/                # Operator deployment
│   └── samples/                # Example CRDs
└── tests/
    └── e2e/
```

### Example Controller Logic

```go
// PrismNamespaceReconciler reconciles a PrismNamespace object
func (r *PrismNamespaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := r.Log.WithValues("prismnamespace", req.NamespacedName)

    // 1. Fetch PrismNamespace
    var ns prismv1alpha1.PrismNamespace
    if err := r.Get(ctx, req.NamespacedName, &ns); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 2. Find appropriate shard for this namespace
    shard, err := r.findShardForNamespace(&ns)
    if err != nil {
        return ctrl.Result{}, err
    }

    // 3. Get Prism instance admin client
    prismClient, err := r.getPrismClient(shard)
    if err != nil {
        return ctrl.Result{}, err
    }

    // 4. Create/update namespace in Prism
    _, err = prismClient.CreateNamespace(ctx, &admin.CreateNamespaceRequest{
        Name:    ns.Spec.Name,
        Backend: ns.Spec.Backend,
        Pattern: ns.Spec.Pattern,
        // ... other config
    })
    if err != nil {
        return ctrl.Result{}, err
    }

    // 5. Update status
    ns.Status.State = "Active"
    ns.Status.PrismInstances = shard.Status.ReadyReplicas
    if err := r.Status().Update(ctx, &ns); err != nil {
        return ctrl.Result{}, err
    }

    log.Info("Reconciled PrismNamespace successfully")
    return ctrl.Result{}, nil
}
```

### Deployment

```bash
# Install CRDs
kubectl apply -f config/crd/

# Deploy operator
kubectl apply -f config/manager/

# Create PrismShard
kubectl apply -f config/samples/shard.yaml

# Create PrismNamespace
kubectl apply -f config/samples/namespace.yaml

# Check status
kubectl get prismnamespaces
kubectl get prismshards
kubectl describe prismnamespace user-profiles
```

### Integration with ADR-036 (SQLite Storage)

Operator provisions PersistentVolumeClaims for SQLite databases:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: prism-playback-0-config
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
  storageClassName: ssd
```

Each Prism pod mounts PVC at `/var/lib/prism/config.db`.

## References

- [Kubernetes Operator Pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)
- [operator-sdk Documentation](https://sdk.operatorframework.io/)
- [kube-rs](https://github.com/kube-rs/kube-rs)
- ADR-034: Product/Feature Sharding (shard deployment)
- ADR-036: SQLite Config Storage (what operator provisions)
- [PostgreSQL Operator](https://github.com/zalando/postgres-operator) (reference implementation)

## Revision History

- 2025-10-08: Initial draft proposing Kubernetes Operator with CRDs