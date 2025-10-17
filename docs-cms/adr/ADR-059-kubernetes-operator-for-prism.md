---
author: Platform Team
created: 2025-10-16
date: 2025-10-16
deciders: Platform Team, DevOps Team
doc_uuid: 9f3e5d7c-4a1b-4e8f-9d2a-6c8e9f0a1b2c
id: adr-059
project_id: prism-data-layer
status: Proposed
tags:
- kubernetes
- operator
- crd
- automation
- deployment
- control-plane
- orchestration
title: Kubernetes Operator for Declarative Prism Deployment
updated: 2025-10-16
---

# Kubernetes Operator for Declarative Prism Deployment

## Status

**Proposed** - Architecture design complete, implementation pending

## Context

Prism deployments on Kubernetes currently require manual YAML manifest management (MEMO-035) with several operational challenges:

### Current Deployment Challenges

1. **Static Configuration**: Manifests are immutable after apply - changing topology requires manual kubectl operations
2. **No Coordination**: Backing services, admin, proxy, and pattern runners deployed independently with no orchestration
3. **Manual Scaling**: Adding new pattern runners requires writing manifests, updating services, configuring backends
4. **Fragile Dependencies**: No automatic ordering (must deploy Redis before keyvalue-runner, NATS before consumer-runner)
5. **Configuration Drift**: Backend connection strings, service discovery, resource limits spread across 20+ YAML files
6. **No Runtime Adaptation**: Cannot dynamically add/remove patterns based on workload without full redeployment

### Prism Control Plane Evolution

Prism has a mature control plane architecture for process-level orchestration:

- **ADR-055**: Proxy-Admin control plane (namespace assignment, health monitoring)
- **ADR-056**: Launcher-Admin control plane (pattern provisioning, lifecycle management)
- **ADR-057**: Unified prism-launcher for all component types

**Missing piece**: Cloud-native control plane for Kubernetes deployments that provides the same declarative, flexible orchestration at cluster level.

### Kubernetes Operator Pattern

The Kubernetes Operator pattern extends Kubernetes API with custom controllers that manage complex applications:

- **Custom Resource Definitions (CRDs)**: Domain-specific resource types (e.g., `PrismCluster`)
- **Controller Reconciliation Loop**: Continuously ensures actual state matches desired state
- **Self-Healing**: Automatically recreates failed components
- **Declarative Configuration**: Single YAML defines entire stack
- **Runtime Flexibility**: Add/remove components by editing CRD spec

**Example Operators in Production**:
- Prometheus Operator (manages Prometheus deployments)
- Strimzi Kafka Operator (manages Kafka clusters)
- Postgres Operator (Zalando, CrunchyData)
- ArgoCD (GitOps deployments)

## Decision

**Implement Kubernetes Operator for Prism using Kubebuilder framework with CRDs for declarative cluster management.**

### Core Design

**Three-level CRD hierarchy**:

1. **PrismCluster** (v1alpha1): Top-level resource defining entire Prism deployment
2. **PrismNamespace** (future v1alpha2): Per-namespace configuration with pattern assignments
3. **PrismPattern** (future v1beta1): Individual pattern runner configuration

**Controller Architecture**:

```text
┌─────────────────────────────────────────────────────────────┐
│                    Kubernetes API Server                     │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ PrismCluster │  │    Service   │  │  Deployment  │      │
│  │     CRD      │  │   StatefulSet│  │     PVC      │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
           │                    │                    │
           │ Watch              │ Watch              │ Watch
           ▼                    ▼                    ▼
┌─────────────────────────────────────────────────────────────┐
│              Prism Operator (Controller Manager)             │
│  ┌──────────────────────────────────────────────────────┐   │
│  │         PrismCluster Controller                      │   │
│  │  ┌────────────────────────────────────────────────┐ │   │
│  │  │      Reconciliation Loop (every 10s)           │ │   │
│  │  │  1. Fetch PrismCluster spec                    │ │   │
│  │  │  2. Reconcile backing services (Redis, NATS)   │ │   │
│  │  │  3. Reconcile admin service                    │ │   │
│  │  │  4. Reconcile proxy with HPA                   │ │   │
│  │  │  5. Reconcile pattern runners                  │ │   │
│  │  │  6. Update status with component health        │ │   │
│  │  └────────────────────────────────────────────────┘ │   │
│  └──────────────────────────────────────────────────────┘   │
│  ┌──────────────────────────────────────────────────────┐   │
│  │         Service Discovery Manager                    │   │
│  │  - Generate ConfigMaps for backend connection strings│   │
│  │  - Create Services for inter-component communication │   │
│  └──────────────────────────────────────────────────────┘   │
│  ┌──────────────────────────────────────────────────────┐   │
│  │         Status Reporter                              │   │
│  │  - Aggregate component health                        │   │
│  │  - Update PrismCluster .status fields               │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
           │                    │                    │
           │ Create/Update      │ Create/Update      │ Create/Update
           ▼                    ▼                    ▼
┌─────────────────────────────────────────────────────────────┐
│                    Kubernetes Resources                      │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ Redis        │  │ prism-admin  │  │ prism-proxy  │      │
│  │ StatefulSet  │  │  Deployment  │  │  Deployment  │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ keyvalue-    │  │ consumer-    │  │ mailbox-     │      │
│  │ runner       │  │ runner       │  │ runner       │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
```

