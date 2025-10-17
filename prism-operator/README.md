# Prism Kubernetes Operator

Kubernetes operator for the Prism data access gateway, providing auto-scaling orchestration for proxies and pattern runners.

## Features

- **Declarative Configuration**: Define entire Prism stack with Kubernetes CRDs
- **Auto-Scaling**: Built-in support for both HPA (CPU/memory) and KEDA (event-driven)
- **Multi-Tenant**: Namespace provisioning and isolation
- **Production-Ready**: High availability, placement control, and observability

## Architecture

The operator manages four primary resources:

- **PrismStack**: Complete Prism deployment (proxy, admin, patterns, backends)
- **PrismPattern**: Individual pattern runner with auto-scaling
- **PrismBackend**: Backend connection configuration
- **PrismNamespace**: Multi-tenant namespace provisioning

### Auto-Scaling Strategies

| Component | Scaler | Metrics | Use Case |
|-----------|--------|---------|----------|
| **Proxy** | HPA | Request rate, CPU | Client traffic |
| **Consumer** | KEDA | Queue depth, lag | Kafka/NATS consumption |
| **Producer** | HPA | CPU, throughput | Message production |
| **KeyValue** | HPA | Connections, CPU | Redis/cache operations |

## Prerequisites

- Kubernetes cluster (Docker Desktop, kind, or cloud provider)
- Go 1.21+
- kubectl
- Helm (for KEDA installation)

## Quick Start

### 1. Install Dependencies

```bash
# Install metrics-server (for HPA)
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml

# Install KEDA (for event-driven scaling)
helm repo add kedacore https://kedacore.github.io/charts
helm repo update
helm install keda kedacore/keda --namespace keda --create-namespace
```

Or use the Makefile:

```bash
make local-install-deps
```

### 2. Install CRDs

```bash
make install
```

### 3. Run Operator Locally

```bash
# Run against Docker Desktop Kubernetes
make local-run
```

### 4. Deploy Example Pattern

**HPA Example (CPU-based)**:

```bash
kubectl apply -f config/samples/prismpattern_hpa_example.yaml
```

**KEDA Example (Kafka lag-based)**:

```bash
kubectl apply -f config/samples/prismpattern_keda_kafka_example.yaml
```

**Multi-Trigger Example (Kafka + NATS + SQS + CPU)**:

```bash
kubectl apply -f config/samples/prismpattern_keda_multi_trigger_example.yaml
```

### 5. Watch Auto-Scaling

```bash
# Watch HPA
kubectl get hpa -w

# Watch KEDA ScaledObject
kubectl get scaledobject -w

# Watch pods scaling
kubectl get pods -l app=prism -w
```

## Examples

### HPA: CPU-Based Scaling (Producer)

```yaml
apiVersion: prism.io/v1alpha1
kind: PrismPattern
metadata:
  name: producer-kafka-events
spec:
  pattern: producer
  backend: kafka
  image: ghcr.io/prism/producer-runner:latest
  replicas: 2

  autoscaling:
    enabled: true
    scaler: hpa
    minReplicas: 2
    maxReplicas: 20
    targetCPUUtilizationPercentage: 75

    behavior:
      scaleDown:
        stabilizationWindowSeconds: 300
      scaleUp:
        stabilizationWindowSeconds: 0
```

### KEDA: Queue Depth Scaling (Consumer)

```yaml
apiVersion: prism.io/v1alpha1
kind: PrismPattern
metadata:
  name: consumer-kafka-orders
spec:
  pattern: consumer
  backend: kafka
  image: ghcr.io/prism/consumer-runner:latest
  replicas: 1

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
          consumerGroup: "prism-orders"
          topic: "orders"
          lagThreshold: "1000"
```

### Custom Metrics (Prometheus)

```yaml
autoscaling:
  enabled: true
  scaler: hpa
  minReplicas: 3
  maxReplicas: 20

  metrics:
    - type: Pods
      pods:
        metric:
          name: http_requests_per_second
        target:
          type: AverageValue
          averageValue: "1000"
```

## Development

### Build Operator

```bash
make build
```

### Run Tests

```bash
make test
```

### Generate Manifests

