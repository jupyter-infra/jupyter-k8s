# Architecture

**Auth middleware** runs as a standalone deployment in the router namespace (e.g. `jupyter-k8s-router`), alongside the reverse proxy and identity provider deployments.

## Deployment

Unlike **Extension API** (which runs in the controller pod), **Auth middleware** is a separate binary with its own image (`jupyter-k8s-authmiddleware`). It is deployed separately.

For example, the {ref}`AWS-OIDC <chart-aws-oidc>` guided chart bundles it with Traefik, Dex and Oauth2-Proxy.

## Configuration

An **Auth middleware** container reads configuration from environment variables. Key settings:

### Server

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP listen port |
| `NAMESPACE` | — | Namespace where the middleware runs (for Secret access) |
| `TRUSTED_PROXIES` | `0.0.0.0/0` | CIDRs allowed to set forwarded headers |

### Authentication

| Variable | Default | Description |
|----------|---------|-------------|
| `ENABLE_OAUTH` | `true` | Enable the `/auth` OIDC endpoint |
| `ENABLE_BEARER_URL_AUTH` | `false` | Enable the `/bearer-auth` endpoint |
| `OIDC_ISSUER_URL` | — | OIDC provider discovery URL |
| `OIDC_CLIENT_ID` | — | OIDC client ID for token validation |

### Routing

| Variable | Default | Description |
|----------|---------|-------------|
| `ROUTING_MODE` | `path` | `path` or `subdomain` — how workspace identity is extracted from the URL |
| `PATH_REGEX_PATTERN` | `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$` | Regex to extract the workspace path prefix |
| `WORKSPACE_NAMESPACE_PATH_REGEX` | `^/workspaces/([^/]+)/[^/]+` | Regex to extract namespace from path |
| `WORKSPACE_NAME_PATH_REGEX` | `^/workspaces/[^/]+/([^/]+)` | Regex to extract workspace name from path |

## Integration with the reverse proxy

The middleware exposes HTTP endpoints that the reverse proxy calls via forward-auth middleware configuration. For example, with Traefik:

```yaml
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: workspace-auth
spec:
  forwardAuth:
    address: http://authmiddleware:8080/verify
    trustForwardHeader: true
```