### PrismCluster CRD Schema (v1alpha1)

```go
// PrismClusterSpec defines the desired state of PrismCluster
type PrismClusterSpec struct {
    // Admin service configuration
    Admin AdminSpec `json:"admin"`

    // Proxy configuration
    Proxy ProxySpec `json:"proxy"`

    // Pattern runners
    Patterns []PatternSpec `json:"patterns"`

    // Backing services
    Backends BackendsSpec `json:"backends"`

    // Ingress configuration
    Ingress IngressSpec `json:"ingress,omitempty"`

    // Observability stack
    Observability ObservabilitySpec `json:"observability,omitempty"`
}

// AdminSpec defines prism-admin deployment
type AdminSpec struct {
    Replicas  int32                      `json:"replicas"`
    Storage   StorageSpec                `json:"storage"`
    Resources corev1.ResourceRequirements `json:"resources"`
}

// ProxySpec defines prism-proxy deployment
type ProxySpec struct {
    Replicas    int32                      `json:"replicas"`
    Autoscaling *AutoscalingSpec           `json:"autoscaling,omitempty"`
    Resources   corev1.ResourceRequirements `json:"resources"`
}

// PatternSpec defines a pattern runner
type PatternSpec struct {
    Name      string                     `json:"name"`
    Type      string                     `json:"type"` // Deployment or StatefulSet
    Replicas  int32                      `json:"replicas"`
    Backends  []string                   `json:"backends"`
    Storage   *StorageSpec               `json:"storage,omitempty"`
    Resources corev1.ResourceRequirements `json:"resources"`
}

// BackendsSpec defines backing services
type BackendsSpec struct {
    Redis    *BackendServiceSpec `json:"redis,omitempty"`
    NATS     *BackendServiceSpec `json:"nats,omitempty"`
    Postgres *BackendServiceSpec `json:"postgres,omitempty"`
    MinIO    *BackendServiceSpec `json:"minio,omitempty"`
    Kafka    *BackendServiceSpec `json:"kafka,omitempty"`
}

// BackendServiceSpec defines a backing service
type BackendServiceSpec struct {
    Enabled  bool        `json:"enabled"`
    Replicas int32       `json:"replicas"`
    Storage  string      `json:"storage,omitempty"` // e.g., "1Gi"
    Version  string      `json:"version"`
    Database string      `json:"database,omitempty"` // For Postgres
}

// PrismClusterStatus defines observed state
type PrismClusterStatus struct {
    // Phase: Pending, Running, Degraded, Failed
    Phase string `json:"phase"`

    // Conditions (Ready, AdminHealthy, ProxyHealthy, etc.)
    Conditions []metav1.Condition `json:"conditions,omitempty"`

    // Component health
    AdminStatus   ComponentStatus   `json:"adminStatus"`
    ProxyStatus   ComponentStatus   `json:"proxyStatus"`
    PatternStatus []PatternStatus   `json:"patternStatus"`
    BackendStatus []BackendStatus   `json:"backendStatus"`

    // Observability
    ObservedGeneration int64  `json:"observedGeneration"`
    LastReconciled     string `json:"lastReconciled"`
}

// ComponentStatus tracks individual component health
type ComponentStatus struct {
    Name             string `json:"name"`
    Ready            bool   `json:"ready"`
    Replicas         int32  `json:"replicas"`
    AvailableReplicas int32  `json:"availableReplicas"`
    Message          string `json:"message,omitempty"`
}
```

