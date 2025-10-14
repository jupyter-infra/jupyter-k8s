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
ifeq ($(CLOUD_PROVIDER),aws)
	AWS_REGION ?= us-west-2
	AWS_ACCOUNT_ID := $(shell aws sts get-caller-identity --query "Account" --output text)
	ECR_REGISTRY := $(AWS_ACCOUNT_ID).dkr.ecr.$(AWS_REGION).amazonaws.com
	ECR_REPOSITORY := jupyter-k8s
	ECR_REPOSITORY_AUTH := jupyter-k8s-auth
	EKS_CLUSTER_NAME ?= jupyter-k8s-cluster
	EKS_CONTEXT := arn:aws:eks:$(AWS_REGION):$(AWS_ACCOUNT_ID):cluster/$(EKS_CLUSTER_NAME)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

# Guided deployment
OAUTH2P_COOKIE_SECRET := $(shell openssl rand -base64 32 | tr -- '+/' '-_')

.PHONY: all
all: build

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

# Used for manual development
DEV_KIND_CLUSTER ?= jupyter-k8s-dev

# Set this variable to true to use local kind cluster for deployment
USE_KIND ?= false

.PHONY: setup-test-e2e
setup-test-e2e: ## Set up a Kind cluster for e2e tests if it does not exist
	@command -v $(KIND) >/dev/null 2>&1 || { \
		echo "Kind is not installed. Please install Kind manually."; \
		exit 1; \
	}
	@case "$$($(KIND) get clusters)" in \
		*"$(KIND_CLUSTER)"*) \
			echo "Kind cluster '$(KIND_CLUSTER)' already exists. Skipping creation." ;; \
		*) \
			echo "Creating Kind cluster '$(KIND_CLUSTER)'..."; \
			$(KIND) create cluster --name $(KIND_CLUSTER) ;; \
	esac
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
test-e2e: setup-test-e2e manifests generate fmt vet ## Run the e2e tests. Expected an isolated environment using Kind.
	KIND_CLUSTER=$(KIND_CLUSTER) go test -tags=e2e ./test/e2e/ -v -ginkgo.v
	$(MAKE) cleanup-test-e2e

.PHONY: cleanup-test-e2e
cleanup-test-e2e: ## Tear down the Kind cluster used for e2e tests
	@$(KIND) delete cluster --name $(KIND_CLUSTER)

.PHONY: lint
lint: golangci-lint helm-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	$(GOLANGCI_LINT) config verify

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

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
	kubebuilder edit --plugins=helm/v1-alpha --force
	./hack/apply-helm-patches.sh

.PHONY: helm-package
helm-package: helm-generate ## Package the Helm chart
	helm package dist/chart -d dist
	helm package guided-charts/aws-traefik-dex -d dist

.PHONY: helm-lint
helm-lint: ## Lint the Helm chart
	helm lint dist/chart
	helm lint guided-charts/aws-traefik-dex \
		--set domain=a.fine.example.com \
		--set certManager.email=admin@example.com \
		--set storageClass.efs.parameters.fileSystemId=fs-00001111222233334 \
		--set github.clientId=some-client-id \
		--set github.clientSecret=some-github-secret \
		--set github.orgs[0].name=some-org \
		--set github.orgs[0].teams[0]=ace-devs \
		--set githubRbac.orgs[0].name=some-org \
		--set githubRbac.orgs[0].teams[0]=ace-devs \
		--set oauth2Proxy.cookieSecret=$(OAUTH2P_COOKIE_SECRET) \
		--set authmiddleware.jwtSigningKey=some-signing-key \
		--set authmiddleware.csrfAuthKey=some-auth-key

.PHONY: helm-test
helm-test: ## Test the Helm chart with helm template
	rm -rf dist/test-output-crd-only
	rm -rf /tmp/helm-test-chart
	cp -r dist/chart /tmp/helm-test-chart
	cd /tmp/helm-test-chart && helm dependency build
	helm template jk8s /tmp/helm-test-chart --output-dir dist/test-output-crd-only
	rm -rf /tmp/helm-test-chart
	go test ./test/helm/crd-only -v

