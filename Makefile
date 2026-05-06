# Image URL to use all building/pushing image targets
IMG ?= controller:latest

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= finch
BUILD_OPTS :=
CLOUD_PROVIDER :=

# Traefik CRD chart version — pinned because latest (1.16.0+) exceeds the 1MB
# Kubernetes Secret size limit for Helm release metadata.
TRAEFIK_CRD_CHART_VERSION ?= 1.15.0

# Use Finch as the container provider for Kind when using Finch
# Update goproxy for cloud desktop compatibility
ifeq ($(CONTAINER_TOOL),finch)
  export KIND_EXPERIMENTAL_PROVIDER=finch
  export GOPROXY=direct

  # Set BUILD_OPTS to '--network host' on cloud desktop (if /etc/os-release exists), otherwise empty
  # You might have to comment BUILD_OPTS out for devdesktop
  BUILD_OPTS := $(shell if [ -f /etc/os-release ]; then echo "--network host"; else echo ""; fi)
endif

# Remote cluster configuration
# AWS_REGION and EKS_CLUSTER_NAME resolution order (highest priority first):
#   1. Command line: make setup-aws AWS_REGION=us-east-2 EKS_CLUSTER_NAME=my-cluster
#   2. .env file: AWS_REGION=us-east-2 (overrides shell environment)
#   3. Defaults: us-west-2 / jupyter-k8s-cluster
ifeq ($(CLOUD_PROVIDER),aws)
	# Read .env values (override env vars, but command-line args still win)
	ifneq (,$(wildcard .env))
		ifneq ($(origin AWS_REGION),command line)
			_ENV_AWS_REGION := $(shell grep -s '^AWS_REGION=' .env | cut -d= -f2)
			ifneq (,$(_ENV_AWS_REGION))
				AWS_REGION := $(_ENV_AWS_REGION)
			endif
		endif
		ifneq ($(origin EKS_CLUSTER_NAME),command line)
			_ENV_EKS_CLUSTER := $(shell grep -s '^EKS_CLUSTER_NAME=' .env | cut -d= -f2)
			ifneq (,$(_ENV_EKS_CLUSTER))
				EKS_CLUSTER_NAME := $(_ENV_EKS_CLUSTER)
			endif
		endif
	endif
	AWS_REGION ?= us-west-2
	EKS_CLUSTER_NAME ?= jupyter-k8s-cluster
	AWS_ACCOUNT_ID := $(shell aws sts get-caller-identity --query "Account" --output text)
	ECR_REGISTRY := $(AWS_ACCOUNT_ID).dkr.ecr.$(AWS_REGION).amazonaws.com
	ECR_REPOSITORY := jupyter-k8s
	ECR_REPOSITORY_AUTH := jupyter-k8s-auth
	ECR_REPOSITORY_ROTATOR := jupyter-k8s-rotator
	EKS_CONTEXT := arn:aws:eks:$(AWS_REGION):$(AWS_ACCOUNT_ID):cluster/$(EKS_CLUSTER_NAME)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

.PHONY: release
release: helm-generate build lint-fix lint-fix-e2e test helm-lint helm-test ## Run all checks required before PR submission (excluding e2e tests)

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)


.PHONY: deps
deps:
	go mod download
	go mod tidy

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases
	# Remove connection.workspace.jupyter.org CRDs as they're meant to be subresources, not standalone CRDs
	rm -f config/crd/bases/connection.workspace.jupyter.org_*.yaml

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
test: manifests generate fmt vet setup-envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $$(go list ./... | grep -v /e2e | grep -v /test/helm) -coverprofile cover.out

# TODO(user): To use a different vendor for e2e tests, modify the setup under 'tests/e2e'.
# The default setup assumes Kind is pre-installed and builds/loads the Manager Docker image locally.
# CertManager is installed by default; skip with:
# CERT_MANAGER_INSTALL_SKIP=true

# Cluster names for different environments

# Used for automated e2e tests
KIND_CLUSTER ?= jupyter-k8s-test-e2e
E2E_MANAGER_IMAGE ?= jupyter.org/jupyter-k8s:v0.0.1

# Used for manual development
DEV_KIND_CLUSTER ?= jupyter-k8s-dev

# Set this variable to true to use local kind cluster for deployment
USE_KIND ?= false