```bash
make manifests generate
```

### Build Docker Image

```bash
make docker-build IMG=prism-operator:dev
```

## Local Testing with Docker Desktop

### Complete Workflow

```bash
# 1. Install dependencies
make local-install-deps

# 2. Install CRDs
make install

# 3. Run operator locally
make local-run

# 4. In another terminal, deploy examples
make local-test-hpa      # HPA example
make local-test-keda     # KEDA example
make local-test-multi    # Multi-trigger example

# 5. Check status
make local-status

# 6. Clean up
make local-clean
```

### Manual Testing

```bash
# Deploy pattern
kubectl apply -f config/samples/prismpattern_hpa_example.yaml

# Watch auto-scaling
kubectl get hpa producer-kafka-events -w

# Generate load (simulate high CPU)
kubectl run load-generator --image=busybox --restart=Never -- /bin/sh -c \
  "while true; do wget -q -O- http://producer-kafka-events:8080/health; done"

# Watch pods scale up
kubectl get pods -l prism.io/pattern-instance=producer-kafka-events -w

# Clean up
kubectl delete pod load-generator
kubectl delete prismpattern producer-kafka-events
```

## KEDA Trigger Types

| Backend | Trigger Type | Metric | Example Threshold |
|---------|--------------|--------|-------------------|
| Kafka | `kafka` | Consumer lag | `lagThreshold: "1000"` |
| NATS | `nats-jetstream` | Pending messages | `lagThreshold: "100"` |
| AWS SQS | `aws-sqs-queue` | Queue depth | `queueLength: "1000"` |
| RabbitMQ | `rabbitmq` | Queue length | `queueLength: "500"` |
| Redis | `redis` | List length | `listLength: "1000"` |
| PostgreSQL | `postgresql` | Unprocessed rows | Custom query |

## Production Deployment

### Deploy Operator to Cluster

```bash
# Build and push image
make docker-build docker-push IMG=ghcr.io/prism/prism-operator:v0.1.0

# Deploy operator
kubectl apply -f config/rbac/
kubectl apply -f config/manager/manager.yaml
```

### Production Configuration

```yaml
apiVersion: prism.io/v1alpha1
kind: PrismPattern
metadata:
  name: consumer-production
spec:
  pattern: consumer
  backend: kafka
  replicas: 5

  autoscaling:
    enabled: true
    scaler: keda
    minReplicas: 5
    maxReplicas: 100
    pollingInterval: 10
    cooldownPeriod: 300

    triggers:
      - type: kafka
        metadata:
          bootstrapServers: "kafka-prod.svc:9092"
          consumerGroup: "prism-prod"
          topic: "orders"
          lagThreshold: "500"

  resources:
    requests:
      cpu: "2000m"
      memory: "4Gi"
    limits:
      cpu: "8000m"
      memory: "16Gi"

  placement:
    nodeSelector:
      workload: data-intensive
    affinity:
      podAntiAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchLabels:
                prism.io/pattern: consumer
            topologyKey: kubernetes.io/hostname
```

## Troubleshooting

### HPA Not Scaling

```bash
# Check HPA status
kubectl describe hpa <name>

# Check metrics server
kubectl top nodes
kubectl top pods

# Check HPA events
kubectl get events --field-selector involvedObject.name=<hpa-name>
```

### KEDA Not Scaling

```bash
# Check ScaledObject status
kubectl describe scaledobject <name>

# Check KEDA operator logs
kubectl logs -n keda deployment/keda-operator

# Check trigger authentication
kubectl get triggerauthentication
kubectl describe triggerauthentication <name>
```

### Pattern Not Reconciling

```bash
# Check operator logs
kubectl logs -n prism-system deployment/prism-operator

# Check PrismPattern status
kubectl describe prismpattern <name>

# Check deployment
kubectl get deployment <name>
kubectl describe deployment <name>
```

## Architecture Decisions

See [MEMO-036](../docs-cms/memos/MEMO-036-kubernetes-operator-development.md) for comprehensive architecture documentation.

## License

MIT

## Contributing

1. Fork the repository
2. Create feature branch
3. Add tests
4. Run `make test`
5. Create pull request
