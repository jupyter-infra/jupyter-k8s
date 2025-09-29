# Jupyter-k8s Developer Guide

## Project Overview

Jupyter-k8s is a Kubernetes operator for Jupyter notebooks and other IDEs. It manages compute, storage, networking, and access control for multiple users in a secure, scalable, usable and flexible way.

### Core Custom Resources

- **Workspace**: A compute unit with dedicated storage, unique URL, and access control list for users
- **WorkspaceManager**: Handles routing, authentication, and authorization for Workspaces
- **WorkspaceTemplate**: Provides default settings and bounds for variations
- **WorkspaceExecutionProfile**: Provides k8s and potentially other identities to a Workspace at runtime
- **WorkspaceShare**: Associates Workspaces with k8s identities for sharing access

### Deployment Modes

1. **Guided Mode**: Helm chart creates all required resources:
   - Reverse proxy
   - Auth middlewares
   - Identity provider with OAuth
   - Namespaces, RBAC, Service Accounts, limits and quotas
   - Basic images and templates for IDEs and sidecars

2. **Customized Mode**: Admins create their own configuration and reference them in custom resources

## Common Development Tasks

### Getting setup
- install: `make deps`
- create local kind cluster: `make setup-kind`

### When changing controller code
- build: `make build`
- lint: `make lint-fix`
- unit tests: `make test`

### When changing helm chart
- modify only files under `/api`, `/config` and possibly `/hack` (for patching `values.yaml` and `manager.yaml`)
- generate: `make helm-generate`, which outputs helm files to `dist/chart`
- lint: `make helm-lint`
- test: `make helm-test`, then observe results in `dist/test-output-crd-only` dir

### End to end testing against local cluster
- deploy chart to local cluster: `make deploy-kind`
- interact: `make port-forward`

### End to end testing against remote cluster (aws)
- setup: `make setup-aws`
- deploy: `make deploy-aws`
- interact: `make port-forward`

### Clean up
Ask user before running.
- `make teardown-kind`

### Before submitting a PR
- Build: `make build`
- Run linter: `make lint`
- Run unit tests: `make test`
- Run end-to-end tests (creates a separate kind cluster): `make test-e2e`

## Notes

- The project uses Kubebuilder with the Helm extension
- Default container runtime is Finch (configurable via CONTAINER_TOOL). Note that GitHub hooks run with `docker`.
- Uses golangci-lint for Go code linting
- E2E tests create a separate Kind cluster