# Variables
PYTHON := python3
UV := uv
HELM := helm
DOCKER := finch

# aws-specific to work on clouddesktop
BUILD_OPTS := --network host

IMAGE_NAME := localhost:5000/jupyter-k8s-controller
IMAGE_TAG := latest
NAMESPACE := jupyter-system
CLUSTER := jupyter-k8s
KUBECONFIG := .kubeconfig
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
check:
	$(UV) run ruff check
	$(UV) run mypy
	$(UV) run pytest

# Run-all
.PHONY: run-all
run-all:
	$(UV) run ruff format
	$(UV) run ruff check --fix
	$(UV) run mypy
	$(UV) run pytest

# Set up local development environment
.PHONY: local-dev-setup
local-dev-setup:
	$(SHELL) ./local-dev/kind/setup.sh

# Build the docker image and push
.PHONY: build
build:
	$(DOCKER) $(BUILD_OPTS) build -t $(IMAGE_NAME):$(IMAGE_TAG) .
	$(DOCKER) push $(IMAGE_NAME):$(IMAGE_TAG)
# $(LOCAL_K8) load docker-image $(IMAGE_NAME) --name $(CLUSTER)

# Build and install Helm chart locally
.PHONY: deploy-local
deploy-local: local-dev-setup
	$(MAKE) build
	$(HELM) lint helm/jupyter-k8s
	$(HELM) install jupyter-k8s helm/jupyter-k8s \
		--namespace $(NAMESPACE) --create-namespace \
		--kubeconfig $(KUBECONFIG)

# Remove and reinstall Helm chart
.PHONY: reinstall
reinstall:
	$(HELM) uninstall jupyter-k8s --namespace $(NAMESPACE) --kubeconfig $(KUBECONFIG)
	$(MAKE) deploy-local

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
	@echo "  deploy-local        - Set up local cluster, configure kubectl access, build and install Helm chart locally"
	@echo "  reinstall           - Build and reinstall the Helm chart on the local cluster"
	@echo "  local-dev-teardown  - Tear down local development environment"
	@echo "  clean               - Clean build artifacts"