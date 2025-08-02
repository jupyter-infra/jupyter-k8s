# Variables
GO := go
HELM := helm
DOCKER := finch

# Go specific variables
BINARY_NAME := manager
CMD_DIR := ./cmd/manager
MAIN_GO := $(CMD_DIR)/main.go

# Build options (remove --network host for standard docker)
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

# Show help
.PHONY: help
help:
	@echo "Available targets:"
	@echo ""
	@echo "Development:"
	@echo "  deps              - Download Go dependencies"
	@echo "  generate          - Generate code (DeepCopy methods, etc.)"
	@echo "  generate-crd      - Generate CRDs from Go structs"
	@echo "  generate-all      - Generate both DeepCopy methods and CRDs"
	@echo "  build             - Build binary and Docker image"
	@echo "  run               - Run the controller locally"
	@echo ""
	@echo "Testing:"
	@echo "  test              - Run Go tests"
	@echo "  test-coverage     - Run tests with coverage"
	@echo "  check-all         - Format, vet, and test code"
	@echo ""
	@echo "Deployment:"
	@echo "  local-deploy      - Full deployment (CRD + controller)"
	@echo "  local-redeploy    - Rebuild and redeploy with new tag"
	@echo "  dev-restart       - Quick rebuild and restart (development)"
	@echo ""
	@echo "Management:"
	@echo "  status            - Show controller and resource status"
	@echo "  logs              - Follow controller logs"
	@echo "  clean             - Clean up all resources"
	@echo ""
	@echo "CRD Management:"
	@echo "  apply-crd         - Apply CRD to cluster"
	@echo "  delete-crd        - Delete CRD from cluster"
	@echo ""
	@echo "Variables:"
	@echo "  IMAGE_TAG=$(IMAGE_TAG)"
	@echo "  IMAGE_NAME=$(IMAGE_NAME)"
	@echo "  NAMESPACE=$(NAMESPACE)"

# Download Go dependencies
.PHONY: deps
deps:
	$(GO) mod download
	$(GO) mod tidy

# Format and fix Go code
.PHONY: fix-all
fix-all:
	$(GO) fmt ./...
	$(GO) vet ./...

# Check Go code without fixing
.PHONY: check-all
check-all:
	$(GO) fmt ./...
	$(GO) vet ./...
	@echo "Checking if code compiles..."
	$(GO) build -o /tmp/test-$(BINARY_NAME) $(MAIN_GO)
	@rm -f /tmp/test-$(BINARY_NAME)
	@echo "Compilation check passed!"
	$(GO) test ./...
	$(HELM) lint --strict helm/jupyter-k8s

# Format, check and test Go code
.PHONY: run-all
run-all:
	$(GO) fmt ./...
	$(GO) vet ./...
	$(GO) test ./...
	$(HELM) lint --strict helm/jupyter-k8s

# Set up local development environment
.PHONY: local-dev-setup
local-dev-setup:
	$(SHELL) ./local-dev/kind/setup.sh

# Run the Go application locally
.PHONY: run
run:
	$(GO) run $(MAIN_GO)

# Run Go tests
.PHONY: test
test:
	$(GO) test ./...

# Run Go tests with coverage
.PHONY: test-coverage
test-coverage:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

# Generate code (DeepCopy methods, etc.)
.PHONY: generate
generate:
	$(GO) run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.15.0 object paths="./api/..."

# Generate CRDs from Go structs
.PHONY: generate-crd
generate-crd:
	$(GO) run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.15.0 crd paths="./api/..." output:crd:artifacts:config=helm/jupyter-k8s/crds

# Generate both DeepCopy methods and CRDs
.PHONY: generate-all
generate-all: generate generate-crd

# Build the Go binary and docker image
.PHONY: build
build:
	$(GO) build -o bin/$(BINARY_NAME) $(MAIN_GO)
	$(DOCKER) $(BUILD_OPTS) build --no-cache -t $(IMAGE_NAME):$(IMAGE_TAG) .
	$(DOCKER) push $(IMAGE_NAME):$(IMAGE_TAG)