.PHONY: helm-test-aws-traefik-dex
helm-test-aws-traefik-dex: ## Test the Helm chart with guided mode (aws-traefik-dex)
	rm -rf dist/test-output-guided
	rm -rf /tmp/helm-test-chart
	cp -r guided-charts/aws-traefik-dex /tmp/helm-test-chart
	cd /tmp/helm-test-chart && helm dependency build
	helm template jk8s /tmp/helm-test-chart --output-dir dist/test-output-guided \
		--set domain=a.fine.example.com \
		--set certManager.email=admin@example.com \
		--set storageClass.efs.parameters.fileSystemId=fs-00001111222233334 \
		--set github.clientId=some-client-id \
		--set github.clientSecret=some-github-secret \
		--set github.orgs[0].name=some-org \
		--set github.orgs[0].teams[0]=ace-devs \
		--set githubRbac.orgs[0].name=some-org \
		--set githubRbac.orgs[0].teams[0]=ace-devs \
		--set oauth2Proxy.cookieSecret=$(OAUTH2P_COOKIE_SECRET) \
		--set authmiddleware.jwtSigningKey=some-signing-key \
		--set authmiddleware.csrfAuthKey=some-auth-key
	# Clean up temporary chart directory
	rm -rf /tmp/helm-test-chart
	# Run helm tests to verify resources
	go test ./test/helm/guided-aws-traefik-dex -v


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

.PHONY: teardown-kind
teardown-kind: ## Tear down the Kind cluster, registry, and clean up images
	# Delete the Kind cluster
	$(KIND) delete cluster --name=$(DEV_KIND_CLUSTER)
	# Stop and remove registry container if running
	$(CONTAINER_TOOL) stop registry || true
	$(CONTAINER_TOOL) rm registry || true
	# Clean up images from Finch cache
	@echo "Cleaning up images from cache..."
	$(CONTAINER_TOOL) rmi ${IMG} || true
	$(MAKE) -C images clean CONTAINER_TOOL=$(CONTAINER_TOOL)

.PHONY: load-images
load-images: docker-build ## Build and load images into the Kind cluster
	@echo "Loading controller image ${IMG} into kind cluster ${DEV_KIND_CLUSTER}..."
	@mkdir -p /tmp/kind-images
	$(CONTAINER_TOOL) save ${IMG} -o /tmp/kind-images/controller.tar
	$(KIND) load image-archive /tmp/kind-images/controller.tar --name $(DEV_KIND_CLUSTER)
	rm -f /tmp/kind-images/controller.tar
	$(MAKE) -C images push-all-kind CLUSTER_NAME=$(DEV_KIND_CLUSTER) CONTAINER_TOOL=$(CONTAINER_TOOL)

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
deploy-kind: docker-build kubectl-kind ## Build, load, and deploy controller to a kind cluster.
	$(MAKE) deploy USE_KIND=true EXTRA_HELM_ARGS="--set application.imagesPullPolicy=Never --set application.imagesRegistry='docker.io/library'"

