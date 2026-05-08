# Auth Middleware

The **Auth middleware** is a stateless HTTP server that authorizes every request before it reaches a workspace pod. It runs as a separate deployment in the router namespace.

## Request flow

On each incoming request the **Auth middleware**:

1. Extracts the JWT from a path-scoped cookie.
2. Verifies the token signature and expiry using the current HMAC key.
3. Confirms the token's path claim matches the requested workspace path.
4. If the token is near expiry, transparently refreshes it and sets a new cookie.
5. Returns 200 (allow) or 401 (deny) to the router's forward-auth mechanism.

## Initial authentication

When no cookie is present, the **Auth middleware** handles first-time authentication depending on the access strategy:

| Strategy | First request |
|----------|--------------|
| **OIDC** | Redirect through the identity provider (OAuth2 flow), then authorize via [`ConnectionAccessReview`](../connections/access-review) |
| **Bearer token** | Validate the bearer token via the Extension API's [`BearerTokenReview`](../connections/token-review), then issue a session cookie |

Both paths end with a signed JWT cookie scoped to the specific workspace path.

## Key rotation

**Auth middleware** pods watch a Kubernetes Secret containing the HMAC signing key. A **Rotator** CronJob periodically generates a new key and appends it to the secret.

**Auth middleware** pods accept tokens signed by any key in the secret (to handle in-flight requests during rotation) but always sign new tokens with the latest key.

## Stateless design

No session store, no database. All session state lives in the JWT cookie. This allows horizontal scaling — any **Auth middleware** pod can verify any request without coordination.

## Separation of responsibilities

**Auth middleware** delegates access decisions and bearer token validation to the **Extension API**. It never reads workspace resources or RBAC policies directly.

The JWTs that **Auth middleware** uses for session cookies are independent from the JWTs that the **Extension API** uses for bearer token URLs — each has its own signing key and rotation schedule.
