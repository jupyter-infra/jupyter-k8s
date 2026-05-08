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