.PHONY: setup-test-e2e
setup-test-e2e: ## Set up a fresh Kind cluster for e2e tests (deletes existing cluster first)
	@command -v $(KIND) >/dev/null 2>&1 || { \
		echo "Kind is not installed. Please install Kind manually."; \
		exit 1; \
	}
	@case "$$($(KIND) get clusters)" in \
		*"$(KIND_CLUSTER)"*) \
			echo "Deleting existing Kind cluster '$(KIND_CLUSTER)' for a fresh start..."; \
			$(KIND) delete cluster --name $(KIND_CLUSTER) ;; \
	esac
	@echo "Creating Kind cluster '$(KIND_CLUSTER)'..."
	@$(KIND) create cluster --name $(KIND_CLUSTER)
	@if ! kubectl get namespace cert-manager > /dev/null 2>&1; then \
		echo "Installing cert-manager"; \
		helm repo add jetstack https://charts.jetstack.io; \
		helm repo update; \
		helm install cert-manager jetstack/cert-manager \
			--namespace cert-manager \
			--create-namespace \
			--set installCRDs=true; \
		echo "Waiting for cert-manager to be ready..."; \
		kubectl wait --for=condition=Available --timeout=300s deployment/cert-manager-webhook -n cert-manager; \
	else \
		echo "cert-manager is already installed, skipping installation"; \
	fi

.PHONY: test-e2e
test-e2e: setup-test-e2e manifests generate fmt vet helm-generate load-images-e2e ## Run the e2e tests. Expected an isolated environment using Kind.
	KIND_CLUSTER=$(KIND_CLUSTER) CONTAINER_TOOL=$(CONTAINER_TOOL) go test -tags=e2e ./test/e2e/ -v -timeout 60m -ginkgo.v -ginkgo.timeout 60m
	$(MAKE) cleanup-test-e2e

.PHONY: cleanup-test-e2e
cleanup-test-e2e: ## Tear down the Kind cluster used for e2e tests
	$(KIND) delete cluster --name=$(KIND_CLUSTER) || { \
		echo "Failed to delete cluster normally. Attempting manual cleanup..."; \
		echo "Removing container $(KIND_CLUSTER)-control-plane directly..."; \
		$(CONTAINER_TOOL) rm -f $(KIND_CLUSTER)-control-plane || true; \
		echo "Retrying cluster deletion..."; \
		$(KIND) delete cluster --name=$(KIND_CLUSTER) || echo "Cluster deletion completed with manual cleanup"; \
	}

.PHONY: cleanup-test-e2e-images
cleanup-test-e2e-images: ## Remove e2e test images to force rebuild
	@echo "Removing e2e test images..."
	@$(CONTAINER_TOOL) rmi jupyter.org/jupyter-k8s:v0.0.1 2>/dev/null || true
	@$(MAKE) -C images clean CONTAINER_TOOL=$(CONTAINER_TOOL)

.PHONY: test-e2e-clean
test-e2e-clean: cleanup-test-e2e cleanup-test-e2e-images test-e2e ## Full cleanup and rerun e2e tests

.PHONY: lint
lint: golangci-lint helm-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-e2e
lint-e2e: golangci-lint ## Run golangci-lint linter on e2e tests
	GOFLAGS="-tags=e2e" $(GOLANGCI_LINT) run ./test/e2e/...

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

.PHONY: lint-fix-e2e
lint-fix-e2e: golangci-lint ## Run golangci-lint linter on e2e tests and perform fixes
	GOFLAGS="-tags=e2e" $(GOLANGCI_LINT) run --fix ./test/e2e/...
	
.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	$(GOLANGCI_LINT) config verify

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: build-e2e
build-e2e: manifests generate fmt vet
	go build -tags=e2e ./test/e2e/...

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build $(BUILD_OPTS) -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name jupyter-k8s-builder
	$(CONTAINER_TOOL) buildx use jupyter-k8s-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm jupyter-k8s-builder
	rm Dockerfile.cross

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > dist/install.yaml

##@ Helm Chart
.PHONY: helm-generate
helm-generate: manifests
	rm -rf dist/chart
	kubebuilder edit --plugins=helm/v2-alpha --force
	./hack/apply-helm-patches.sh

.PHONY: helm-package
helm-package: helm-generate ## Package the Helm chart
	helm package dist/chart -d dist

.PHONY: helm-lint
helm-lint: ## Lint the Helm chart
	helm lint dist/chart

.PHONY: helm-test
helm-test: ## Test the Helm chart with helm template
	rm -rf dist/test-output-crd-only
	rm -rf /tmp/helm-test-chart
	cp -r dist/chart /tmp/helm-test-chart
	cd /tmp/helm-test-chart && helm dependency build
	helm template $(HELM_RELEASE) /tmp/helm-test-chart --output-dir dist/test-output-crd-only \
		--set accessResources.traefik.enable=true \
		--set extensionApi.enable=true
	rm -rf /tmp/helm-test-chart
	go test ./test/helm/crd-only -v


##@ Deployment
ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	@echo "Using kubectl context: $$(kubectl config current-context)"
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -
	@echo "Applying extension API auth RoleBinding in kube-system (not managed by kustomize)..."
	$(KUBECTL) apply -f config/rbac/extension_api_auth_binding.yaml
	@if [ "$(USE_KIND)" = "true" ]; then \
		echo "Using Kind cluster mode, patching deployment for local images..."; \
		$(MAKE) load-images; \
		$(KUBECTL) patch deployment jupyter-k8s-controller-manager -n jupyter-k8s-system --type=json \
			-p='[{"op": "add", "path": "/spec/template/spec/containers/0/imagePullPolicy", "value": "Never"}]'; \
	fi

