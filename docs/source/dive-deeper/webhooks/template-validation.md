# Template Validation

WorkspaceTemplates have a validating-only webhook that fires on `UPDATE`. It uses `failurePolicy: Ignore`, so template updates succeed even when the webhook is unavailable.

## Behavior

The webhook never blocks updates. When constraint fields change, it returns a **warning** telling the user that the template controller will re-validate affected workspaces.

## Constraint fields

Changes to any of the following fields trigger the warning:

- `allowedImages`
- `resourceBounds`
- `primaryStorage` (min/max size)
- `idleShutdownOverrides` (allow, min/max timeout)
- `envRequirements`

## Deletion

The webhook does not intercept `DELETE`. The [lazy finalizer](workspace-defaults) on the template prevents deletion while any active workspace references it.

## Access strategy protection (mutating)

A separate mutating webhook (`failurePolicy: Ignore`, create/update) handles a template's reference to an access strategy via `spec.defaultAccessStrategy`:

- it stamps access strategy lookup labels (`workspace.jupyter.org/access-strategy-name` / `-namespace`) on the template, resolving the namespace as the explicit ref namespace or the template's own namespace; and
- it adds the `workspace.jupyter.org/accessstrategy-template-protection` finalizer to the referenced access strategy.

This is the template-side counterpart of the workspace access strategy finalizer, but uses a distinct finalizer name so workspace and template references are tracked independently. Either finalizer blocks full deletion of the access strategy: Kubernetes holds it in `Terminating` until the last referrer of each kind dereferences it.

When a template removes or changes its `defaultAccessStrategy`, the controller clears the stale labels and removes the template finalizer from the previously referenced access strategy once no other template references it. The controller backfills the same labels and finalizer as a safety net for templates created before this logic existed.
