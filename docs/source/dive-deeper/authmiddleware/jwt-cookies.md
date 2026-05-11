# JWT & Cookies

**Auth middleware** issues and validates JWT session tokens stored in HTTP cookies.

These tokens provide stateless authentication on every proxied request.

![JWT lifecycle](/_static/img/diagrams/jwt-lifecycle.svg)

## Token lifecycle

1. **Issued** — when a user authenticates via {ref}`/auth <authmiddleware-auth>` (OIDC) or {ref}`/bearer-auth <authmiddleware-bearer-auth>` (bearer token exchange).
2. **Validated** — on every request via {ref}`/verify <authmiddleware-verify>`.
3. **Refreshed** — transparently when nearing expiration (if refresh is enabled and access review passes).
4. **Expired** — after `jwtExpiration` elapses, requiring re-authentication.

## Token claims

| Claim | Description |
|-------|-------------|
| `user` | Kubernetes username |
| `groups` | Group memberships |
| `uid` | Kubernetes user UID |
| `extra` | Additional user info from the K8s auth layer (e.g. OIDC claims, impersonation metadata) |
| `path` | Workspace path prefix this token authorizes |
| `domain` | Host this token is valid for |
| `type` | `session` (distinguishes from Extension API `bootstrap` tokens) |
| `iss` | Issuer — `workspaces-auth` (default) |
| `aud` | Audience — `workspace-users` (default) |
| `exp` | Expiration time |
| `iat` | Issued-at time |

## Cookie configuration

| Setting | Default | Description |
|---------|---------|-------------|
| `COOKIE_NAME` | `workspace_auth` | Cookie name |
| `COOKIE_SECURE` | `true` | HTTPS only |
| `COOKIE_HTTP_ONLY` | `true` | Not accessible to JavaScript |
| `COOKIE_SAME_SITE` | `Lax` | CSRF protection |
| `COOKIE_MAX_AGE` | 24 hours | Browser-side expiry |

**Auth middleware** scopes the cookies to the workspace path — each workspace gets its own cookie. This prevents cookies from one workspace being sent with requests to another.

## Token refresh

| Setting | Default | Description |
|---------|---------|-------------|
| `JWT_REFRESH_ENABLE` | `true` | Enable transparent token refresh |
| `JWT_REFRESH_WINDOW` | 15 minutes | Refresh if token expires within this window |
| `JWT_REFRESH_HORIZON` | 12 hours | Maximum total session duration before forced re-authentication |

During a refresh, **Auth middleware** re-checks authorization via [`ConnectionAccessReview`](../../concepts/connections/access-review). If the user's access has been revoked (workspace deleted, access type changed to OwnerOnly, RBAC removed), the refresh fails and the cookie is cleared.

## Signing

**Auth middleware** uses HMAC symmetric signing with keys stored in a Kubernetes Secret (`JWT_SECRET_NAME`, default: `authmiddleware-secrets`). The middleware watches this Secret for changes — when the rotator adds a new key, each auth middleware pod picks it up without restart.

Each token carries a `kid` (key ID) in its JWT header. On validation, the middleware looks up the corresponding key by `kid` — so multiple keys can coexist during rotation without trial-and-error.

## Separation from Extension API

**Auth middleware** and **Extension API** each have their own:
- JWT signing Secret
- Rotator CronJob
- Issuer/audience values

They may run in different namespaces. **Auth middleware**'s tokens (`type: session`, issuer: `workspaces-auth`) are distinct from the **Extension API**'s tokens (`type: bootstrap`, issuer: `workspaces-controller`).
