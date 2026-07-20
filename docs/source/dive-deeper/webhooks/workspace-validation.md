# Workspace Validation

The validating webhook enforces constraints on workspace create, update, and delete. Some checks apply to all users; others the controller service account and cluster admins bypass.

## Always enforced

| Check | Description |
|-------|-------------|
| Template constraints | Validates resources, images, storage size, and idle shutdown bounds against the template's constraint fields |
| Reference namespace scope | Rejects references to templates or access strategies outside the workspace's own namespace or the configured shared namespace |
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

## Idle shutdown override enforcement

When the template sets `idleShutdownOverrides.allow: false`, the webhook requires the workspace's `idleShutdown` to match the template's `defaultIdleShutdown` on every field except `idleTimeoutInMinutes`. Declared `minIdleTimeoutInMinutes`/`maxIdleTimeoutInMinutes` bounds are enforced on any *enabled* idle shutdown regardless of `allow`; a disabled idle shutdown has no timeout to bound. See [idle shutdown bounds](../../concepts/templates/bounds.md) for the policy semantics.

## Lazy constraint enforcement

The webhook enforces template constraints at **admission time**, when a user creates or updates a workspace.

Changing constraints on a template does not immediately impact running workspaces. Instead, the template controller marks affected workspaces for compliance checking.

## Protection finalizers on referenced resources

The mutating webhook protects the resources a workspace depends on from being deleted while still in use. When a workspace references a template, the webhook stamps a `workspace.jupyter.org/template-protection` finalizer on that template; when it references an access strategy, it stamps a `workspace.jupyter.org/accessstrategy-protection` finalizer on that access strategy. These are added **lazily**, only once a referencing workspace exists. The webhook also rejects the workspace if the referenced resource does not exist.

The webhook only adds finalizers; it never removes them. Removal stays with the template and access strategy controllers, which strip a protection finalizer once the last referring workspace (and, for access strategies, the last referring template) is gone.