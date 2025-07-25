# Variables
UV := uv
PYTHON := uv run --script
HELM := helm
DOCKER := finch

# aws-specific to work on clouddesktop
BUILD_OPTS := --network host

IMAGE_NAME := localhost:5000/jupyter-k8s-controller
IMAGE_TAG := latest
NAMESPACE := jupyter-system
CLUSTER := jupyter-k8s
KUBECONFIG := .kubeconfig
KUBECLT := kubectl
LOCAL_K8 := kind
SHELL := sh

# Default target
.DEFAULT_GOAL := help

# runs uv sync
.PHONY: sync
sync:
	$(UV) sync

# Fix issues
.PHONY: fix-all
fix-all:
	$(UV) run ruff format
	$(UV) run ruff check --fix

# Check without fixing
.PHONY: check-all
check-all:
	$(UV) run ruff check
	$(UV) run mypy
	$(UV) run pytest
	$(PYTHON) scripts/verify_format.py
	$(HELM) lint --strict helm/jupyter-k8s

# Attempt to fix issues, then run tests
.PHONY: run-all
run-all:
	$(UV) run ruff format
	$(UV) run ruff check --fix
	$(UV) run mypy
	$(UV) run pytest
	$(HELM) lint --strict helm/jupyter-k8s

# Set up local development environment
.PHONY: local-dev-setup
local-dev-setup:
	$(SHELL) ./local-dev/kind/setup.sh

# Build the docker image and push to local registry
.PHONY: build
build:
	$(DOCKER) $(BUILD_OPTS) build --no-cache -t $(IMAGE_NAME):$(IMAGE_TAG) .
	$(DOCKER) push $(IMAGE_NAME):$(IMAGE_TAG)

# Apply CRD
.PHONY: apply-crd
apply-crd:
	$(KUBECLT) --kubeconfig=$(KUBECONFIG) apply -f helm/jupyter-k8s/crds/jupyter.yaml

# Delete CRD
.PHONY: delete-crd
delete-crd:
	$(KUBECLT) --kubeconfig=$(KUBECONFIG) delete crd servers.jupyter.org --ignore-not-found=true

# Build and install Helm chart locally
.PHONY: local-deploy
local-deploy: local-dev-setup
	$(MAKE) build
	$(HELM) lint --strict helm/jupyter-k8s
	$(MAKE) delete-crd
	$(MAKE) apply-crd
	$(HELM) upgrade --install jupyter-k8s helm/jupyter-k8s \
		--namespace $(NAMESPACE) --create-namespace \
		--kubeconfig $(KUBECONFIG)

# Run basic tests with the CRD on the running cluster
.PHONY: operator-tests
operator-tests:
	@echo "CREATE TEST"
	$(KUBECLT) --kubeconfig=$(KUBECONFIG) apply -f examples/sample-notebook.yaml
	@echo "JupyterServer successfully created"
	@echo ""
	@echo "GET TEST"
	$(KUBECLT) --kubeconfig=$(KUBECONFIG) get JupyterServer
	@echo "JupyterServer successfully retrieved"
	@echo ""
	@echo "DELETE TEST"
	$(KUBECLT) --kubeconfig=$(KUBECONFIG) delete JupyterServer sample-notebook
	@echo "JupyterServer successfully deleted"

# Tear down local development environment
.PHONY: local-dev-teardown
local-dev-teardown:
	$(LOCAL_K8) delete cluster --name=$(CLUSTER)
	$(DOCKER) stop registry || true
	$(DOCKER) rm registry || true

# Clean build artifacts
.PHONY: clean
clean:
	rm -rf build/
	rm -rf dist/
	rm -rf *.egg-info
	find . -type d -name __pycache__ -exec rm -rf {} +
	find . -type f -name "*.pyc" -delete

# Help message
help:
	@echo "Makefile targets:"
	@echo "  sync                - Sync the UV environment"
	@echo "  fix-all             - Run formatter and linter fix"
	@echo "  check-all           - Run linter without fix, type-checker and unit tests"
	@echo "  run-all             - Run formatter and linter fix, type-check and unit tests"
	@echo "  build               - Bundle dependencies, build Docker image and push to local registry"
	@echo "  local-dev-setup     - Set up local cluster and configure kubectl access"
	@echo "  build               - Build the image and push to local finch cluster"
	@echo "  apply-crd           - Manually apply CRD to the cluster"
	@echo "  delete-crd          - Delete the old CRD from the cluster, no-op if does not exist"
	@echo "  local-deploy        - Set up local cluster, configure kubectl access, build and install Helm chart locally"
	@echo "  operator-tests      - Test basic operation on the custom operators"
	@echo "  local-dev-teardown  - Tear down local development environment"
	@echo "  clean               - Clean build artifacts"