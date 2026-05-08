# Workspace Validation

The validating webhook enforces constraints on workspace create, update, and delete. Some checks apply to all users; others the controller service account and cluster admins bypass.

## Always enforced

| Check | Description |
|-------|-------------|
| Template constraints | Validates resources, images, storage size, and idle shutdown bounds against the template's constraint fields |
| Access strategy namespace | Rejects references to access strategies outside the workspace's own namespace or the configured shared namespace |
| Volume ownership | Rejects references to other workspaces' primary storage PVCs (secondary storage can be shared freely) |

## Bypassed for controller/admins

| Check | Description |
|-------|-------------|
| Reserved prefixes | Rejects user-submitted labels or annotations with operator-reserved prefixes |
| Service account access | Rejects workspaces that specify a service account the user cannot use |
| Ownership permission | For `OwnerOnly` workspaces, rejects updates and deletes from non-owners |

## Ownership enforcement

When a workspace has `ownershipType: OwnerOnly`:
- Only the user in the `created-by` annotation can update or delete it.
- Changing a workspace **to** `OwnerOnly` also requires being the original creator.
- The controller and cluster admins always bypass this check.

## Deletion validation

On `DELETE`, the webhook only checks ownership permission for `OwnerOnly` workspaces. All other deletes pass through (RBAC is the primary guard).

## Lazy constraint enforcement

The webhook enforces template constraints at **admission time** — when a user creates or updates a workspace.

Changing constraints on a template does not immediately impact running workspaces. Instead, the template controller marks affected workspaces for compliance checking.
