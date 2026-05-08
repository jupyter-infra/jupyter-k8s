# Authmiddleware

**Auth middleware** sits between the reverse proxy (e.g. Traefik) and workspace pods.

It authorizes workspace access and manages JWT session cookies.

## Role in the routing layer

```text
Browser ──► Router ──► Auth Middleware ──► Workspace Pod
                            │
                        Extension API
            (ConnectionAccessReview, BearerTokenReview)
```

The reverse proxy delegates authorization decisions to **Auth middleware** via forward-auth. On every request, the proxy sends the request headers to the middleware's {ref}`/verify <authmiddleware-verify>` route before forwarding traffic to the workspace.

## Fast and slow routes

**Auth middleware** has two categories of routes:

| Route | Category | When it runs | What it does |
|-------|----------|--------------|--------------|
| {ref}`/verify <authmiddleware-verify>` | Fast | Every proxied request | Validates the JWT cookie locally (signature + expiry + path). No network calls unless a token refresh is needed. |
| {ref}`/auth <authmiddleware-auth>` | Slow | First request (no session) | Verifies an OIDC token with the identity provider, calls [`ConnectionAccessReview`](../../concepts/connections/access-review), and issues a session cookie. |
| {ref}`/bearer-auth <authmiddleware-bearer-auth>` | Slow | First request (bearer URL) | Calls [`BearerTokenReview`](../../concepts/connections/token-review) on the Extension API, then issues a session cookie. |

This separation matters for performance: the fast route handles the vast majority of requests with a purely local JWT validation (no I/O). The slow routes only run once per session establishment and involve external calls (OIDC provider, **Extension API**). This keeps per-request latency minimal even under high load.

```{toctree}
:hidden:

architecture
routes
jwt-cookies
key-rotation
```
