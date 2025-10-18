# Prism Kubernetes Operator - Quick Start

Get the Prism operator running on Docker Desktop Kubernetes in 5 minutes.

## Prerequisites

- Docker Desktop with Kubernetes enabled
- `kubectl` configured for docker-desktop context
- `helm` (optional, for KEDA installation)
- Go 1.21+ (for building from source)

## Installation

### 1. Install CRDs

```bash
kubectl apply -f config/crd/bases/prism.io_prismpatterns.yaml
```

### 2. Install Dependencies (Optional)

For auto-scaling support, install metrics-server and/or KEDA:

```bash
# HPA support (CPU/memory based autoscaling)
make local-install-metrics

# KEDA support (event-driven autoscaling)
make local-install-keda

# Or install both
make local-install-deps
```

See [KEDA_INSTALL_GUIDE.md](KEDA_INSTALL_GUIDE.md) for detailed KEDA installation options.

### 3. Build and Run Operator

```bash
# Build operator
go build -o bin/manager cmd/manager/main.go

# Run locally
./bin/manager
```

Or use the Makefile:

```bash
make build
make run
```

## Deploy Your First Pattern

### Basic Pattern (No Autoscaling)

```bash
kubectl apply -f - <<EOF
apiVersion: prism.io/v1alpha1
kind: PrismPattern
metadata:
  name: my-first-pattern
  namespace: prism-system
spec:
  pattern: keyvalue
  backend: redis
  image: busybox:latest
  replicas: 2

  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "200m"
      memory: "256Mi"

  service:
    type: ClusterIP
    port: 8080
EOF
```

### Pattern with HPA Autoscaling

```bash
kubectl apply -f config/samples/prismpattern_hpa_example.yaml
```

### Pattern with KEDA Autoscaling

```bash
kubectl apply -f config/samples/test-keda-simple.yaml
```

## Verify Installation

```bash
# Check operator is running
kubectl get pods -n prism-system

# Check PrismPattern status
kubectl get prismpattern -n prism-system

# Check created resources
kubectl get deployment,service,hpa,scaledobject -n prism-system

# View pattern status
kubectl describe prismpattern my-first-pattern -n prism-system
```

## Status Tracking

The operator updates PrismPattern status with:

- **Phase**: `Pending` → `Progressing` → `Running`
- **Replicas**: Current replica count
- **AvailableReplicas**: Ready replica count
- **Conditions**: Ready condition with details

```bash
# Watch pattern status
kubectl get prismpattern -w

# Check detailed status
kubectl get prismpattern my-first-pattern -o jsonpath='{.status}' | jq .
```

## Autoscaling Options

### Option 1: HPA (Horizontal Pod Autoscaler)

CPU/memory-based scaling using Kubernetes native HPA:

```yaml
autoscaling:
  enabled: true
  scaler: hpa  # or omit (hpa is default)
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilizationPercentage: 80
  targetMemoryUtilizationPercentage: 80
```

**Requirements**: metrics-server installed (`make local-install-metrics`)

### Option 2: KEDA (Event-Driven Autoscaling)

Scale based on external metrics (Kafka lag, queue depth, etc.):

```yaml
autoscaling:
  enabled: true
  scaler: keda
  minReplicas: 1
  maxReplicas: 50
  pollingInterval: 10
  cooldownPeriod: 300
  triggers:
    - type: kafka
      metadata:
        bootstrapServers: "kafka:9092"
        consumerGroup: "my-group"
        topic: "my-topic"
        lagThreshold: "1000"
```

**Requirements**: KEDA installed (`make local-install-keda`)

See [KEDA_INSTALL_GUIDE.md](KEDA_INSTALL_GUIDE.md) for 60+ supported scalers.

## Cleanup

```bash
# Delete pattern (cascades to deployment/service/hpa/scaledobject)
kubectl delete prismpattern my-first-pattern -n prism-system

# Uninstall KEDA (optional)
make local-uninstall-keda

# Uninstall CRDs
kubectl delete -f config/crd/bases/prism.io_prismpatterns.yaml
```

## Makefile Commands

```bash
# Build
make build                   # Build operator binary
make docker-build            # Build Docker image

# Development
make run                     # Run operator locally
make test                    # Run tests
make fmt                     # Format code
make vet                     # Run go vet

# CRDs
make install                 # Install CRDs
make uninstall               # Uninstall CRDs
make manifests               # Generate manifests

# Local Development (Docker Desktop)
make local-install-metrics   # Install metrics-server
make local-install-keda      # Install KEDA
make local-install-deps      # Install both
make local-run               # Run operator against Docker Desktop
make local-test-hpa          # Test HPA example
make local-test-keda         # Test KEDA example
make local-status            # Show all Prism resources
make local-clean             # Clean up all patterns

# KEDA Management
make local-keda-status       # Show KEDA status
make local-uninstall-keda    # Uninstall KEDA
```

