# Bearer Token

**Extension API** signs short-lived JWT bearer tokens when creating `web-ui` connections. These tokens bootstrap the user's session — they are not session tokens themselves.

## Bearer token flow

1. A user creates a `WorkspaceConnection` with type `web-ui`.
2. **Extension API** signs a JWT with the user's identity, scoped to the workspace's path and domain.
3. The token is embedded in a URL (rendered from the access strategy's `bearerAuthURLTemplate`).
4. The user opens this URL in their browser.
5. **Auth middleware** validates the token via a [`BearerTokenReview`](../../concepts/connections/token-review) call back to **Extension API**, then issues a long-lived session cookie.

## Token properties

| Property | Value | Notes |
|----------|-------|-------|
| Type | `bootstrap` | Distinguishes from session tokens |
| TTL | 5 minutes (default) | Short-lived — meant for immediate use |
| Issuer | `workspaces-controller` | Identifies the Extension API |
| Audience | `workspaces-controller` | Validated by BearerTokenReview |
| Skip refresh | `true` | Bootstrap tokens are never refreshed |

## Claims

The bearer token includes:
- **Subject** — the Kubernetes username
- **Groups** — the user's group memberships
- **UID** — the Kubernetes user UID
- **Extra** — additional user info from the K8s auth layer
- **Path** — workspace path prefix (e.g. `/workspaces/team-alice/my-notebook`)
- **Domain** — the host the token is valid for

## Signing

**Extension API** uses a `CompositeSignerFactory` that supports multiple signing backends:

- **k8s-native** — HMAC signing with keys stored in a Kubernetes Secret. This is the default when `extensionApi.jwtSecret.enable=true`.
- **Plugin-delegated** — signing delegated to a [plugin](../../integrations/plugins/index.md) via `JwtPluginApis`. Used when an access strategy references a plugin-backed signer.

The signer selection depends on the access strategy configuration.

## BearerTokenReview

When **Auth middleware** receives a bearer token URL, it calls **Extension API**'s `bearertokenreviews` endpoint:

1. **Extension API** extracts the `kid` from the token header and validates the signature against the corresponding key.
2. It checks that the token has not expired.
3. It returns the authenticated user identity (username, groups, UID, extra, path).
4. **Auth middleware** uses this identity to issue a session cookie.