### Example PrismCluster Resource

```yaml
apiVersion: prism.io/v1alpha1
kind: PrismCluster
metadata:
  name: prism-local
  namespace: prism
spec:
  # Admin service
  admin:
    replicas: 1
    storage:
      type: sqlite
      size: 1Gi
    resources:
      requests: {memory: 128Mi, cpu: 100m}
      limits: {memory: 256Mi, cpu: 500m}

  # Proxy with autoscaling
  proxy:
    replicas: 2
    autoscaling:
      enabled: true
      minReplicas: 2
      maxReplicas: 10
      targetCPU: 70
      targetMemory: 80
    resources:
      requests: {memory: 256Mi, cpu: 200m}
      limits: {memory: 512Mi, cpu: 1000m}

  # Pattern runners
  patterns:
    - name: keyvalue
      type: StatefulSet
      replicas: 1
      backends: [redis]
      resources:
        requests: {memory: 128Mi, cpu: 100m}
        limits: {memory: 256Mi, cpu: 500m}

    - name: consumer
      type: Deployment
      replicas: 2
      backends: [nats]
      resources:
        requests: {memory: 128Mi, cpu: 100m}
        limits: {memory: 256Mi, cpu: 500m}

    - name: producer
      type: Deployment
      replicas: 2
      backends: [nats]
      resources:
        requests: {memory: 128Mi, cpu: 100m}
        limits: {memory: 256Mi, cpu: 500m}

    - name: mailbox
      type: StatefulSet
      replicas: 1
      backends: [minio, sqlite]
      storage:
        size: 1Gi
      resources:
        requests: {memory: 128Mi, cpu: 100m}
        limits: {memory: 256Mi, cpu: 500m}

  # Backing services
  backends:
    redis:
      enabled: true
      replicas: 1
      storage: 1Gi
      version: "7-alpine"

    nats:
      enabled: true
      replicas: 1
      version: "latest"

    postgres:
      enabled: false

    minio:
      enabled: true
      replicas: 1
      storage: 5Gi
      version: "latest"

    kafka:
      enabled: false

  # Ingress
  ingress:
    enabled: true
    className: nginx
    host: prism.local
    annotations:
      nginx.ingress.kubernetes.io/backend-protocol: "GRPC"

  # Observability
  observability:
    prometheus:
      enabled: true
      scrapeInterval: 30s
    grafana:
      enabled: true
    loki:
      enabled: false
```

### Controller Reconciliation Logic