## Examples

### Production-Ready Pattern

```yaml
apiVersion: prism.io/v1alpha1
kind: PrismPattern
metadata:
  name: orders-processor
  namespace: production
spec:
  pattern: consumer
  backend: kafka
  image: ghcr.io/myorg/orders-processor:v1.2.3
  replicas: 3

  resources:
    requests:
      cpu: "500m"
      memory: "1Gi"
    limits:
      cpu: "2000m"
      memory: "4Gi"

  service:
    type: ClusterIP
    port: 8080

  # KEDA autoscaling based on Kafka lag
  autoscaling:
    enabled: true
    scaler: keda
    minReplicas: 3
    maxReplicas: 50
    pollingInterval: 10
    cooldownPeriod: 300

    triggers:
      - type: kafka
        metadata:
          bootstrapServers: "kafka.production.svc:9092"
          consumerGroup: "orders-processor"
          topic: "orders"
          lagThreshold: "1000"

    # Advanced scaling behavior
    behavior:
      scaleDown:
        stabilizationWindowSeconds: 300
        policies:
          - type: Percent
            value: 50
            periodSeconds: 60
      scaleUp:
        stabilizationWindowSeconds: 0
        policies:
          - type: Percent
            value: 100
            periodSeconds: 15

  # Placement configuration
  placement:
    nodeSelector:
      workload-type: compute-intensive
    tolerations:
      - key: "dedicated"
        operator: "Equal"
        value: "prism"
        effect: "NoSchedule"
    affinity:
      podAntiAffinity:
        preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                  - key: prism.io/pattern-instance
                    operator: In
                    values:
                      - orders-processor
              topologyKey: kubernetes.io/hostname
```

## Troubleshooting

### Operator Not Starting

```bash
# Check logs
tail -f /tmp/operator.log  # If running via Makefile
# or
kubectl logs -n prism-system deployment/prism-operator  # If deployed

# Check CRDs are installed
kubectl get crd prismpatterns.prism.io

# Check RBAC permissions
kubectl auth can-i create deployments --as=system:serviceaccount:prism-system:prism-operator
```

### Pattern Not Reconciling

```bash
# Check pattern status
kubectl describe prismpattern <name> -n <namespace>

# Check operator logs for errors
grep ERROR /tmp/operator.log

# Check deployment created
kubectl get deployment <name> -n <namespace>
```

### KEDA Not Working

```bash
# Check KEDA installed
./scripts/install-keda.sh status

# Check ScaledObject
kubectl describe scaledobject <name> -n <namespace>

# Check KEDA operator logs
kubectl logs -n keda deployment/keda-operator -f
```

### Autoscaling Not Triggering

**For HPA**:
```bash
# Check metrics-server running
kubectl get deployment metrics-server -n kube-system

# Check metrics available
kubectl top nodes
kubectl top pods -n <namespace>

# Check HPA status
kubectl describe hpa <name> -n <namespace>
```

**For KEDA**:
```bash
# Check ScaledObject status
kubectl get scaledobject <name> -n <namespace> -o yaml

# Check if scaler is active
kubectl get scaledobject <name> -n <namespace> -o jsonpath='{.status.conditions[?(@.type=="Active")].status}'

# Check KEDA HPA created
kubectl get hpa keda-hpa-<name> -n <namespace>
```

## Architecture

```
┌─────────────────┐
│  PrismPattern   │  ← Custom Resource (user creates)
└────────┬────────┘
         │
         │ watches
         ↓
┌─────────────────────────────────────┐
│      Prism Operator                 │
│  ┌──────────────────────────────┐   │
│  │  PrismPattern Controller     │   │
│  │  - Reconcile Deployment      │   │
│  │  - Reconcile Service         │   │
│  │  - Reconcile HPA/KEDA        │   │
│  └──────────────────────────────┘   │
└─────────────────┬───────────────────┘
                  │ creates/updates
                  ↓
         ┌────────────────┐
         │  Deployment    │  ← Kubernetes native
         └────────────────┘
         ┌────────────────┐
         │  Service       │  ← Kubernetes native
         └────────────────┘
         ┌────────────────┐
         │  HPA or        │  ← Auto-scaling (optional)
         │  ScaledObject  │
         └────────────────┘
```

## Next Steps

- Read [TEST_REPORT.md](TEST_REPORT.md) for detailed test results
- See [KEDA_INSTALL_GUIDE.md](KEDA_INSTALL_GUIDE.md) for KEDA details
- Explore example patterns in `config/samples/`
- Check operator code in `controllers/` and `pkg/autoscaling/`

## Support

For issues or questions:
1. Check operator logs for errors
2. Verify CRDs are installed correctly
3. Ensure dependencies (metrics-server/KEDA) are running
4. Review example patterns in `config/samples/`
5. Open an issue with logs and pattern YAML