.PHONY: setup-kind
setup-kind: ## Set up a Kind cluster for development if it does not exist
	@command -v $(KIND) >/dev/null 2>&1 || { \
		echo "Kind is not installed. Please install Kind manually."; \
		exit 1; \
	}
	@case "$$($(KIND) get clusters)" in \
		*"$(DEV_KIND_CLUSTER)"*) \
			echo "Kind cluster '$(DEV_KIND_CLUSTER)' already exists. Skipping creation." ;; \
		*) \
			echo "Creating Kind cluster '$(DEV_KIND_CLUSTER)'..."; \
			$(KIND) create cluster --name $(DEV_KIND_CLUSTER) ;; \
	esac
	@echo "Setting kubectl context to kind-$(DEV_KIND_CLUSTER)..."
	kubectl config use-context kind-$(DEV_KIND_CLUSTER)
	@if ! $(CONTAINER_TOOL) ps | grep -q "registry:2"; then \
		echo "Setting up local Docker registry..."; \
		$(CONTAINER_TOOL) run -d --restart=always -p 5000:5000 --name registry registry:2; \
	else \
		echo "Local registry is already running."; \
	fi
	@if ! kubectl get namespace cert-manager > /dev/null 2>&1; then \
		echo "Installing cert-manager"; \
		helm repo add jetstack https://charts.jetstack.io; \
		helm repo update; \
		helm install cert-manager jetstack/cert-manager \
			--namespace cert-manager \
			--create-namespace \
			--set installCRDs=true; \
		echo "Waiting for cert-manager to be ready..."; \
		kubectl wait --for=condition=Available --timeout=300s deployment/cert-manager-webhook -n cert-manager; \
	else \
		echo "cert-manager is already installed, skipping installation"; \
	fi
	@if ! kubectl get crds ingressroutes.traefik.io > /dev/null 2>&1; then \
		echo "Installing Traefik CRDs"; \
		helm repo add traefik https://traefik.github.io/charts; \
		helm install traefik-crd traefik/traefik-crds \
			--namespace traefik \
			--create-namespace \
			--version $(TRAEFIK_CRD_CHART_VERSION); \
	else \
		echo "Traefik CRDs are already installed, skipping installation"; \
	fi

.PHONY: test-e2e-focus
test-e2e-focus: setup-test-e2e manifests generate fmt vet helm-generate load-images-e2e ## Run specific e2e tests using FOCUS parameter. Usage: make test-e2e-focus FOCUS="Primary Storage"
	@if [ -z "$(FOCUS)" ]; then \
		echo "Error: FOCUS parameter is required. Usage: make test-e2e-focus FOCUS=\"Primary Storage\""; \
		exit 1; \
	fi
	KIND_CLUSTER=$(KIND_CLUSTER) CONTAINER_TOOL=$(CONTAINER_TOOL) go test -tags=e2e ./test/e2e/ -v -timeout 60m -ginkgo.v -ginkgo.focus="$(FOCUS)" -ginkgo.timeout 60m
	$(MAKE) cleanup-test-e2e

STAGING_REGISTRY ?= ghcr.io/jupyter-infra/staging
STAGING_TAG ?=
STAGING_CHART_VERSION ?=

.PHONY: test-e2e-staging
test-e2e-staging: setup-test-e2e ## Run e2e tests against staging GHCR images and chart. Set STAGING_TAG and STAGING_CHART_VERSION.
	@if [ -z "$(STAGING_TAG)" ] || [ -z "$(STAGING_CHART_VERSION)" ]; then \
		echo "Error: STAGING_TAG and STAGING_CHART_VERSION are required."; \
		echo "Usage: make test-e2e-staging STAGING_TAG=v0.1.0-rc.1 STAGING_CHART_VERSION=0.1.0-rc.1"; \
		exit 1; \
	fi
	@echo "Pulling staging images..."
	$(CONTAINER_TOOL) pull $(STAGING_REGISTRY)/jupyter-k8s-controller:$(STAGING_TAG)
	$(CONTAINER_TOOL) pull $(STAGING_REGISTRY)/jupyter-k8s-rotator:$(STAGING_TAG)
	@echo "Loading staging images into e2e cluster $(KIND_CLUSTER)..."
	@mkdir -p /tmp/kind-images
	$(CONTAINER_TOOL) save $(STAGING_REGISTRY)/jupyter-k8s-controller:$(STAGING_TAG) -o /tmp/kind-images/staging-controller.tar
	$(KIND) load image-archive /tmp/kind-images/staging-controller.tar --name $(KIND_CLUSTER)
	rm -f /tmp/kind-images/staging-controller.tar
	$(CONTAINER_TOOL) save $(STAGING_REGISTRY)/jupyter-k8s-rotator:$(STAGING_TAG) -o /tmp/kind-images/staging-rotator.tar
	$(KIND) load image-archive /tmp/kind-images/staging-rotator.tar --name $(KIND_CLUSTER)
	rm -f /tmp/kind-images/staging-rotator.tar
	@echo "Loading application images into e2e cluster $(KIND_CLUSTER)..."
	$(MAKE) -C images push-all-kind CLUSTER_NAME=$(KIND_CLUSTER) CONTAINER_TOOL=$(CONTAINER_TOOL)
	@echo "Running e2e tests against staging artifacts..."
	KIND_CLUSTER=$(KIND_CLUSTER) CONTAINER_TOOL=$(CONTAINER_TOOL) \
		E2E_MANAGER_IMAGE=$(STAGING_REGISTRY)/jupyter-k8s-controller:$(STAGING_TAG) \
		E2E_ROTATOR_IMAGE=$(STAGING_REGISTRY)/jupyter-k8s-rotator:$(STAGING_TAG) \
		E2E_CHART_SOURCE=oci://$(STAGING_REGISTRY)/charts/jupyter-k8s \
		E2E_CHART_VERSION=$(STAGING_CHART_VERSION) \
		go test -tags=e2e ./test/e2e/ -v -timeout 60m -ginkgo.v -ginkgo.timeout 60m
	$(MAKE) cleanup-test-e2e

