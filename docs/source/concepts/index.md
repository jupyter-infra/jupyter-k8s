# Concepts

How the components of **Jupyter K8s** fit together — from custom resources to network routing.

**Jupyter K8s** manages three custom resources:
- **Workspace**: Represents a single compute environment — a pod with dedicated storage and possibly a unique URL.
- **WorkspaceTemplate**: Provides default configuration to a workspace, and enforces bounds for variations.
- **WorkspaceAccessStrategy**: Configures a workspace so that the routing layers can connect to it.

## 10k View

![Architecture overview](/_static/img/diagrams/architecture-overview.svg)

## Components

| Component | Role | Deployment |
|-----------|------|------------|
| **Controller** | Reconciles Workspace CRs into deployments, services, and routing resources | `jupyter-k8s-system` namespace |
| **Extension API** | Serves the Connection APIs (aggregated into the K8s API server) | Same pod as controller |
| **Router** | Reverse proxy (e.g. Traefik) that routes HTTPS traffic to workspaces | `jupyter-k8s-router` namespace |
| **Auth Middleware** | Validates JWTs and enforces per-workspace authorization on every request | Same namespace as router |
| **Rotator** | Runs in a CronJob to rotate HMAC signing keys in a Kubernetes Secret | One per JWT secret |

## How they connect

```text
kubectl ──► K8s API Server ──► Controller ──► (pods, services, ingress routes)

Browser ──► Router ──► Auth Middleware ──► Workspace Pod
                                │
                            Extension API
                        (connections, bearer-URL)
```

1. A user creates a **Workspace** resource via `kubectl` (or any K8s client).
2. The **Controller** reconciles the resource — creating a deployment, service, and routing resources according to the workspace's **Access Strategy**.
3. The user obtains a connection URL (from `status.accessURL` or via the `Create:Connection` API).
4. The **Router** receives the HTTPS request, delegates authorization to the **Auth Middleware** (when in use), then proxies the verified request to the workspace pod.

## Namespace layout

**Jupyter K8s** separates concerns across namespaces (default values below):

- **`jupyter-k8s-system`** — controller deployment (controller + **Extension API**) and its JWT signing secret.
- **`jupyter-k8s-router`** — reverse proxy, auth middleware, identity provider (if using OIDC), and its own JWT signing secret.
- **`jupyter-k8s-shared`** - a [special namespace](../concepts/templates/shared-namespace) for templates and access strategies that can be referenced by any workspace in the cluster. 
- **Workspace namespaces** — one or more namespaces where the workspace resources, as well as their pods, services, and ingress routes live.

```{toctree}
:hidden:

workspaces/index
routing/index
access-strategies/index
templates/index
connections/index
```