```go
func (r *PrismClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // 1. Fetch PrismCluster
    var prismCluster prismv1alpha1.PrismCluster
    if err := r.Get(ctx, req.NamespacedName, &prismCluster); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 2. Reconcile backing services (in dependency order)
    if err := r.reconcileBackends(ctx, &prismCluster); err != nil {
        return ctrl.Result{}, err
    }

    // 3. Reconcile admin service
    if err := r.reconcileAdmin(ctx, &prismCluster); err != nil {
        return ctrl.Result{}, err
    }

    // 4. Reconcile proxy
    if err := r.reconcileProxy(ctx, &prismCluster); err != nil {
        return ctrl.Result{}, err
    }

    // 5. Reconcile pattern runners
    if err := r.reconcilePatterns(ctx, &prismCluster); err != nil {
        return ctrl.Result{}, err
    }

    // 6. Reconcile ingress
    if err := r.reconcileIngress(ctx, &prismCluster); err != nil {
        return ctrl.Result{}, err
    }

    // 7. Update status
    if err := r.updateStatus(ctx, &prismCluster); err != nil {
        return ctrl.Result{}, err
    }

    // Requeue after 10s for health checks
    return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *PrismClusterReconciler) reconcileBackends(ctx context.Context, cluster *prismv1alpha1.PrismCluster) error {
    // For each enabled backend, create/update StatefulSet + Service + PVC
    if cluster.Spec.Backends.Redis != nil && cluster.Spec.Backends.Redis.Enabled {
        if err := r.reconcileRedis(ctx, cluster); err != nil {
            return err
        }
    }

    if cluster.Spec.Backends.NATS != nil && cluster.Spec.Backends.NATS.Enabled {
        if err := r.reconcileNATS(ctx, cluster); err != nil {
            return err
        }
    }

    // ... more backends
    return nil
}

func (r *PrismClusterReconciler) reconcileRedis(ctx context.Context, cluster *prismv1alpha1.PrismCluster) error {
    // Create StatefulSet for Redis
    statefulSet := &appsv1.StatefulSet{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "redis",
            Namespace: cluster.Namespace,
            Labels:    map[string]string{"app": "redis", "managed-by": "prism-operator"},
        },
        Spec: appsv1.StatefulSetSpec{
            Replicas: &cluster.Spec.Backends.Redis.Replicas,
            Selector: &metav1.LabelSelector{
                MatchLabels: map[string]string{"app": "redis"},
            },
            Template: corev1.PodTemplateSpec{
                ObjectMeta: metav1.ObjectMeta{
                    Labels: map[string]string{"app": "redis"},
                },
                Spec: corev1.PodSpec{
                    Containers: []corev1.Container{
                        {
                            Name:  "redis",
                            Image: fmt.Sprintf("redis:%s", cluster.Spec.Backends.Redis.Version),
                            Ports: []corev1.ContainerPort{{ContainerPort: 6379}},
                            VolumeMounts: []corev1.VolumeMount{
                                {Name: "data", MountPath: "/data"},
                            },
                        },
                    },
                },
            },
            VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
                {
                    ObjectMeta: metav1.ObjectMeta{Name: "data"},
                    Spec: corev1.PersistentVolumeClaimSpec{
                        AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
                        Resources: corev1.ResourceRequirements{
                            Requests: corev1.ResourceList{
                                corev1.ResourceStorage: resource.MustParse(cluster.Spec.Backends.Redis.Storage),
                            },
                        },
                    },
                },
            },
        },
    }

    // Set controller reference (for garbage collection)
    ctrl.SetControllerReference(cluster, statefulSet, r.Scheme)

    // Create or update
    if err := r.Client.Create(ctx, statefulSet); err != nil {
        if errors.IsAlreadyExists(err) {
            return r.Client.Update(ctx, statefulSet)
        }
        return err
    }

    // Create headless service for Redis
    service := &corev1.Service{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "redis",
            Namespace: cluster.Namespace,
        },
        Spec: corev1.ServiceSpec{
            ClusterIP: "None",
            Selector:  map[string]string{"app": "redis"},
            Ports:     []corev1.ServicePort{{Port: 6379, Name: "redis"}},
        },
    }
    ctrl.SetControllerReference(cluster, service, r.Scheme)

    if err := r.Client.Create(ctx, service); err != nil {
        if errors.IsAlreadyExists(err) {
            return r.Client.Update(ctx, service)
        }
        return err
    }

    return nil
}

func (r *PrismClusterReconciler) reconcilePatterns(ctx context.Context, cluster *prismv1alpha1.PrismCluster) error {
    // For each pattern, create Deployment or StatefulSet
    for _, pattern := range cluster.Spec.Patterns {
        // Generate backend connection env vars
        envVars := r.generateBackendEnv(cluster, pattern.Backends)

        if pattern.Type == "StatefulSet" {
            if err := r.createPatternStatefulSet(ctx, cluster, pattern, envVars); err != nil {
                return err
            }
        } else {
            if err := r.createPatternDeployment(ctx, cluster, pattern, envVars); err != nil {
                return err
            }
        }
    }
    return nil
}

func (r *PrismClusterReconciler) generateBackendEnv(cluster *prismv1alpha1.PrismCluster, backends []string) []corev1.EnvVar {
    var envVars []corev1.EnvVar

    for _, backend := range backends {
        switch backend {
        case "redis":
            envVars = append(envVars, corev1.EnvVar{
                Name:  "REDIS_URL",
                Value: fmt.Sprintf("redis://redis.%s.svc.cluster.local:6379", cluster.Namespace),
            })
        case "nats":
            envVars = append(envVars, corev1.EnvVar{
                Name:  "NATS_URL",
                Value: fmt.Sprintf("nats://nats.%s.svc.cluster.local:4222", cluster.Namespace),
            })
        case "minio":
            envVars = append(envVars,
                corev1.EnvVar{Name: "S3_ENDPOINT", Value: fmt.Sprintf("http://minio.%s.svc.cluster.local:9000", cluster.Namespace)},
                corev1.EnvVar{Name: "S3_ACCESS_KEY", Value: "minioadmin"},
                corev1.EnvVar{Name: "S3_SECRET_KEY", Value: "minioadmin"},
            )
        }
    }

    return envVars
}

func (r *PrismClusterReconciler) updateStatus(ctx context.Context, cluster *prismv1alpha1.PrismCluster) error {
    // Aggregate component health
    adminReady := r.isComponentReady(ctx, cluster.Namespace, "prism-admin")
    proxyReady := r.isComponentReady(ctx, cluster.Namespace, "prism-proxy")

    cluster.Status.AdminStatus = ComponentStatus{
        Name:  "prism-admin",
        Ready: adminReady,
    }
    cluster.Status.ProxyStatus = ComponentStatus{
        Name:  "prism-proxy",
        Ready: proxyReady,
    }

    // Update phase
    if adminReady && proxyReady {
        cluster.Status.Phase = "Running"
    } else {
        cluster.Status.Phase = "Degraded"
    }

    cluster.Status.LastReconciled = time.Now().Format(time.RFC3339)

    return r.Status().Update(ctx, cluster)
}
```

