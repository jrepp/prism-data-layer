#!/usr/bin/env bash
set -euo pipefail

# install-keda.sh - Install KEDA for event-driven autoscaling
# Works with any Kubernetes cluster including Docker Desktop

KEDA_VERSION="${KEDA_VERSION:-2.12.1}"
KEDA_NAMESPACE="${KEDA_NAMESPACE:-keda}"
INSTALL_METHOD="${INSTALL_METHOD:-helm}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_prerequisites() {
    log_info "Checking prerequisites..."

    # Check kubectl
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl not found. Please install kubectl first."
        exit 1
    fi

    # Check cluster connectivity
    if ! kubectl cluster-info &> /dev/null; then
        log_error "Cannot connect to Kubernetes cluster. Check your kubeconfig."
        exit 1
    fi

    # Check Helm if using helm method
    if [[ "$INSTALL_METHOD" == "helm" ]]; then
        if ! command -v helm &> /dev/null; then
            log_error "Helm not found. Please install Helm or use INSTALL_METHOD=yaml"
            exit 1
        fi
    fi

    log_info "Prerequisites check passed"
}

check_existing_installation() {
    log_info "Checking for existing KEDA installation..."

    if kubectl get namespace "$KEDA_NAMESPACE" &> /dev/null; then
        log_warn "KEDA namespace '$KEDA_NAMESPACE' already exists"

        # Check if KEDA is actually installed
        if kubectl get deployment -n "$KEDA_NAMESPACE" keda-operator &> /dev/null; then
            log_warn "KEDA operator is already installed"
            read -p "Do you want to upgrade/reinstall? (y/N) " -n 1 -r
            echo
            if [[ ! $REPLY =~ ^[Yy]$ ]]; then
                log_info "Installation cancelled"
                exit 0
            fi
            return 1  # Upgrade mode
        fi
    fi
    return 0  # Fresh install
}

install_keda_helm() {
    log_info "Installing KEDA using Helm..."

    # Add KEDA Helm repository
    log_info "Adding KEDA Helm repository..."
    helm repo add kedacore https://kedacore.github.io/charts
    helm repo update

    # Check if release exists
    if helm list -n "$KEDA_NAMESPACE" | grep -q keda; then
        log_info "Upgrading existing KEDA installation..."
        helm upgrade keda kedacore/keda \
            --namespace "$KEDA_NAMESPACE" \
            --version "$KEDA_VERSION" \
            --wait
    else
        log_info "Installing KEDA version $KEDA_VERSION..."
        helm install keda kedacore/keda \
            --namespace "$KEDA_NAMESPACE" \
            --create-namespace \
            --version "$KEDA_VERSION" \
            --wait
    fi

    log_info "KEDA installed successfully via Helm"
}

install_keda_yaml() {
    log_info "Installing KEDA using YAML manifests..."

    # Create namespace if it doesn't exist
    kubectl create namespace "$KEDA_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

    # Install KEDA CRDs and operator
    log_info "Applying KEDA manifests version $KEDA_VERSION..."
    kubectl apply -f "https://github.com/kedacore/keda/releases/download/v${KEDA_VERSION}/keda-${KEDA_VERSION}.yaml"

    log_info "KEDA installed successfully via YAML"
}

verify_installation() {
    log_info "Verifying KEDA installation..."

    # Wait for KEDA operator to be ready
    log_info "Waiting for KEDA operator to be ready..."
    kubectl wait --for=condition=available \
        --timeout=120s \
        deployment/keda-operator \
        -n "$KEDA_NAMESPACE" || {
        log_error "KEDA operator failed to become ready"
        exit 1
    }

    # Wait for KEDA metrics server to be ready
    log_info "Waiting for KEDA metrics server to be ready..."
    kubectl wait --for=condition=available \
        --timeout=120s \
        deployment/keda-operator-metrics-apiserver \
        -n "$KEDA_NAMESPACE" || {
        log_error "KEDA metrics server failed to become ready"
        exit 1
    }

    # Check CRDs
    log_info "Checking KEDA CRDs..."
    local crds=("scaledobjects.keda.sh" "scaledjobs.keda.sh" "triggerauthentications.keda.sh")
    for crd in "${crds[@]}"; do
        if kubectl get crd "$crd" &> /dev/null; then
            log_info "  ✓ $crd"
        else
            log_error "  ✗ $crd not found"
            exit 1
        fi
    done

    log_info "KEDA installation verified successfully!"
}

