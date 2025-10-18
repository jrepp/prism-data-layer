# KEDA Installation Guide for Prism Operator

This guide explains how to install KEDA (Kubernetes Event-Driven Autoscaling) for use with the Prism Kubernetes Operator on Docker Desktop or any Kubernetes cluster.

## Quick Start

### Install KEDA (Recommended: Helm)

```bash
make local-install-keda
```

This uses the included installation script with Helm (default method).

### Alternative: Install via YAML manifests

```bash
INSTALL_METHOD=yaml make local-install-keda
```

### Check KEDA Status

```bash
make local-keda-status
```

### Uninstall KEDA

```bash
make local-uninstall-keda
```

## Installation Script

The `scripts/install-keda.sh` script provides a comprehensive installer with the following features:

- **Multiple installation methods**: Helm (default) or YAML manifests
- **Version control**: Specify KEDA version via `KEDA_VERSION` env var (default: 2.12.1)
- **Namespace customization**: Change namespace with `KEDA_NAMESPACE` (default: keda)
- **Verification**: Automatic verification of CRDs, deployments, and readiness
- **Graceful upgrades**: Detects existing installation and prompts for upgrade
- **Clean uninstall**: Removes all KEDA resources including CRDs

### Script Usage

```bash
# Install KEDA
./scripts/install-keda.sh install

# Install specific version
KEDA_VERSION=2.11.0 ./scripts/install-keda.sh install

# Use YAML manifests instead of Helm
INSTALL_METHOD=yaml ./scripts/install-keda.sh install

# Check status
./scripts/install-keda.sh status

# Uninstall
./scripts/install-keda.sh uninstall

# Show help
./scripts/install-keda.sh help
```

## What Gets Installed

KEDA installation includes:

1. **KEDA Operator** - Main controller managing ScaledObjects
2. **KEDA Metrics Server** - External metrics API for HPA
3. **KEDA Admission Controller** - Webhook validation for ScaledObjects
4. **Custom Resource Definitions (CRDs)**:
   - `scaledobjects.keda.sh` - Scale configuration for deployments
   - `scaledjobs.keda.sh` - Scale configuration for jobs
   - `triggerauthentications.keda.sh` - Authentication for scalers
   - `clustertriggerauthentications.keda.sh` - Cluster-wide authentication

## Using KEDA with Prism Operator

### Example: CPU-based Autoscaling

```yaml
apiVersion: prism.io/v1alpha1
kind: PrismPattern
metadata:
  name: my-pattern
  namespace: prism-system
spec:
  pattern: keyvalue
  backend: redis
  image: ghcr.io/prism/keyvalue-runner:latest
  replicas: 1

  autoscaling:
    enabled: true
    scaler: keda  # Use KEDA instead of HPA

    minReplicas: 1
    maxReplicas: 10

    pollingInterval: 10    # Check every 10 seconds
    cooldownPeriod: 30     # Wait 30s before scaling down

    triggers:
      - type: cpu
        metadata:
          type: Utilization
          value: "50"      # Scale when CPU > 50%
```

### Example: Kafka Consumer Lag

```yaml
apiVersion: prism.io/v1alpha1
kind: PrismPattern
metadata:
  name: orders-consumer
  namespace: prism-system
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
          lagThreshold: "1000"  # Scale when lag > 1000 messages
```

## Verification

After installation, verify KEDA is working:

```bash
# Check KEDA pods
kubectl get pods -n keda

# Check CRDs
kubectl get crd | grep keda.sh

# Deploy a KEDA-enabled pattern
kubectl apply -f config/samples/prismpattern_keda_kafka_example.yaml

# Watch ScaledObject
kubectl get scaledobject -w
```

## Troubleshooting

### KEDA CRDs Not Found

If you see errors like `"KEDA not installed: please install KEDA CRDs"`:

1. Verify KEDA is installed: `./scripts/install-keda.sh status`
2. Check CRDs exist: `kubectl get crd | grep keda.sh`
3. If missing, reinstall: `./scripts/install-keda.sh install`

The Prism operator gracefully handles missing KEDA CRDs - patterns will be created without autoscaling if KEDA is not installed.

### KEDA Operator Not Ready

```bash
# Check operator logs
kubectl logs -n keda deployment/keda-operator

# Restart operator
kubectl rollout restart deployment/keda-operator -n keda
```

### ScaledObject Not Scaling

```bash
# Check ScaledObject status
kubectl describe scaledobject <name> -n <namespace>

# Check HPA created by KEDA
kubectl get hpa

# Check KEDA operator logs
kubectl logs -n keda deployment/keda-operator -f
```

## Docker Desktop Specific Notes

### Metrics Server Required

For CPU/memory-based scaling on Docker Desktop, you need metrics-server:

```bash
make local-install-metrics
```

This installs and patches metrics-server to work with Docker Desktop's TLS certificates.

### Resource Limits

Docker Desktop default resources may be insufficient for KEDA + workloads:

- **Recommended**: 4 CPU, 8GB RAM minimum
- **Settings**: Docker Desktop → Settings → Resources

## Makefile Targets

```bash
# Install metrics-server only
make local-install-metrics

# Install KEDA only
make local-install-keda

# Install both (metrics-server + KEDA)
make local-install-deps

# Check KEDA status
make local-keda-status

# Uninstall KEDA
make local-uninstall-keda

# Test KEDA with examples
make local-test-keda
make local-test-multi
```

## Advanced Configuration

### Custom KEDA Version

```bash
KEDA_VERSION=2.13.0 ./scripts/install-keda.sh install
```

### Custom Namespace

```bash
KEDA_NAMESPACE=custom-keda ./scripts/install-keda.sh install
```

### Helm Values

For advanced Helm configuration, modify the script or use Helm directly:

```bash
helm install keda kedacore/keda \
  --namespace keda \
  --create-namespace \
  --set resources.operator.limits.memory=2Gi \
  --set resources.metricServer.limits.memory=1Gi
```

## Supported Scalers

KEDA supports 60+ scalers including:

- **Messaging**: Kafka, RabbitMQ, NATS, AWS SQS, Azure Service Bus
- **Databases**: PostgreSQL, MySQL, Redis, MongoDB
- **Cloud Services**: AWS CloudWatch, GCP Pub/Sub, Azure Monitor
- **Metrics**: Prometheus, Datadog, New Relic
- **System**: CPU, Memory, Cron

See [KEDA Scalers Documentation](https://keda.sh/docs/scalers/) for complete list.

## Production Considerations

### High Availability

For production, run KEDA operator with multiple replicas:

```bash
helm upgrade keda kedacore/keda \
  --namespace keda \
  --set operator.replicaCount=2
```

### Resource Limits

Set appropriate resource limits in production:

```yaml
resources:
  operator:
    limits:
      cpu: 1000m
      memory: 1000Mi
    requests:
      cpu: 100m
      memory: 100Mi
```

### Monitoring

KEDA exposes Prometheus metrics on `:8080/metrics`:

- `keda_scaler_active` - Scaler active status
- `keda_scaler_errors_total` - Scaler error count
- `keda_scaled_object_errors` - ScaledObject errors

## References

- [KEDA Documentation](https://keda.sh/docs/)
- [KEDA Scalers](https://keda.sh/docs/scalers/)
- [KEDA Concepts](https://keda.sh/docs/concepts/)
- [Prism Operator Examples](config/samples/)

## Support

For issues related to:
- **KEDA installation**: Check [KEDA Troubleshooting](https://keda.sh/docs/troubleshooting/)
- **Prism operator integration**: Open issue in Prism operator repository
- **ScaledObject configuration**: See example patterns in `config/samples/`