### Runtime Flexibility Examples

**Example 1: Add new pattern runner**

```bash
kubectl edit prismcluster prism-local

# Add to spec.patterns:
# - name: claimcheck
#   type: Deployment
#   replicas: 2
#   backends: [minio, redis]
#   resources:
#     requests: {memory: 128Mi, cpu: 100m}
#     limits: {memory: 256Mi, cpu: 500m}

# Controller automatically:
# 1. Creates claimcheck-runner Deployment
# 2. Injects MINIO_URL and REDIS_URL env vars
# 3. Creates Service for discovery
# 4. Updates status with new pattern health
```

**Example 2: Scale proxy based on load**

```bash
kubectl patch prismcluster prism-local --type='json' -p='[
  {"op": "replace", "path": "/spec/proxy/autoscaling/maxReplicas", "value": 20}
]'

# Controller updates HPA immediately
```

**Example 3: Enable Kafka backend**

```bash
kubectl patch prismcluster prism-local --type='json' -p='[
  {"op": "replace", "path": "/spec/backends/kafka/enabled", "value": true},
  {"op": "add", "path": "/spec/backends/kafka/replicas", "value": 3},
  {"op": "add", "path": "/spec/backends/kafka/version", "value": "latest"}
]'

# Controller:
# 1. Deploys Kafka StatefulSet + ZooKeeper
# 2. Creates Services
# 3. Updates ConfigMaps with Kafka URLs
# 4. Restarts pattern runners with new env vars
```

**Example 4: Check cluster health**

```bash
kubectl get prismcluster prism-local -o yaml

# Output shows detailed status:
# status:
#   phase: Running
#   adminStatus:
#     name: prism-admin
#     ready: true
#     replicas: 1
#     availableReplicas: 1
#   proxyStatus:
#     name: prism-proxy
#     ready: true
#     replicas: 3
#     availableReplicas: 3
#   patternStatus:
#     - name: keyvalue-runner
#       ready: true
#       replicas: 1
#       availableReplicas: 1
#     - name: consumer-runner
#       ready: true
#       replicas: 2
#       availableReplicas: 2
#   backendStatus:
#     - name: redis
#       ready: true
#       replicas: 1
#       availableReplicas: 1
#   lastReconciled: "2025-10-16T12:34:56Z"
```

## Technology Choice: Kubebuilder

**Kubebuilder** selected over Operator SDK for the following reasons:

| Feature | Kubebuilder | Operator SDK |
|---------|-------------|--------------|
| **Maintainer** | Kubernetes SIG API Machinery | Red Hat (community edition) |
| **Language** | Go only | Go, Ansible, Helm |
| **Complexity** | Lower (single approach) | Higher (multiple approaches) |
| **Code Generation** | Excellent (controller-gen) | Good |
| **Testing** | envtest built-in | Requires setup |
| **Documentation** | Excellent (kubebuilder book) | Good |
| **Community** | Very active | Active |
| **Production Use** | Prometheus, Istio, ArgoCD | Many Red Hat operators |