.PHONY: teardown-kind
teardown-kind: ## Tear down the Kind cluster, registry, and clean up images
	# Delete the Kind cluster
	$(KIND) delete cluster --name=$(DEV_KIND_CLUSTER) || { \
		echo "Failed to delete cluster normally. Attempting manual cleanup..."; \
		echo "Removing container $(DEV_KIND_CLUSTER)-control-plane directly..."; \
		$(CONTAINER_TOOL) rm -f $(DEV_KIND_CLUSTER)-control-plane || true; \
		echo "Retrying cluster deletion..."; \
		$(KIND) delete cluster --name=$(DEV_KIND_CLUSTER) || echo "Cluster deletion completed with manual cleanup"; \
	}
	# Stop and remove registry container if running
	$(CONTAINER_TOOL) stop registry || true
	$(CONTAINER_TOOL) rm registry || true
	# Clean up images from Finch cache
	@echo "Cleaning up images from cache..."
	$(CONTAINER_TOOL) rmi ${IMG} || true
	$(MAKE) -C images clean CONTAINER_TOOL=$(CONTAINER_TOOL)

.PHONY: load-images
load-images: docker-build build-rotator ## Build and load images into the Kind cluster
	@echo "Loading controller image ${IMG} into kind cluster ${DEV_KIND_CLUSTER}..."
	@mkdir -p /tmp/kind-images
	$(CONTAINER_TOOL) save ${IMG} -o /tmp/kind-images/controller.tar
	$(KIND) load image-archive /tmp/kind-images/controller.tar --name $(DEV_KIND_CLUSTER)
	rm -f /tmp/kind-images/controller.tar
	@echo "Loading rotator image into kind cluster ${DEV_KIND_CLUSTER}..."
	$(CONTAINER_TOOL) save docker.io/library/rotator:local -o /tmp/kind-images/rotator.tar
	$(KIND) load image-archive /tmp/kind-images/rotator.tar --name $(DEV_KIND_CLUSTER)
	rm -f /tmp/kind-images/rotator.tar
	$(MAKE) -C images push-all-kind CLUSTER_NAME=$(DEV_KIND_CLUSTER) CONTAINER_TOOL=$(CONTAINER_TOOL)

.PHONY: load-images-e2e
load-images-e2e: build-rotator ## Build and load all images into the e2e test Kind cluster
	@echo "Building manager image..."
	$(CONTAINER_TOOL) build $(BUILD_OPTS) -t $(E2E_MANAGER_IMAGE) .
	@echo "Loading images into e2e test cluster ${KIND_CLUSTER}..."
	@mkdir -p /tmp/kind-images
	$(CONTAINER_TOOL) save $(E2E_MANAGER_IMAGE) -o /tmp/kind-images/manager.tar
	$(KIND) load image-archive /tmp/kind-images/manager.tar --name $(KIND_CLUSTER)
	rm -f /tmp/kind-images/manager.tar
	$(CONTAINER_TOOL) save docker.io/library/rotator:local -o /tmp/kind-images/rotator.tar
	$(KIND) load image-archive /tmp/kind-images/rotator.tar --name $(KIND_CLUSTER)
	rm -f /tmp/kind-images/rotator.tar
	$(MAKE) -C images push-all-kind CLUSTER_NAME=$(KIND_CLUSTER) CONTAINER_TOOL=$(CONTAINER_TOOL)

