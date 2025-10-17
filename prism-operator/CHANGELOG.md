# Changelog

All notable changes to the Prism Kubernetes Operator will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **KEDA Integration**: Full support for KEDA (Kubernetes Event-Driven Autoscaling)
  - Optional KEDA installer script (`scripts/install-keda.sh`)
  - Automatic KEDA scheme registration in operator
  - Support for 60+ KEDA scalers (Kafka, RabbitMQ, NATS, AWS SQS, etc.)
  - Example patterns with KEDA configuration
  - Makefile targets for KEDA management (`local-install-keda`, `local-uninstall-keda`, `local-keda-status`)

- **Enhanced Status Tracking**: Comprehensive PrismPattern status updates
  - Three-phase lifecycle: `Pending` → `Progressing` → `Running`
  - Replica count tracking (`replicas`, `availableReplicas`)
  - Kubernetes Conditions with detailed messages
  - ObservedGeneration tracking

- **Graceful Degradation**: Operator handles missing dependencies elegantly
  - Informative logging when KEDA CRDs are not installed (INFO level, not ERROR)
  - Patterns deploy successfully without autoscaling when KEDA is unavailable
  - Comprehensive error detection using both type assertions and string matching

- **Installation Script**: Production-ready KEDA installer
  - Multiple installation methods: Helm (default) or YAML manifests
  - Version control and namespace customization
  - Automatic verification of CRDs and deployments
  - Graceful upgrade support
  - Clean uninstall with CRD cleanup

- **Documentation**:
  - `QUICK_START.md` - Get started in 5 minutes
  - `KEDA_INSTALL_GUIDE.md` - Comprehensive KEDA installation and usage guide
  - `TEST_REPORT.md` - Detailed test results and verification
  - Example patterns for HPA and KEDA autoscaling

- **Makefile Improvements**:
  - Split metrics-server and KEDA installation into separate targets
  - Added Docker Desktop TLS patch for metrics-server
  - Added KEDA management targets
  - Improved target organization and documentation

### Fixed
- **KEDA Error Handling**: Fixed errors when KEDA CRDs are not installed
  - Added `isKEDANotInstalledError()` helper function to detect missing KEDA CRDs
  - Applied graceful handling to all KEDA reconciliation paths (Get, Create, Delete)
  - Changed log level from ERROR to INFO for missing KEDA CRDs
  - Prevents reconciliation failures when KEDA is not needed

- **Status Update Robustness**: Improved error handling in status updates
  - Handle deployment not found gracefully (early in reconciliation)
  - Better phase detection logic
  - Improved condition messages with replica counts
  - Added verbose logging for troubleshooting

### Changed
- **Operator Startup**: Added KEDA scheme registration to manager initialization
  - Imports `kedav1alpha1` package
  - Registers KEDA scheme in init function
  - Enables KEDA CRD operations when KEDA is installed

- **Controller Logging**: Enhanced logging throughout reconciliation loop
  - All major reconciliation steps now logged at INFO level
  - Status updates logged at DEBUG/INFO with details
  - Better error context in log messages

- **Autoscaling Architecture**: Improved separation of concerns
  - HPA and KEDA reconcilers cleanly separated
  - Automatic cleanup when switching scaler types
  - Independent installation of autoscaling dependencies

## [0.1.0] - 2025-10-17

### Added
- Initial release of Prism Kubernetes Operator
- PrismPattern CRD for managing pattern deployments
- HPA (Horizontal Pod Autoscaler) support for CPU/memory-based scaling
- Automatic Deployment and Service creation from PrismPattern
- Owner references for cascade deletion
- Docker Desktop Kubernetes compatibility
- Comprehensive test coverage on local Docker Desktop cluster
- Basic Makefile with development targets
- go.mod with controller-runtime v0.16.3

### Known Limitations
- PrismStack CRD not yet implemented (DeepCopy methods incomplete)
- KEDA AuthenticationRef support incomplete (type mismatch with v2.12 API)
- No container image published yet (run from source only)
- No Helm chart yet (manual kubectl apply required)

## Release Notes

### Breaking Changes
None - this is the initial release.

### Migration Guide
N/A - initial release.

### Deprecations
None.

### Security
- All resources created with proper owner references
- RBAC manifests included in CRD definitions
- No secrets or credentials stored in operator

## Links
- [Quick Start Guide](QUICK_START.md)
- [KEDA Installation Guide](KEDA_INSTALL_GUIDE.md)
- [Test Report](TEST_REPORT.md)
- [Examples](config/samples/)
