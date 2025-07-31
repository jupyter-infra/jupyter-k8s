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
	$(UV) run mypy src tests
	$(UV) run pytest
	$(PYTHON) scripts/verify_format.py
	$(HELM) lint --strict helm/jupyter-k8s

# Attempt to fix issues, then run tests
.PHONY: run-all
run-all:
	$(UV) run ruff format
	$(UV) run ruff check --fix
	$(UV) run mypy src tests
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
	$(KUBECLT) --kubeconfig=$(KUBECONFIG) delete crd jupyterservers.servers.jupyter.org --ignore-not-found=true

# Build and install Helm chart locally
.PHONY: local-deploy
local-deploy: local-dev-setup
	$(MAKE) build
	$(HELM) lint --strict helm/jupyter-k8s
	$(MAKE) delete-crd
	$(MAKE) apply-crd
	$(HELM) upgrade --install jupyter-k8s helm/jupyter-k8s \
		--namespace $(NAMESPACE) --create-namespace \
		--kubeconfig $(KUBECONFIG) \
		--set image.repository=$(IMAGE_NAME) \
		--set image.tag=$(IMAGE_TAG)

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
	@echo "Waiting for operator to process the resource..."
	@sleep 5
	@echo "Checking if deployment was created..."
	$(KUBECLT) --kubeconfig=$(KUBECONFIG) get deployment jupyter-sample-notebook
	@echo "Deployment found!"
	@echo ""
	@echo "Waiting for deployment to be ready..."
	$(KUBECLT) --kubeconfig=$(KUBECONFIG) wait --for=condition=available --timeout=300s deployment/jupyter-sample-notebook
	@echo "Deployment is ready!"
	@echo ""
	@echo "Checking service..."
	$(KUBECLT) --kubeconfig=$(KUBECONFIG) get service jupyter-sample-notebook-service
	@echo "Service found!"
	@echo ""
	@echo "Checking JupyterServer status..."
	$(KUBECLT) --kubeconfig=$(KUBECONFIG) get jupyterserver sample-notebook -o jsonpath='{.status}' || echo "Status not yet available"
	@echo ""
	@echo "DELETE TEST"
	$(KUBECLT) --kubeconfig=$(KUBECONFIG) delete JupyterServer sample-notebook
	@echo "JupyterServer successfully deleted"
	@echo ""
	@echo "Verifying cleanup..."
	@sleep 3
	@echo "Checking if deployment was cleaned up..."
	$(KUBECLT) --kubeconfig=$(KUBECONFIG) get deployment jupyter-sample-notebook 2>/dev/null && echo "WARNING: Deployment still exists" || echo "Deployment cleaned up successfully"
	@echo "Checking if service was cleaned up..."
	$(KUBECLT) --kubeconfig=$(KUBECONFIG) get service jupyter-sample-notebook-service 2>/dev/null && echo "WARNING: Service still exists" || echo "Service cleaned up successfully"

# Port forward to a specific Jupyter server
.PHONY: port-forward
port-forward:
	@echo "Available Jupyter servers:"
	@$(KUBECLT) --kubeconfig=$(KUBECONFIG) get jupyterservers --no-headers | awk '{print "  " $$1}'
	@echo ""
	@read -p "Enter server name: " SERVER_NAME; \
	if [ -z "$$SERVER_NAME" ]; then \
		echo "Server name cannot be empty"; \
		exit 1; \
	fi; \
	echo "Port forwarding to jupyter-$$SERVER_NAME-service..."; \
	echo "Access at http://localhost:8888"; \
	echo "Press Ctrl+C to stop port forwarding"; \
	$(KUBECLT) --kubeconfig=$(KUBECONFIG) port-forward service/jupyter-$$SERVER_NAME-service 8888:8888

# Get logs from a Jupyter server
.PHONY: logs
logs:
	@read -p "Enter server name: " SERVER_NAME; \
	if [ -z "$$SERVER_NAME" ]; then \
		echo "Server name cannot be empty"; \
		exit 1; \
	fi; \
	echo "Getting logs for jupyter-$$SERVER_NAME..."; \
	$(KUBECLT) --kubeconfig=$(KUBECONFIG) logs -l app=jupyter-server,instance=$$SERVER_NAME -f

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
	@echo ""
	@echo "Development:"
	@echo "  sync                - Sync the UV environment"
	@echo "  fix-all             - Run formatter and linter fix"
	@echo "  check-all           - Run linter without fix, type-checker and unit tests"
	@echo "  run-all             - Run formatter and linter fix, type-check and unit tests"
	@echo ""
	@echo "Deployment:"
	@echo "  local-dev-setup     - Set up local cluster and configure kubectl access"
	@echo "  build               - Build the image and push to local finch cluster"
	@echo "  apply-crd           - Manually apply CRD to the cluster"
	@echo "  delete-crd          - Delete the old CRD from the cluster, no-op if does not exist"
	@echo "  local-deploy        - Set up local cluster, configure kubectl access, build and install Helm chart locally"
	@echo "  local-dev-teardown  - Tear down local development environment"
	@echo ""
	@echo "Testing:"
	@echo "  operator-tests      - Test basic operation on the custom operators"
	@echo ""
	@echo "Jupyter Server Management:"
	@echo "  list-servers        - List all Jupyter servers and their status"
	@echo "  port-forward        - Interactive port forward to a Jupyter server"
	@echo "  port-forward-custom - Port forward with custom local port"
	@echo "  port-forward-sample - Quick port forward to sample-notebook server"
	@echo "  show-access         - Show access instructions for a server"
	@echo "  watch-servers       - Watch Jupyter server status in real-time"
	@echo "  logs                - Get logs from a Jupyter server"
	@echo ""
	@echo "Cleanup:"
	@echo "  clean               - Clean build artifacts"