**Decision**: Kubebuilder provides cleaner Go-first approach with better alignment to Kubernetes SIG projects.

## Implementation Plan

### Phase 1: Scaffolding (Week 1)

```bash
# Initialize operator project
kubebuilder init --domain prism.io --repo github.com/jrepp/prism-data-layer/operator

# Create PrismCluster CRD
kubebuilder create api --group prism --version v1alpha1 --kind PrismCluster

# Generate CRD manifests
make manifests

# Generate deepcopy code
make generate
```

**Deliverables**:
- `operator/` directory with Kubebuilder scaffolding
- `api/v1alpha1/prismcluster_types.go` with CRD schema
- `controllers/prismcluster_controller.go` with reconciler stub
- `config/crd/` with generated CRD YAML
- `config/samples/` with example PrismCluster resource

### Phase 2: Backend Reconciliation (Week 2)

**Tasks**:
- Implement `reconcileBackends()` for Redis, NATS, Postgres, MinIO
- Create StatefulSets with volume claims
- Create headless Services for service discovery
- Generate ConfigMaps with connection strings
- Add owner references for garbage collection

**Tests**:
- Unit tests with envtest (in-memory Kubernetes API)
- Integration tests with k3d cluster
- Test backend provisioning order (Redis → NATS → patterns)

### Phase 3: Admin and Proxy Reconciliation (Week 3)

**Tasks**:
- Implement `reconcileAdmin()` with SQLite or Postgres storage
- Implement `reconcileProxy()` with optional HPA
- Create Deployments with health checks
- Configure resource requests/limits
- Add liveness/readiness probes

**Tests**:
- Admin deployment with SQLite PVC
- Admin deployment with Postgres backend
- Proxy deployment with HPA enabled/disabled
- Rolling update scenarios

### Phase 4: Pattern Runner Reconciliation (Week 4)

**Tasks**:
- Implement `reconcilePatterns()` for all pattern types
- Support both Deployment and StatefulSet
- Inject backend connection env vars dynamically
- Create Services for pattern discovery
- Handle pattern additions/removals

**Tests**:
- Pattern runner creation with correct backends
- Pattern scaling (replicas change)
- Pattern deletion with graceful cleanup
- Multi-backend patterns (mailbox with S3 + SQLite)

### Phase 5: Status Management (Week 5)

**Tasks**:
- Implement `updateStatus()` with component health
- Aggregate Deployment/StatefulSet readiness
- Set PrismCluster phase (Pending, Running, Degraded, Failed)
- Add Conditions for detailed state tracking
- Update observedGeneration and timestamps

**Tests**:
- Status updates on component changes
- Phase transitions (Pending → Running → Degraded)
- Condition management (Ready, AdminHealthy, ProxyHealthy)

### Phase 6: Ingress and Observability (Week 6)

**Tasks**:
- Implement `reconcileIngress()` with Nginx configuration
- Add Prometheus ServiceMonitor CRDs
- Configure Grafana dashboards via ConfigMaps
- Optional Loki integration
- TLS certificate management (cert-manager)

**Tests**:
- Ingress creation with correct annotations
- ServiceMonitor generation for Prometheus
- Grafana dashboard provisioning

### Phase 7: Production Hardening (Week 7-8)

**Tasks**:
- Leader election for multi-replica operator
- Retry logic with exponential backoff
- Rate limiting for reconciliation
- Finalizers for cleanup on deletion
- Webhook validation for spec changes
- RBAC configuration (ClusterRole, ServiceAccount)
- Security context for operator pod

**Tests**:
- Leader election with 3 operator replicas
- Invalid spec rejection via webhook
- Finalizer cleanup on PrismCluster deletion
- RBAC permission verification

## Comparison with Alternatives

### Helm Charts (Current Alternative)

| Aspect | Helm Chart | Kubernetes Operator |
|--------|-----------|---------------------|
| **Configuration** | Static (install-time only) | Dynamic (runtime changes) |
| **Updates** | Manual `helm upgrade` | Automatic reconciliation |
| **Dependency Management** | Hooks (limited ordering) | Full reconciliation loop |
| **Self-Healing** | None | Automatic recreation |
| **Status Reporting** | None | Rich status fields |
| **Extensibility** | Templates + values | Custom controllers |
| **Learning Curve** | Lower | Higher |
| **GitOps Support** | Good | Excellent |