.PHONY: redeploy-kind
redeploy-kind: kubectl-kind
	$(KUBECTL) delete deployment jupyter-k8s-controller-manager -n jupyter-k8s-system
	$(MAKE) deploy-kind


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
  			--create-namespace; \
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

	@echo "Building controller image..."
	$(CONTAINER_TOOL) build $(BUILD_OPTS) --platform=linux/amd64 -t $(ECR_REGISTRY)/$(ECR_REPOSITORY):latest .
	$(CONTAINER_TOOL) push $(ECR_REGISTRY)/$(ECR_REPOSITORY):latest
	@echo "Controller image built and pushed successfully to $(ECR_REGISTRY)/$(ECR_REPOSITORY):latest"

	@echo "Building auth middleware image..."
	$(CONTAINER_TOOL) build $(BUILD_OPTS) --platform=linux/amd64 -t $(ECR_REGISTRY)/$(ECR_REPOSITORY_AUTH):latest -f images/authmiddleware/Dockerfile .
	$(CONTAINER_TOOL) push $(ECR_REGISTRY)/$(ECR_REPOSITORY_AUTH):latest
	@echo "Auth middleware image built and pushed successfully to $(ECR_REGISTRY)/$(ECR_REPOSITORY_AUTH):latest"

	@echo "Building application images..."
	$(MAKE) -C images push-all-aws CLOUD_PROVIDER=aws CONTAINER_TOOL=$(CONTAINER_TOOL)
	@echo "All images built and pushed successfully to $(ECR_REGISTRY)"

.PHONY: load-images-aws
load-images-aws:
	$(MAKE) load-images-aws-internal CLOUD_PROVIDER=aws

deploy-aws-internal: helm-generate load-images-aws ## Deploy helm chart to remote cluster
	@echo "Deploying jupyter-k8s CRD and controller with Helm chart to remote AWS cluster..."
	rm -rf /tmp/jk8s-helm-crd-only
	cp -r dist/chart /tmp/jk8s-helm-crd-only
	cd /tmp/jk8s-helm-crd-only
	helm upgrade --install jk8s /tmp/jk8s-helm-crd-only \
		--namespace jupyter-k8s-system --create-namespace \
		--set controllerManager.container.imagePullPolicy=Always \
		--set controllerManager.container.image.repository=$(ECR_REGISTRY)/$(ECR_REPOSITORY) \
		--set controllerManager.container.image.tag=latest \
		--set application.imagesPullPolicy=Always \
		--set application.imagesRegistry=$(ECR_REGISTRY) \
		--set traefik.enable=true
	@echo "Helm chart jupyter-k8s deployed successfully to remote AWS cluster"
	@echo "Restarting deployments to use new images..."
	kubectl rollout restart deployment -n jupyter-k8s-system jupyter-k8s-controller-manager
	rm -rf /tmp/jk8s-helm-crd-only

.PHONY: deploy-aws
deploy-aws:
	$(MAKE) deploy-aws-internal CLOUD_PROVIDER=aws