# Apply CRD
.PHONY: apply-crd
apply-crd:
	$(KUBECLT) --kubeconfig=$(KUBECONFIG) apply -f helm/jupyter-k8s/crds/servers.jupyter.org_jupyterservers.yaml

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
		--set image.tag=$(IMAGE_TAG) \
		--force

# Build and redeploy with a new image tag (forces restart)
.PHONY: local-redeploy
local-redeploy: local-dev-setup
	$(eval NEW_TAG := $(shell date +%Y%m%d-%H%M%S))
	@echo "Building with new tag: $(NEW_TAG)"
	$(MAKE) build IMAGE_TAG=$(NEW_TAG)
	$(HELM) lint --strict helm/jupyter-k8s
	$(HELM) upgrade jupyter-k8s helm/jupyter-k8s \
		--namespace $(NAMESPACE) \
		--kubeconfig $(KUBECONFIG) \
		--set image.repository=$(IMAGE_NAME) \
		--set image.tag=$(NEW_TAG) \
		--reuse-values
	@echo "Waiting for rollout to complete..."
	$(KUBECLT) --kubeconfig=$(KUBECONFIG) rollout status deployment/jupyter-k8s -n $(NAMESPACE)
	@echo "Deployment complete with image: $(IMAGE_NAME):$(NEW_TAG)"

# Quick rebuild and restart (for development)
.PHONY: dev-restart
dev-restart:
	$(eval NEW_TAG := dev-$(shell date +%H%M%S))
	@echo "Quick rebuild with tag: $(NEW_TAG)"
	$(MAKE) build IMAGE_TAG=$(NEW_TAG)
	$(KUBECLT) --kubeconfig=$(KUBECONFIG) set image deployment/jupyter-k8s jupyter-k8s=$(IMAGE_NAME):$(NEW_TAG) -n $(NAMESPACE)
	$(KUBECLT) --kubeconfig=$(KUBECONFIG) rollout status deployment/jupyter-k8s -n $(NAMESPACE)
	@echo "Controller restarted with image: $(IMAGE_NAME):$(NEW_TAG)"

# Check controller status and logs
.PHONY: status
status:
	@echo "=== Controller Pod Status ==="
	$(KUBECLT) --kubeconfig=$(KUBECONFIG) get pods -n $(NAMESPACE) -l app.kubernetes.io/name=jupyter-k8s
	@echo ""
	@echo "=== JupyterServer Resources ==="
	$(KUBECLT) --kubeconfig=$(KUBECONFIG) get jupyterserver -A
	@echo ""
	@echo "=== Recent Controller Logs ==="
	$(KUBECLT) --kubeconfig=$(KUBECONFIG) logs -n $(NAMESPACE) -l app.kubernetes.io/name=jupyter-k8s --tail=10

# Follow controller logs
.PHONY: logs
logs:
	$(KUBECLT) --kubeconfig=$(KUBECONFIG) logs -n $(NAMESPACE) -l app.kubernetes.io/name=jupyter-k8s -f

# Clean up everything
.PHONY: clean
clean:
	@echo "Cleaning up JupyterServer resources..."
	$(KUBECLT) --kubeconfig=$(KUBECONFIG) delete jupyterserver --all --all-namespaces --ignore-not-found=true
	@echo "Uninstalling Helm release..."
	$(HELM) uninstall jupyter-k8s --namespace $(NAMESPACE) --kubeconfig $(KUBECONFIG) --ignore-not-found
	@echo "Deleting CRD..."
	$(MAKE) delete-crd
	@echo "Cleanup complete"

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
.PHONY: jupyter-logs
jupyter-logs:
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
.PHONY: clean-build
clean-build:
	rm -rf bin/
	rm -f coverage.out coverage.html
	$(GO) clean