**Decision**: Operator provides superior runtime flexibility and self-healing. Helm remains useful for initial operator installation.

### Manual YAML Manifests (MEMO-035)

| Aspect | Manual YAML | Kubernetes Operator |
|--------|-------------|---------------------|
| **Simplicity** | Very simple | More complex |
| **Flexibility** | Low (static) | High (dynamic) |
| **Maintenance** | High (20+ files) | Low (single CRD) |
| **Orchestration** | Manual kubectl order | Automatic |
| **Learning** | Quick start | Requires operator knowledge |

**Decision**: MEMO-035 remains best for learning and POC. Operator recommended for production deployments.

### Kustomize Overlays

| Aspect | Kustomize | Kubernetes Operator |
|--------|-----------|---------------------|
| **Configuration** | Multiple overlays (dev/staging/prod) | Single CRD with env-specific values |
| **Updates** | Manual `kubectl apply -k` | Automatic reconciliation |
| **Validation** | None | Webhook validation |
| **Status** | None | Rich status fields |

**Decision**: Kustomize useful for non-operator resources. Operator handles Prism-specific orchestration.

## Migration Path

### For New Users

**Recommended Path**:
1. **Week 1-2**: Start with MEMO-035 manual YAML for learning
2. **Week 3-4**: Convert to Helm chart for easier deployment
3. **Production**: Use Kubernetes Operator for runtime flexibility

### For Existing Deployments

**Migration Steps**:

1. **Backup existing state**:
```bash
kubectl get all -n prism -o yaml > prism-backup.yaml
```

2. **Install operator**:
```bash
kubectl apply -f https://github.com/jrepp/prism-data-layer/releases/download/v0.1.0/prism-operator.yaml
```

3. **Create PrismCluster matching existing deployment**:
```bash
# Generate PrismCluster from existing resources
prismctl kubernetes export --namespace prism > prismcluster.yaml

# Apply CRD
kubectl apply -f prismcluster.yaml
```

4. **Transfer ownership to operator**:
```bash
# Operator automatically adopts existing resources via label matching
# No downtime - existing pods continue running
```

5. **Verify operator management**:
```bash
kubectl get prismcluster prism-local -o yaml
# Status should show all components as Ready
```

## Deployment Workflow

### Development

```bash
# 1. Deploy operator to k3d cluster
make docker-build
k3d image import prism-operator:latest -c prism-local
kubectl apply -f config/crd/bases/prism.io_prismclusters.yaml
kubectl apply -f config/rbac/
kubectl apply -f config/manager/manager.yaml

# 2. Create PrismCluster
kubectl apply -f config/samples/prism_v1alpha1_prismcluster.yaml

# 3. Watch reconciliation
kubectl get prismcluster prism-local -w

# 4. Check detailed status
kubectl describe prismcluster prism-local
```

### Production

```bash
# 1. Install operator via Helm
helm repo add prism https://jrepp.github.io/prism-operator
helm install prism-operator prism/prism-operator --namespace prism-system --create-namespace

# 2. Create PrismCluster in target namespace
kubectl create namespace prism
kubectl apply -f prismcluster-production.yaml -n prism

# 3. GitOps: Commit PrismCluster to Git, ArgoCD syncs automatically
git add k8s/prismcluster-production.yaml
git commit -m "Deploy Prism to production"
git push
# ArgoCD detects change and applies
```

## Observability

### Operator Metrics

The operator exposes Prometheus metrics:

```promql
# Reconciliation loop metrics
prism_operator_reconcile_duration_seconds{controller="prismcluster"}
prism_operator_reconcile_errors_total{controller="prismcluster"}
prism_operator_reconcile_success_total{controller="prismcluster"}

# Component creation metrics
prism_operator_resource_created_total{resource_type="statefulset"}
prism_operator_resource_updated_total{resource_type="deployment"}
prism_operator_resource_deleted_total{resource_type="service"}

# Cluster health
prism_cluster_phase{cluster="prism-local",phase="running"} 1
prism_cluster_components_ready{cluster="prism-local",component="admin"} 1
prism_cluster_components_ready{cluster="prism-local",component="proxy"} 1
prism_cluster_patterns_ready{cluster="prism-local",pattern="keyvalue"} 1
```

### Logging

Operator uses structured logging (zap):

