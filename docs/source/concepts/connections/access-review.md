# Access Review

A **ConnectionAccessReview** is a request to the Extension API that checks whether a given user is allowed to connect to a workspace — without actually creating a connection. It performs a similar function as the core Kubernetes API [SubjectAccessReview](https://dev-k8sref-io.web.app/docs/authorization/subjectaccessreview-v1/).

Authorization components (such as the auth middleware) call `Create:ConnectionAccessReview` when making decision to grant or reject access to a workspace.

## How it works

The caller sends a `Create:ConnectionAccessReview` with the subject's Kubernetes identity and the target workspace. The Extension API performs two checks in sequence:

1. **RBAC check** — does the user have permission to create `workspaces/connection` in the workspace's namespace?
2. **Workspace access check** — is the workspace public, or is the user the owner?

Both checks must pass for the review to return `status.allowed: true`.

## Request

```yaml
apiVersion: connection.workspace.jupyter.org/v1alpha1
kind: ConnectionAccessReview
metadata:
  namespace: team-alice
spec:
  workspaceName: alice-workspace
  user: alice
  groups:
    - team-alice
    - system:authenticated
```

| Field | Purpose |
|-------|---------|
| `spec.workspaceName` | The workspace to check access against |
| `spec.user` | The Kubernetes username of the subject |
| `spec.groups` | The Kubernetes groups of the subject |
| `spec.uid` | (optional) The UID of the subject |
| `spec.extra` | (optional) Extra attributes from the authentication layer |

## Response

The Extension API returns the same object with `status` populated:

```yaml
status:
  allowed: true
  reason: "Valid RBAC and the subject Workspace is public"
```

| Field | Meaning |
|-------|---------|
| `status.allowed` | Whether the user can connect |
| `status.notFound` | Whether the workspace was not found |
| `status.reason` | Human-readable explanation of the decision |

## Who calls it

The auth middleware calls `Create:ConnectionAccessReview` during initial authentication (its `/auth` route) — when a user first accesses a workspace and no valid session cookie exists yet. Once authorized, it issues a signed JWT cookie. Subsequent requests hit the `/verify` route, which only validates the JWT signature and expiry — no call to the Extension API.

The RBAC permissions required by the auth middleware's service account are:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
rules:
  - apiGroups: ["connection.workspace.jupyter.org"]
    resources: ["connectionaccessreviews"]
    verbs: ["create"]
```

## Difference from Create:Connection

| | `Create:Connection` | `Create:ConnectionAccessReview` |
|-|--------------------|---------------------------------|
| **Purpose** | Generate a connection URL or session | Check if access would be allowed |
| **Side effects** | May sign a bearer token, invoke plugins | None — read-only check |
| **Caller** | Workspace users (via kubectl or API) | Authorization middleware |
| **Persists** | No (virtual resource) | No (virtual resource) |
