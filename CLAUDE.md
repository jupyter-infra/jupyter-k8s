# Jupyter-k8s Developer Guide

## Project Overview

Jupyter-k8s is a Kubernetes operator for Jupyter notebooks and other IDEs. It manages compute, storage, networking, and access control for multiple users in a secure, scalable, usable and flexible way.
- the project is not live yet, do not worry about backward compatibility.

### Kubernetes Custom Resources

- **Workspace**: A compute unit with dedicated storage, unique URL, and access control list for users
- **WorkspaceAccessStrategy**: Handles network routing with HTTPS ingress or tunneling out from workspaces
- **WorkspaceTemplate**: Provides default settings and bounds for variations
  - Template constraints are enforced **lazily** via admission webhooks during workspace CREATE/UPDATE operations
  - Templates use **lazy finalizers** - only added when workspaces reference them, removed when no workspaces use them
  - Template changes do NOT trigger proactive workspace validation (webhook validates on next workspace mutation)

## Architecture

### Kubernetes Controller
**Code:** `./internal/controller` and `./internal/webhook`

The Kubernetes controller implements the operator pattern to manage workspace resources:
- **Reconciliation Loops**: Continuously reconciles desired state with actual cluster state
- **Custom Resource Management**: Creates, updates, and deletes underlying resources
- **Status Updates**: Reports resource status back to the custom resources
- **Event Recording**: Logs significant events for auditing and troubleshooting
- **Admission Webhooks**: Validates and mutates resources before they are stored in etcd

### Extension API
**Code:** `./internal/extensionapi`

In addition to the controller, the jupyter-k8s operator starts an extension API server in the
same pod as the controller, and managed by the same manager.

The extension API serves APIs under `connection.workspace.jupyter.org` that cannot be represented
as CRD.
- `create Connection`
- `create ConnectionAccessReview`
`ConnectionAccessReview` performs an RBAC check on a virtual resource `workspaces/connection` in
`workspace.jupyter.org` API group, and simulates the validation webhook.

### Auth Middleware
**Code:** `./internal/authmiddleware`

The auth middleware component provides authentication and authorization for workspace access:

- **JWT-Based Authentication**: Uses JSON Web Tokens (JWT) for stateless authentication
- **Path-Based Authorization**: Tokens are scoped to specific workspace paths
- **Token Refresh**: Transparently refreshes tokens within a configurable window
- **Cookie Management**: Handles secure cookie storage with path-specific scopes

#### Endpoints

- `/auth`: Initial authentication endpoint that generates JWT tokens from proxy headers
  - Takes user and group information from headers
  - Creates a token scoped to the workspace path

- `/verify`: Token verification endpoint for validating requests
  - Verifies token validity and freshness
  - Ensures path and domain match the request
  - Refreshes tokens when nearing expiration

- `/health`: System health check endpoint for monitoring

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
- Run linter for e2e code: `make lint-e2e`
- Run end-to-end tests (creates a separate kind cluster): `make test-e2e`
- Run focused e2e tests: `make test-e2e-focus FOCUS="<selector name>"` (e.g., `FOCUS="Workspace Access Strategy"`)

## Notes

- The project uses Kubebuilder with the Helm extension
- Default container runtime is Finch (configurable via CONTAINER_TOOL). Note that GitHub hooks run with `docker`.
- Uses golangci-lint for Go code linting
- E2E tests create a separate Kind cluster