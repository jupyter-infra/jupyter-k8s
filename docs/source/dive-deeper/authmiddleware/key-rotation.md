# Key Rotation

**Auth middleware**'s HMAC signing keys rotate automatically via a dedicated CronJob, separate from the **Extension API**'s rotator.

## Setup

The helm charts deploy the following to support **Auth middleware**:
- A Kubernetes Secret (default name: `authmiddleware-secrets`) in the router namespace
- A CronJob running the rotator image

## Rotation behavior

The rotator follows the same mechanism as the **Extension API** rotator:

1. Generates a new HMAC key and appends it to the Secret.
2. Retains up to a configurable number of keys (default: 3).
3. Prunes the oldest key when the limit is exceeded.

## Graceful transition

**Auth middleware** pods its signing Secret via controller-runtime event handlers. On update:

1. All keys are reloaded from the Secret.
2. A `newKeyUseDelay` (default: 5 seconds) prevents using the new key for signing until all replicas have observed it.
3. Tokens carry a `kid` header — validation looks up the key by ID, so tokens signed with a previous key remain valid as long as that key is retained.

Because session tokens have a 1-hour TTL (default) and refresh happens transparently, users experience no interruption during rotation.

## Why separate from the Extension API

The **Auth middleware** and **Extension API** run in different namespaces and serve different purposes:

| | Extension API | Auth Middleware |
|---|---|---|
| Namespace | `jupyter-k8s-system` | Router namespace (e.g. `jupyter-k8s-router`) |
| Token type | `bootstrap` (short-lived, 5 min) | `session` (longer-lived, 1 hour) |
| Issuer | `workspaces-controller` | `workspaces-auth` |
| Secret | `extensionapi-jwt-secrets` | `authmiddleware-secrets` |

Each component only trusts tokens it signed itself. **Auth middleware** validates **Extension API** bearer tokens by calling [`BearerTokenReview`](../../concepts/connections/token-review) — it never validates them locally.
