# Prism Operator Makefile

# Image URL to use all building/pushing image targets
IMG ?= ghcr.io/prism/prism-operator:latest

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet ## Run tests.
	go test ./... -coverprofile cover.out

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/manager/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/manager/main.go

.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	kubectl apply -f config/crd/bases

.PHONY: uninstall
uninstall: manifests ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	kubectl delete --ignore-not-found=$(ignore-not-found) -f config/crd/bases

.PHONY: deploy
deploy: manifests ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && kubectl apply -f manager.yaml

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	cd config/manager && kubectl delete --ignore-not-found=$(ignore-not-found) -f manager.yaml

##@ Local Development with Docker Desktop

.PHONY: local-install-deps
local-install-deps: ## Install required dependencies (metrics-server, KEDA)
	@echo "Installing metrics-server..."
	kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
	@echo "Installing KEDA..."
	helm repo add kedacore https://kedacore.github.io/charts
	helm repo update
	helm install keda kedacore/keda --namespace keda --create-namespace
	@echo "Verifying installations..."
	kubectl get deployment metrics-server -n kube-system
	kubectl get pods -n keda

.PHONY: local-run
local-run: install ## Run operator locally against Docker Desktop k8s
	kubectl config use-context docker-desktop
	go run ./cmd/manager/main.go

.PHONY: local-test-hpa
local-test-hpa: ## Deploy HPA example to local cluster
	kubectl apply -f config/samples/prismpattern_hpa_example.yaml
	@echo "Watching HPA..."
	kubectl get hpa -w

.PHONY: local-test-keda
local-test-keda: ## Deploy KEDA example to local cluster
	kubectl apply -f config/samples/prismpattern_keda_kafka_example.yaml
	@echo "Watching ScaledObject..."
	kubectl get scaledobject -w

.PHONY: local-test-multi
local-test-multi: ## Deploy multi-trigger KEDA example
	kubectl apply -f config/samples/prismpattern_keda_multi_trigger_example.yaml
	@echo "Watching ScaledObject..."
	kubectl get scaledobject consumer-multi-source -w

.PHONY: local-clean
local-clean: ## Clean up local cluster
	kubectl delete prismpattern --all
	kubectl delete hpa --all
	kubectl delete scaledobject --all

.PHONY: local-status
local-status: ## Show status of all Prism resources
	@echo "=== PrismPatterns ==="
	kubectl get prismpattern
	@echo ""
	@echo "=== HPAs ==="
	kubectl get hpa
	@echo ""
	@echo "=== KEDA ScaledObjects ==="
	kubectl get scaledobject
	@echo ""
	@echo "=== Deployments ==="
	kubectl get deployments -l app=prism
	@echo ""
	@echo "=== Pods ==="
	kubectl get pods -l app=prism

##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen

## Tool Versions
CONTROLLER_TOOLS_VERSION ?= v0.13.0

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	test -s $(LOCALBIN)/controller-gen || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)
