# Token Review

A **BearerTokenReview** is a request to the **Extension API** that validates an opaque bearer token and returns the authenticated identity embedded in it. It performs a similar function as the core Kubernetes API [TokenReview](https://kubernetes.io/docs/reference/kubernetes-api/authentication-resources/token-review-v1/).

Authentication components (such as the auth middleware) call `Create:BearerTokenReview` when they receive a request containing a bearer token and need to verify its authenticity.

## How it works

The caller sends a `Create:BearerTokenReview` with the opaque token string. **Extension API**:

1. **Validates the signature** — verifies the token was signed by a known HMAC key.
2. **Checks expiry** — rejects expired tokens.
3. **Checks token type** — only accepts bootstrap tokens (bearer tokens used for initial authentication), not session tokens.

If all checks pass, the response includes the authenticated user identity and the workspace path the token was scoped to.

## Request

```yaml
apiVersion: connection.workspace.jupyter.org/v1alpha1
kind: BearerTokenReview
spec:
  token: "<opaque-bearer-token>"
```

| Field | Purpose |
|-------|---------|
| `spec.token` | The opaque bearer token to validate |

## Response

**Extension API** returns the same object with `status` populated:

```yaml
status:
  authenticated: true
  path: "/workspaces/team-alice/alice-workspace/"
  domain: "jupyter.example.com"
  user:
    username: alice
    groups:
      - team-alice
      - system:authenticated
```

| Field | Meaning |
|-------|---------|
| `status.authenticated` | Whether the token is valid |
| `status.path` | The workspace path the token is scoped to |
| `status.domain` | The domain the token is scoped to |
| `status.user.username` | The Kubernetes username embedded in the token |
| `status.user.groups` | The Kubernetes groups embedded in the token |
| `status.user.uid` | (optional) The UID embedded in the token |
| `status.error` | Error message if authentication failed |

## Who calls it

**Auth middleware** calls `Create:BearerTokenReview` during bearer-token authentication (its [`/bearer-auth` route](../../dive-deeper/authmiddleware/routes)) — when a user first accesses a workspace using a bearer token URL. Once the token is verified, the middleware issues a signed JWT session cookie. Subsequent requests use the cookie and do not require another token review.

The RBAC permissions required by **Auth middleware**'s service account are:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
rules:
  - apiGroups: ["connection.workspace.jupyter.org"]
    resources: ["bearertokenreviews"]
    verbs: ["create"]
```

## Difference from ConnectionAccessReview

| | `Create:BearerTokenReview` | `Create:ConnectionAccessReview` |
|-|---------------------------|---------------------------------|
| **Purpose** | Authenticate a bearer token and extract the identity | Authorize a known identity against a workspace |
| **Input** | Opaque token string | Username, groups, workspace name |
| **Output** | Authenticated identity + path/domain | Allowed/denied + reason |
| **When called** | Bearer token flow (first request with token in URL) | After authentication, to check authorization |
