# Web UI

The **Web UI** is a workspace management console that lets users create, monitor, and manage their Jupyter workspaces from a browser.

## What it does

1. **List workspaces** — shows the user's workspaces with real-time status (Starting, Running, Stopped).
2. **Create workspaces** — form-based creation with template selection, resource configuration, and validation against template bounds.
3. **Manage lifecycle** — start, stop, and delete workspaces.
4. **View details** — resource usage, status conditions, and workspace metadata.

The UI operates on the `workspace.jupyter.org/v1alpha1` CRD API directly — it has no dependency on the controller.

## Installation

The **Web UI** is included in the {ref}`AWS-OIDC chart <chart-aws-oidc>`. No additional installation is required when using that chart.

For standalone deployment, add the container to your cluster with a routing layer that provides OIDC authentication:

```yaml
webApp:
  repository: ghcr.io/jupyter-infra
  imageName: jupyter-k8s-ui
  imageTag: v0.1.0-rc.1
  namespace: default
```

## Requirements

The **Web UI** expects an authenticating reverse proxy upstream (e.g. OAuth2-Proxy) that:

1. Authenticates the user via OIDC.
2. Forwards the access token in the `X-Forwarded-Access-Token` header.

The container's service account does not need Kubernetes permissions — each request uses the forwarded user token to talk to the API server directly.

## Access flow

```text
Browser → Router → OAuth2-Proxy → Web UI → K8s API Server
```

- **OAuth2-Proxy** handles the OIDC login flow and sets the token header.
- **Web UI** extracts the token, creates a per-user Kubernetes client, and performs workspace CRUD.
- A session cookie layer caches the authenticated session to reduce latency on subsequent requests.

## Configuration

The container is configured via environment variables. The most common ones:

| Variable | Default | Description |
|----------|---------|-------------|
| `NAMESPACE` | `default` | Kubernetes namespace for workspace operations |
| `SESSION_ENABLED` | `true` | Enable session cookie layer |
| `SESSION_EXPECTED_DOMAIN` | (empty) | CSRF origin domain enforcement |

See the [source repository](https://github.com/jupyter-infra/jupyter-k8s-ui) for the full list of configuration options.

## Source and packages

| | |
|---|---|
| Repository | [jupyter-k8s-ui](https://github.com/jupyter-infra/jupyter-k8s-ui) |
| Image | `ghcr.io/jupyter-infra/jupyter-k8s-ui` |
