# Jupyter-k8s Developer Guide

## Project Overview

Jupyter-k8s is a Kubernetes operator for Jupyter notebooks and other IDEs. It manages compute, storage, networking, and access control for multiple users in a secure, scalable, usable and flexible way.
- the project is live, be mindful of backward compatibility.

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

### Plugin Architecture
**Code:** `internal/plugin`, `internal/pluginclient`, `internal/pluginserver`, `internal/pluginadapters`, `internal/awsadapter`, `internal/awsplugin`

Cloud-provider operations are decoupled from the core operator via an HTTP sidecar plugin pattern.
The controller is the HTTP client; the plugin runs as a sidecar on `localhost` in the same pod.

- **`internal/plugin/`**: Shared interfaces (`RemoteAccessPluginApis`, `JwtPluginApis`) and utilities (`ParseHandlerRef`) used by both client and server sides
- **`internal/pluginclient/`**: `PluginClient` — HTTP client that implements the plugin interfaces by calling sidecar endpoints
- **`internal/pluginserver/`**: HTTP server framework that routes requests to handler implementations
- **`internal/pluginadapters/`**: Controller-side adapter interfaces (`PodEventPluginAdapter`) for pod lifecycle orchestration (pod exec, state files, restart detection)
- **`internal/awsadapter/`**: AWS-specific adapter (`AwsSsmPodEventAdapter`) — orchestrates SSM registration using `pluginclient` for SDK calls and pod exec for k8s operations
- **`internal/awsplugin/`**: AWS SDK handler implementations (SSM client, remote access routes), compiled into `cmd/aws-plugin/`

Plugin routing is driven by `AccessStrategy` fields: `PodEventsHandler` (e.g. `"aws:ssm-remote-access"`) and `CreateConnectionHandlerMap` (e.g. `vscode-remote: "aws:createSession"`).
The controller parses the `plugin:action` format and dispatches to the matching `PluginClient`.

### JWT Key Rotator
**Code:** `internal/rotator/`

Deployed as a CronJob to rotate the HMAC signing keys stored in a Kubernetes secret.
- Used by both `extensionapi` and `authmiddleware`.
- Each has its own CronJob and Kubernetes secret; deployed by different charts and may live in different namespaces.

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

### When changing any go code
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

All AWS make targets read `AWS_REGION` and `EKS_CLUSTER_NAME` from `.env`, command line, or defaults in `Makefile`.

- setup: `make setup-aws EKS_CLUSTER_NAME=<cluster> AWS_REGION=<region>`
- deploy (CRDs+controller): `make deploy-aws`
- interact: `make port-forward`
- deploy oidc chart: `make deploy-aws-traefik-dex`
- deploy sagemaker chart: `make deploy-aws-hyperpod`

**Switching clusters:**
- set `AWS_REGION` and `EKS_CLUSTER_NAME` in `.env`
- call `make kubectl-aws` to switch kubectl context
All subsequent make targets use the `.env` values.

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