# Access Types

Every workspace has two permission control dimensions.

## Ownership type (`spec.ownershipType`)

Controls who can **modify or delete** the workspace.

| Value | Meaning |
|-------|---------|
| `Public` | Any user with the appropriate RBAC permissions can update or delete the workspace |
| `OwnerOnly` | Only the creator (a Kubernetes username) can update or delete; note that RBAC permission also applies |

## Access type (`spec.accessType`)

Controls who can **connect** to the workspace (open it in a browser or desktop IDE).

| Value | Meaning |
|-------|---------|
| `Public` | Any user with RBAC `workspaces/connection` permission in the namespace can connect |
| `OwnerOnly` | Only the creator (a Kubernetes username) can create connections; note that RBAC permission also applies |

Both default to `Public` when unset (or when the template's defaults apply).

## How access is enforced

The **Extension API** enforces these rules at connection time, either directly when it handles a [`Create:Connection`](../connections/index) request, or by handling a [`Create:ConnectionAccessReview`](../connections/access-review) coming from an authorization component, such as the auth middleware.

**Extension API** checks both RBAC permission and the `workspace.spec.accessType` attribute before issuing a connection URL, a bearer token URL, or setting a `ConnectionAccessReview.status.Allowed` attribute to `true`.

When in use, the auth middleware re-validates on every request using the JWT claims embedded at connection time.

## RBAC example

To grant a user permission to connect to workspaces in a namespace, create a Role and RoleBinding:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: workspace-user
  namespace: team-alice
rules:
  - apiGroups: ["connection.workspace.jupyter.org"]
    resources: ["workspaceconnections"]
    verbs: ["create"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: alice-workspace-user
  namespace: team-alice
subjects:
  - kind: User
    name: alice
roleRef:
  kind: Role
  name: workspace-user
  apiGroup: rbac.authorization.k8s.io
```

This grants `alice` the ability to create connections (and therefore access workspaces) in the `team-alice` namespace. The `workspace.spec.accessType` further narrows access — even with RBAC permission, an `OwnerOnly` workspace only allows its creator to connect.