# Load environment variables from .env file for guided deployment
deploy-aws-traefik-dex-internal:
	@if [ ! -f .env ]; then \
		echo "❌ .env file not found. Copy the `.env.example` file to `.env` and edit the values."; \
		echo "Required variables: DOMAIN, LETSENCRYPT_EMAIL, GITHUB_CLIENT_ID, GITHUB_CLIENT_SECRET, GITHUB_ORG_NAME"; \
		echo "Optional variables: DEX_OAUTH2_SECRET (defaults to auto-generated), DEX_K8S_SECRET (can be empty for public clients)"; \
		exit 1; \
	fi
	@echo "Loading configuration from .env file and deploying..."
	@( \
		set -e; \
		. ./.env; \
		rm -rf /tmp/jk8s-aws-traefik-dex; \
		mkdir /tmp/jk8s-aws-traefik-dex; \
		cp -r guided-charts/aws-traefik-dex/ /tmp/jk8s-aws-traefik-dex/; \
		echo 'Deploying AWS traefik dex helm chart'; \
		HELM_ARGS="--set domain=$$DOMAIN \
			--set certManager.email=$$LETSENCRYPT_EMAIL \
			--set storageClass.efs.parameters.fileSystemId=$$EFS_ID \
			--set github.clientId=$$GITHUB_CLIENT_ID \
			--set github.clientSecret=$$GITHUB_CLIENT_SECRET \
			--set github.orgs[0].name=$$GITHUB_ORG_NAME \
			--set github.orgs[0].teams[0]=$$GITHUB_TEAM \
			--set githubRbac.orgs[0].name=$$GITHUB_ORG_NAME \
			--set githubRbac.orgs[0].teams[0]=$$GITHUB_TEAM \
			--set oauth2Proxy.cookieSecret=$(OAUTH2P_COOKIE_SECRET) \
			--set authmiddleware.repository=$(ECR_REGISTRY) \
			--set authmiddleware.imageName=$(ECR_REPOSITORY_AUTH) \
			--set authmiddleware.jwtSigningKey=$$JWT_SIGNING_KEY \
			--set authmiddleware.csrfAuthKey=$$CSRF_AUTH_KEY"; \
		\
		if [ ! -z "$$DEX_OAUTH2_SECRET" ]; then \
			HELM_ARGS="$$HELM_ARGS --set dex.oauth2ProxyClientSecret=$$DEX_OAUTH2_SECRET"; \
		fi; \
		\
		if [ ! -z "$$DEX_K8S_SECRET" ]; then \
			HELM_ARGS="$$HELM_ARGS --set dex.kubernetesClientSecret=$$DEX_K8S_SECRET"; \
		fi; \
		\
		# Default redirect ports are set in values.yaml (8000, 18000, 9800) \
		# But allow overriding with env vars \
		if [ ! -z "$$KUBECTL_REDIRECT_PORTS" ]; then \
			IFS=',' read -ra PORTS <<< "$$KUBECTL_REDIRECT_PORTS"; \
			PORT_INDEX=0; \
			for PORT in "$${PORTS[@]}"; do \
				HELM_ARGS="$$HELM_ARGS --set dex.kubernetesClientRedirectPorts[$${PORT_INDEX}]=$${PORT}"; \
				PORT_INDEX=$$((PORT_INDEX + 1)); \
			done; \
		fi; \
		\
		helm upgrade --install jk8-aws-traefik-dex /tmp/jk8s-aws-traefik-dex/aws-traefik-dex \
			-n jupyter-k8s-router \
			--create-namespace \
			--force \
			$$HELM_ARGS; \
		\
		$(SHELL) scripts/aws-traefik-dex/generate-client.sh \
			$(EKS_CLUSTER_NAME) \
			https://$$DOMAIN/dex \
			$(AWS_REGION) \
			9800 \
			dist/users-scripts/set-kubeconfig.sh; \
	)
	@echo "Restarting deployments to use new images..."
	kubectl rollout restart deployment -n jupyter-k8s-router \
		traefik oauth2-proxy dex authmiddleware
	@echo "All deployments to use new images..."
	# Clean up temporary chart directory
	rm -rf /tmp/jk8s-aws-traefik-dex
	@echo "Bash script for end-users to set their kubeconfig available at: dist/users-scripts"


.PHONY: deploy-aws-traefik-dex
deploy-aws-traefik-dex:
	$(MAKE) deploy-aws-traefik-dex-internal CLOUD_PROVIDER=aws

.PHONY: apply-sample-routing
apply-sample-routing:
	@echo "Loading configuration from .env file and deploying..."
	@( \
		set -e; \
		. ./.env; \
		export DOMAIN=$$DOMAIN; \
		kubectl apply -k config/samples_routing --dry-run=client -o yaml | envsubst | kubectl apply -f -; \
	)

.PHONY: delete-sample-routing
delete-sample-routing:
	kubectl delete -k config/samples_routing

.PHONY: undeploy-aws
undeploy-aws: ## Uninstall the Helm chart from remote cluster
	@echo "Undeploying Helm chart from remote AWS cluster..."
	# Not specifying namespace to allow Helm to clean up resources across all namespaces
	helm uninstall jk8s -n jupyter-k8s-system --ignore-not-found
	helm uninstall jk8-aws-traefik-dex -n jupyter-k8s-router --ignore-not-found
	helm uninstall traefik-crd --ignore-not-found
	# CRDs with resource-policy: keep are not deleted by Helm, but we can find them by release name

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
