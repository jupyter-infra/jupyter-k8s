# Architecture

**Auth middleware** runs as a standalone deployment in the router namespace (e.g. `jupyter-k8s-router`), alongside the reverse proxy and identity provider deployments.

## Deployment

Unlike **Extension API** (which runs in the controller pod), **Auth middleware** is a separate binary with its own image (`jupyter-k8s-authmiddleware`). It is deployed separately.

For example, the {ref}`AWS-OIDC <chart-aws-oidc>` guided chart bundles it with Traefik, Dex and 
Oauth2-Proxy.

## Flows

### Initial connection with OIDC

![OIDC web access flow](/_static/img/diagrams/web-access-oidc.svg)

### Initial connection with Bearer Token URL

![Bearer token web access flow](/_static/img/diagrams/web-access-bearer-token.svg)

### Subsequent connections

![Subsequent access flow](/_static/img/diagrams/jwt-read.svg)

## Configuration

An **Auth middleware** container reads configuration from environment variables. Key settings:

### Server

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP listen port |
| `NAMESPACE` | — | Namespace where the middleware runs (for Secret access) |
| `TRUSTED_PROXIES` | `0.0.0.0/0` | CIDRs allowed to set forwarded headers |
| `TLS_CERT_FILE` | — | Path to a PEM certificate. Set together with `TLS_KEY_FILE` to serve HTTPS |
| `TLS_KEY_FILE` | — | Path to the PEM private key matching `TLS_CERT_FILE` |

### TLS

**Auth middleware** serves plain HTTP by default. When you set both `TLS_CERT_FILE` and `TLS_KEY_FILE`, it serves HTTPS on `PORT` instead, including the `/health` route, and accepts TLS 1.3 only. Setting only one of the two is a configuration error.

**Auth middleware** loads the keypair at startup and exits if either file is unreadable or the certificate does not match the key. It then watches both files and serves a renewed certificate without a restart, so you can mount a cert-manager `Certificate` Secret directly. Switch the container probes to `scheme: HTTPS`, and point the reverse proxy at an `https://` address.

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