show_status() {
    log_info "KEDA Status:"
    echo ""
    kubectl get pods -n "$KEDA_NAMESPACE"
    echo ""

    log_info "KEDA version:"
    kubectl get deployment keda-operator -n "$KEDA_NAMESPACE" -o jsonpath='{.spec.template.spec.containers[0].image}' | sed 's/.*://'
    echo ""

    log_info "Available CRDs:"
    kubectl get crd | grep keda.sh
    echo ""
}

uninstall_keda() {
    log_warn "Uninstalling KEDA..."

    if [[ "$INSTALL_METHOD" == "helm" ]]; then
        if helm list -n "$KEDA_NAMESPACE" | grep -q keda; then
            log_info "Uninstalling KEDA Helm release..."
            helm uninstall keda -n "$KEDA_NAMESPACE"
        fi
    else
        log_info "Deleting KEDA resources..."
        kubectl delete -f "https://github.com/kedacore/keda/releases/download/v${KEDA_VERSION}/keda-${KEDA_VERSION}.yaml" --ignore-not-found
    fi

    # Clean up namespace
    log_info "Deleting KEDA namespace..."
    kubectl delete namespace "$KEDA_NAMESPACE" --ignore-not-found

    # Clean up CRDs
    log_info "Cleaning up KEDA CRDs..."
    kubectl delete crd scaledobjects.keda.sh --ignore-not-found
    kubectl delete crd scaledjobs.keda.sh --ignore-not-found
    kubectl delete crd triggerauthentications.keda.sh --ignore-not-found
    kubectl delete crd clustertriggerauthentications.keda.sh --ignore-not-found

    log_info "KEDA uninstalled successfully"
}

show_usage() {
    cat << EOF
Usage: $0 [COMMAND] [OPTIONS]

Install KEDA (Kubernetes Event-Driven Autoscaling) for Prism Operator

Commands:
    install     Install KEDA (default)
    upgrade     Upgrade existing KEDA installation
    uninstall   Remove KEDA from cluster
    status      Show KEDA installation status
    help        Show this help message

Environment Variables:
    KEDA_VERSION      KEDA version to install (default: 2.12.1)
    KEDA_NAMESPACE    Namespace for KEDA (default: keda)
    INSTALL_METHOD    Installation method: helm or yaml (default: helm)

Examples:
    # Install KEDA using Helm (default)
    $0 install

    # Install specific version using YAML manifests
    KEDA_VERSION=2.11.0 INSTALL_METHOD=yaml $0 install

    # Check installation status
    $0 status

    # Uninstall KEDA
    $0 uninstall

For more information, see: https://keda.sh/docs/
EOF
}

main() {
    local command="${1:-install}"

    case "$command" in
        install|upgrade)
            check_prerequisites
            check_existing_installation

            if [[ "$INSTALL_METHOD" == "helm" ]]; then
                install_keda_helm
            else
                install_keda_yaml
            fi

            verify_installation
            show_status

            log_info ""
            log_info "✓ KEDA is ready!"
            log_info "You can now use KEDA scalers in your PrismPattern resources."
            log_info ""
            log_info "Example:"
            log_info "  kubectl apply -f config/samples/prismpattern_keda_kafka_example.yaml"
            ;;

        uninstall)
            uninstall_keda
            ;;

        status)
            if kubectl get namespace "$KEDA_NAMESPACE" &> /dev/null; then
                show_status
            else
                log_warn "KEDA is not installed (namespace '$KEDA_NAMESPACE' not found)"
                exit 1
            fi
            ;;

        help|--help|-h)
            show_usage
            ;;

        *)
            log_error "Unknown command: $command"
            show_usage
            exit 1
            ;;
    esac
}

main "$@"