.PHONY: kubectl-kind
kubectl-kind: ## Configure kubectl to use kind cluster
	@echo "Setting kubectl context to kind-$(DEV_KIND_CLUSTER)..."
	@if kubectl config get-contexts | grep -q "kind-$(DEV_KIND_CLUSTER)"; then \
		kubectl config use-context kind-$(DEV_KIND_CLUSTER); \
		echo "✅ kubectl configured to use kind cluster. Current context: $$(kubectl config current-context)"; \
	else \
		echo "❌ kind-$(DEV_KIND_CLUSTER) context not found. Try running 'make setup-kind' first."; \
		exit 1; \
	fi
	@echo "Checking connection to kind cluster..."
	@kubectl cluster-info || { \
		echo "❌ Cannot connect to kind cluster. There might be an issue with your kubeconfig or the cluster might not be running."; \
		echo "Try running 'make setup-kind' to recreate the cluster."; \
		exit 1; \
	}

.PHONY: deploy-kind
deploy-kind: docker-build build-rotator helm-generate kubectl-kind ## Build, load, and deploy controller to a kind cluster.
	$(MAKE) load-images
	helm upgrade --install $(HELM_RELEASE) dist/chart \
		--namespace jupyter-k8s-system --create-namespace \
		--set manager.image.repository=$${IMG%:*} \
		--set manager.image.tag=$${IMG##*:} \
		--set manager.image.pullPolicy=Never \
		--set application.imagesPullPolicy=Never \
		--set application.imagesRegistry='docker.io/library' \
		--set extensionApi.enable=true \
		--set extensionApi.jwtSecret.enable=true \
		--set extensionApi.jwtSecret.rotator.repository=docker.io/library \
		--set extensionApi.jwtSecret.rotator.imageName=rotator \
		--set extensionApi.jwtSecret.rotator.imageTag=local \
		--set extensionApi.jwtSecret.rotator.imagePullPolicy=Never \
		--set workspacePodWatching.enable=true \
		--set accessResources.traefik.enable=true

.PHONY: redeploy-kind
redeploy-kind: kubectl-kind ## Rebuild and redeploy controller to the kind cluster.
	$(KUBECTL) delete deployment jupyter-k8s-controller-manager -n jupyter-k8s-system --ignore-not-found
	$(MAKE) deploy-kind

.PHONY: apply-samples
apply-samples: ## Create sample workspaces in the kind cluster
	$(KUBECTL) apply -k config/samples

.PHONY: delete-samples
delete-samples: ## Delete sample workspaces from the kind cluster
	$(KUBECTL) delete -k config/samples --ignore-not-found

##@ Local Auth Middleware & Rotator
.PHONY: build-authmiddleware
build-authmiddleware: ## Build authmiddleware image for local testing
	@echo "Building authmiddleware image..."
	$(CONTAINER_TOOL) build $(BUILD_OPTS) -t docker.io/library/authmiddleware:local -f images/authmiddleware/Dockerfile .

.PHONY: build-rotator
build-rotator: ## Build rotator image for local testing
	@echo "Building rotator image..."
	$(CONTAINER_TOOL) build $(BUILD_OPTS) -t docker.io/library/rotator:local -f images/rotator/Dockerfile .

.PHONY: load-auth-images
load-auth-images: build-authmiddleware build-rotator ## Build and load authmiddleware and rotator images into Kind cluster
	@echo "Loading authmiddleware and rotator images into kind cluster ${DEV_KIND_CLUSTER}..."
	@mkdir -p /tmp/kind-images
	$(CONTAINER_TOOL) save docker.io/library/authmiddleware:local -o /tmp/kind-images/authmiddleware.tar
	$(CONTAINER_TOOL) save docker.io/library/rotator:local -o /tmp/kind-images/rotator.tar
	$(KIND) load image-archive /tmp/kind-images/authmiddleware.tar --name $(DEV_KIND_CLUSTER)
	$(KIND) load image-archive /tmp/kind-images/rotator.tar --name $(DEV_KIND_CLUSTER)
	rm -f /tmp/kind-images/authmiddleware.tar /tmp/kind-images/rotator.tar

.PHONY: deploy-auth-kind
deploy-auth-kind: kubectl-kind ## Deploy authmiddleware and rotator to Kind cluster
	@echo "Deploying authmiddleware and rotator to kind cluster..."
	$(KUSTOMIZE) build config-auth/default | $(KUBECTL) apply -f -
	@echo "✅ Authmiddleware and rotator deployed to jupyter-k8s-router namespace"

.PHONY: update-auth-kind
update-auth-kind: kubectl-kind ## Rebuild, reload images, and redeploy authmiddleware/rotator
	$(KUBECTL) delete deployment jupyter-k8s-authmiddleware -n jupyter-k8s-router --ignore-not-found=true
	$(MAKE) load-auth-images
	$(MAKE) deploy-auth-kind

.PHONY: undeploy-auth-kind
undeploy-auth-kind: kubectl-kind ## Remove authmiddleware and rotator from Kind cluster
	@echo "Removing authmiddleware and rotator from kind cluster..."
	$(KUSTOMIZE) build config-auth/default | $(KUBECTL) delete --ignore-not-found=true -f -
	@echo "✅ Authmiddleware and rotator removed (namespace preserved)"

##@ AWS Deployment
setup-aws-internal: ## Setup connection to remote cluster
	@echo "Setting up remote cluster connection..."
	@if [ -n "$(EKS_CLUSTER_NAME)" ]; then \
		echo "Getting kubeconfig from EKS cluster $(EKS_CLUSTER_NAME)..."; \
		aws eks update-kubeconfig \
			--name $(EKS_CLUSTER_NAME) \
			--region $(AWS_REGION); \
	else \
		echo "EKS_CLUSTER_NAME not provided. Please set it when running this command."; \
		exit 1; \
	fi
	@echo "Creating ECR repository for controller if it doesn't exist..."
	aws ecr describe-repositories --repository-names $(ECR_REPOSITORY) --region $(AWS_REGION) > /dev/null || \
	aws ecr create-repository --repository-name $(ECR_REPOSITORY) --region $(AWS_REGION)

	@echo "Creating ECR repository for auth middleware if it doesn't exist..."
	aws ecr describe-repositories --repository-names $(ECR_REPOSITORY_AUTH) --region $(AWS_REGION) > /dev/null || \
	aws ecr create-repository --repository-name $(ECR_REPOSITORY_AUTH) --region $(AWS_REGION)

	@echo "Creating ECR repository for rotator if it doesn't exist..."
	aws ecr describe-repositories --repository-names $(ECR_REPOSITORY_ROTATOR) --region $(AWS_REGION) > /dev/null || \
	aws ecr create-repository --repository-name $(ECR_REPOSITORY_ROTATOR) --region $(AWS_REGION)

	@if ! kubectl get namespace cert-manager > /dev/null 2>&1; then \
		echo "Installing cert-manager"; \
		helm repo add jetstack https://charts.jetstack.io; \
		helm repo update; \
		helm install cert-manager jetstack/cert-manager \
			--namespace cert-manager \
			--create-namespace \
			--set installCRDs=true; \
		echo "Waiting for cert-manager to be ready..."; \
		kubectl wait --for=condition=Available --timeout=300s deployment/cert-manager-webhook -n cert-manager; \
	else \
		echo "cert-manager is already installed, skipping installation"; \
	fi

	@if ! kubectl get crds | grep traefik > /dev/null 2>&1; then \
		echo "Installing traefik"; \
		helm repo add traefik https://traefik.github.io/charts; \
		helm repo update; \
		helm install traefik-crd traefik/traefik-crds \
			--namespace traefik \
  			--create-namespace \
			--version $(TRAEFIK_CRD_CHART_VERSION); \
		echo "Successfully installed traefik CRDs"; \
	else \
		echo "traefik is already installed, skipping installation"; \
	fi

	@echo "Remote AWS setup complete. Credentials added to ~/.kube/config"

.PHONY: setup-aws
setup-aws:
	$(MAKE) setup-aws-internal CLOUD_PROVIDER=aws

kubectl-aws-internal: ## Configure kubectl to use remote cluster
	@echo "Setting up kubectl to use remote cluster..."
	@if kubectl config get-contexts | grep -q "$(EKS_CLUSTER_NAME)"; then \
		echo "Switching to EKS cluster context..."; \
		kubectl config use-context "$(EKS_CONTEXT)"; \
		echo "✅ kubectl configured to use remote cluster. Current context: $$(kubectl config current-context)"; \
	else \
		echo "❌ EKS cluster context not found. Try running 'make setup-aws CLOUD_PROVIDER=aws EKS_CLUSTER_NAME=your-cluster-name' first."; \
		exit 1; \
	fi
	@echo "\nTest your connection with: kubectl get nodes"

.PHONY: kubectl-aws
kubectl-aws:
	$(MAKE) kubectl-aws-internal CLOUD_PROVIDER=aws

load-images-aws-internal: manifests generate fmt vet ## Build and push container images to remote registry
	@echo "Logging in to ECR..."
	aws ecr get-login-password --region $(AWS_REGION) | $(CONTAINER_TOOL) login --username AWS --password-stdin $(ECR_REGISTRY)

	@echo "Creating ECR repositories if they don't exist..."
	@aws ecr describe-repositories --repository-names $(ECR_REPOSITORY) --region $(AWS_REGION) > /dev/null 2>&1 || \
		aws ecr create-repository --repository-name $(ECR_REPOSITORY) --region $(AWS_REGION)
	@aws ecr describe-repositories --repository-names $(ECR_REPOSITORY_AUTH) --region $(AWS_REGION) > /dev/null 2>&1 || \
		aws ecr create-repository --repository-name $(ECR_REPOSITORY_AUTH) --region $(AWS_REGION)
	@aws ecr describe-repositories --repository-names $(ECR_REPOSITORY_ROTATOR) --region $(AWS_REGION) > /dev/null 2>&1 || \
		aws ecr create-repository --repository-name $(ECR_REPOSITORY_ROTATOR) --region $(AWS_REGION)

	@echo "Building controller image..."
	$(CONTAINER_TOOL) build $(BUILD_OPTS) --platform=linux/amd64 -t $(ECR_REGISTRY)/$(ECR_REPOSITORY):latest .
	$(CONTAINER_TOOL) push $(ECR_REGISTRY)/$(ECR_REPOSITORY):latest
	@echo "Controller image built and pushed successfully to $(ECR_REGISTRY)/$(ECR_REPOSITORY):latest"

	@echo "Building auth middleware image..."
	$(CONTAINER_TOOL) build $(BUILD_OPTS) --platform=linux/amd64 -t $(ECR_REGISTRY)/$(ECR_REPOSITORY_AUTH):latest -f images/authmiddleware/Dockerfile .
	$(CONTAINER_TOOL) push $(ECR_REGISTRY)/$(ECR_REPOSITORY_AUTH):latest
	@echo "Auth middleware image built and pushed successfully to $(ECR_REGISTRY)/$(ECR_REPOSITORY_AUTH):latest"

	@echo "Building rotator image..."
	$(CONTAINER_TOOL) build $(BUILD_OPTS) --platform=linux/amd64 -t $(ECR_REGISTRY)/$(ECR_REPOSITORY_ROTATOR):latest -f images/rotator/Dockerfile .
	$(CONTAINER_TOOL) push $(ECR_REGISTRY)/$(ECR_REPOSITORY_ROTATOR):latest
	@echo "Rotator image built and pushed successfully to $(ECR_REGISTRY)/$(ECR_REPOSITORY_ROTATOR):latest"

	@echo "Building application images..."
	$(MAKE) -C images push-all-aws CLOUD_PROVIDER=aws CONTAINER_TOOL=$(CONTAINER_TOOL)
	@echo "All images built and pushed successfully to $(ECR_REGISTRY)"

.PHONY: load-images-aws
load-images-aws:
	$(MAKE) load-images-aws-internal CLOUD_PROVIDER=aws

WS_NAMESPACE ?= default
.PHONY: bearer-token
bearer-token: ## Create a bearer token connection for a workspace. Usage: make bearer-token WS_NAME=<name> [WS_NAMESPACE=default]
	@bash -c '\
		RESULT=$$(kubectl create --raw "/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/$(WS_NAMESPACE)/workspaceconnections" \
			-f <(echo '"'"'{"apiVersion":"connection.workspace.jupyter.org/v1alpha1","kind":"WorkspaceConnection","metadata":{"namespace":"$(WS_NAMESPACE)"},"spec":{"workspaceName":"$(WS_NAME)","workspaceConnectionType":"web-ui"}}'"'"') 2>&1) && \
		URL=$$(echo "$$RESULT" | jq -r ".status.workspaceConnectionUrl // empty") && \
		echo "$$URL" || \
		{ echo "$$RESULT"; exit 1; } \
	'

.PHONY: vscode-token
vscode-token: ## Create a VS Code remote connection for a workspace. Usage: make vscode-token WS_NAME=<name> [WS_NAMESPACE=default]
	@bash -c '\
		RESULT=$$(kubectl create --raw "/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/$(WS_NAMESPACE)/workspaceconnections" \
			-f <(echo '"'"'{"apiVersion":"connection.workspace.jupyter.org/v1alpha1","kind":"WorkspaceConnection","metadata":{"namespace":"$(WS_NAMESPACE)"},"spec":{"workspaceName":"$(WS_NAME)","workspaceConnectionType":"vscode-remote"}}'"'"') 2>&1) && \
		URL=$$(echo "$$RESULT" | jq -r ".status.workspaceConnectionUrl // empty") && \
		echo "$$URL" || \
		{ echo "$$RESULT"; exit 1; } \
	'


# Port forward to a specific Jupyter server
.PHONY: port-forward
port-forward:
	@echo "Available Jupyter servers:"
	@$(KUBECTL) get Workspaces --no-headers | awk '{print "  " $$1}'
	@echo ""
	@read -p "Enter server name: " SERVER_NAME; \
	if [ -z "$$SERVER_NAME" ]; then \
		echo "Server name cannot be empty"; \
		exit 1; \
	fi; \
	echo "Port forwarding to jupyter-$$SERVER_NAME-service..."; \
	if [ "$(uname)" = "Darwin" ]; then \
		echo "Setting up port with localhost for laptop development..."; \
		echo "Proxy available at http://localhost:8888/ (routes to web app and API)"; \
		$(KUBECTL) port-forward service/jupyter-$$SERVER_NAME-service 8888:8888; \
	else \
		echo "Setting up port forwarding using hostname for desktop development..."; \
		HOST=$$(hostname -f); \
		echo "Using hostname: $$HOST"; \
		echo "Available at http://$$HOST:9888/"; \
		$(KUBECTL) port-forward service/jupyter-$$SERVER_NAME-service 9888:8888 --address=0.0.0.0 --request-timeout=5m; \
	fi

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -
	$(KUBECTL) delete -f config/rbac/extension_api_auth_binding.yaml --ignore-not-found

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KIND ?= kind
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest

# GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
GOLANGCI_LINT := golangci-lint

## Tool Versions
KUSTOMIZE_VERSION ?= v5.6.0
CONTROLLER_TOOLS_VERSION ?= v0.18.0
#ENVTEST_VERSION is the version of controller-runtime release branch to fetch the envtest setup script (i.e. release-0.20)
ENVTEST_VERSION ?= $(shell go list -m -f "{{ .Version }}" sigs.k8s.io/controller-runtime | awk -F'[v.]' '{printf "release-%d.%d", $$2, $$3}')
#ENVTEST_K8S_VERSION is the version of Kubernetes to use for setting up ENVTEST binaries (i.e. 1.31)
ENVTEST_K8S_VERSION ?= $(shell go list -m -f "{{ .Version }}" k8s.io/api | awk -F'[v.]' '{printf "1.%d", $$3}')
GOLANGCI_LINT_VERSION ?= v2.4.0

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: setup-envtest
setup-envtest: envtest ## Download the binaries required for ENVTEST in the local bin directory.
	@echo "Setting up envtest binaries for Kubernetes version $(ENVTEST_K8S_VERSION)..."
	@$(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path || { \
		echo "Error: Failed to set up envtest binaries for version $(ENVTEST_K8S_VERSION)."; \
		exit 1; \
	}

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint:
	@which golangci-lint > /dev/null 2>&1 || { \
		echo "golangci-lint not found. Installing with brew..."; \
		brew install golangci-lint; \
	}
# golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
# $(GOLANGCI_LINT): $(LOCALBIN)
# 	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] && [ "$$(readlink -- "$(1)" 2>/dev/null)" = "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $$(realpath $(1)-$(3)) $(1)
endef

##@ Helm Deployment

## Helm binary to use for deploying the chart
HELM ?= helm
## Namespace to deploy the Helm release
HELM_NAMESPACE ?= jupyter-k8s-system
## Name of the Helm release
HELM_RELEASE ?= jupyter-k8s
## Path to the Helm chart directory
HELM_CHART_DIR ?= dist/chart
## Additional arguments to pass to helm commands
HELM_EXTRA_ARGS ?=

.PHONY: install-helm
install-helm: ## Install the latest version of Helm.
	@command -v $(HELM) >/dev/null 2>&1 || { \
		echo "Installing Helm..." && \
		curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-4 | bash; \
	}

.PHONY: helm-deploy
helm-deploy: install-helm ## Deploy manager to the K8s cluster via Helm. Specify an image with IMG.
	$(HELM) upgrade --install $(HELM_RELEASE) $(HELM_CHART_DIR) \
		--namespace $(HELM_NAMESPACE) \
		--create-namespace \
		--set manager.image.repository=$${IMG%:*} \
		--set manager.image.tag=$${IMG##*:} \
		--wait \
		--timeout 5m \
		$(HELM_EXTRA_ARGS)

.PHONY: helm-uninstall
helm-uninstall: ## Uninstall the Helm release from the K8s cluster.
	$(HELM) uninstall $(HELM_RELEASE) --namespace $(HELM_NAMESPACE)

.PHONY: helm-status
helm-status: ## Show Helm release status.
	$(HELM) status $(HELM_RELEASE) --namespace $(HELM_NAMESPACE)

.PHONY: helm-history
helm-history: ## Show Helm release history.
	$(HELM) history $(HELM_RELEASE) --namespace $(HELM_NAMESPACE)

.PHONY: helm-rollback
helm-rollback: ## Rollback to previous Helm release.
	$(HELM) rollback $(HELM_RELEASE) --namespace $(HELM_NAMESPACE)

##@ Documentation

.PHONY: docs
docs: docs-diagrams ## Build documentation HTML.
	$(MAKE) -C docs html

.PHONY: docs-serve
docs-serve: ## Serve documentation with live reload.
	sphinx-autobuild docs/source docs/build/html --port 8080

.PHONY: docs-serve-host
docs-serve-host: docs ## Serve documentation on all interfaces (for remote access).
	@echo "Serving at http://$$(hostname):8080"
	python3 -m http.server 8080 --bind 0.0.0.0 --directory docs/build/html

.PHONY: docs-diagrams
docs-diagrams: ## Render d2 diagrams to SVG.
	@mkdir -p docs/source/_static/img/diagrams
	@for f in diagrams/*.d2; do \
		[ -f "$$f" ] || continue; \
		d2 "$$f" "docs/source/_static/img/diagrams/$$(basename "$${f%.d2}.svg")"; \
	done