```json
{
  "level": "info",
  "ts": "2025-10-16T12:34:56.789Z",
  "msg": "Reconciling PrismCluster",
  "controller": "prismcluster",
  "namespace": "prism",
  "name": "prism-local",
  "reconcile_id": "abc123"
}
{
  "level": "info",
  "ts": "2025-10-16T12:34:57.123Z",
  "msg": "Created Redis StatefulSet",
  "controller": "prismcluster",
  "namespace": "prism",
  "statefulset": "redis"
}
```

## Consequences

### Positive

1. **Declarative Deployment**: Single YAML defines entire Prism stack
2. **Runtime Flexibility**: Add/remove components without manual kubectl commands
3. **Self-Healing**: Automatically recreates failed components
4. **Operational Simplicity**: Controller handles complex orchestration logic
5. **GitOps Ready**: PrismCluster resources commit to Git, ArgoCD syncs automatically
6. **Status Visibility**: Rich status fields expose component health
7. **Extensibility**: Future CRDs (PrismNamespace, PrismPattern) extend capabilities
8. **Backend Coordination**: Automatic service discovery and dependency ordering
9. **Resource Management**: Centralized resource limits and autoscaling configuration
10. **Production-Grade**: Follows Kubernetes best practices with RBAC, webhooks, finalizers

### Negative

1. **Increased Complexity**: Requires understanding of Kubernetes operators
2. **Development Overhead**: More code than static manifests (~2000 LOC vs 500 LOC)
3. **Testing Complexity**: Needs envtest and integration test infrastructure
4. **Maintenance Burden**: Operator itself requires updates, bug fixes, security patches
5. **Learning Curve**: Team needs to learn Kubebuilder, controller-runtime, CRD schemas
6. **Initial Setup Time**: 8 weeks implementation vs 1 week for Helm chart
7. **Debugging Complexity**: Reconciliation loop bugs harder to diagnose than static YAML

### Neutral

1. **Helm Still Useful**: Operator itself installed via Helm chart
2. **MEMO-035 Remains Valid**: Static manifests still best for learning and POC
3. **Migration Path Required**: Existing deployments need adoption strategy
4. **Operator Versioning**: Separate versioning from Prism components (operator v0.1.0, Prism v1.5.0)

## Future Extensions

### PrismNamespace CRD (v1alpha2)

Manage individual namespaces with pattern assignments:

```yaml
apiVersion: prism.io/v1alpha2
kind: PrismNamespace
metadata:
  name: orders
spec:
  cluster: prism-local
  patterns:
    - keyvalue
    - consumer
  isolation: namespace
  quota:
    maxSessions: 1000
    maxRequestsPerSecond: 10000
```

### PrismPattern CRD (v1beta1)

Fine-grained pattern configuration:

```yaml
apiVersion: prism.io/v1beta1
kind: PrismPattern
metadata:
  name: keyvalue-redis-high-memory
spec:
  type: keyvalue
  backends:
    - name: redis
      config:
        maxConnections: 1000
        evictionPolicy: allkeys-lru
  resources:
    requests: {memory: 1Gi, cpu: 500m}
    limits: {memory: 2Gi, cpu: 2000m}
  autoscaling:
    enabled: true
    minReplicas: 2
    maxReplicas: 20
    targetMemory: 80
```

### Multi-Cluster Support

Operator manages Prism deployments across multiple Kubernetes clusters:

```yaml
apiVersion: prism.io/v1alpha3
kind: PrismFederation
metadata:
  name: global-prism
spec:
  clusters:
    - name: us-west-2
      endpoint: https://k8s-usw2.example.com
    - name: eu-central-1
      endpoint: https://k8s-euc1.example.com
  routing:
    strategy: latency-based
    failover: enabled
```

## References

- **Kubebuilder Book**: https://book.kubebuilder.io/
- **ADR-055**: Proxy-Admin Control Plane Protocol
- **ADR-056**: Launcher-Admin Control Plane Protocol
- **ADR-057**: Refactor pattern-launcher to prism-launcher
- **MEMO-035**: Local Kubernetes Deployment with k3d
- **Operator Pattern**: https://kubernetes.io/docs/concepts/extend-kubernetes/operator/
- **Custom Resources**: https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/

## Revision History

- 2025-10-16: Initial proposal with CRD schema, controller design, implementation plan
