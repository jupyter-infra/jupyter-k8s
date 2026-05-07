# Workspace Defaults

The mutating webhook applies defaults and adds metadata to workspaces before they reach etcd. It fires on `CREATE` and `UPDATE`.

## Steps

| Step | What it does |
|------|--------------|
| Ownership annotations | Sets `created-by` (on CREATE) and `last-updated-by` from the request user |
| Template resolution | Resolves the template reference and applies its defaults (resources, storage, env, scheduling, lifecycle, access strategy) |
| Service account | Applies the default service account from the template if the workspace doesn't specify one |
| Sharing defaults | Sets `ownershipType` and `accessType` to their default values if unset |
| Template finalizer | Adds a finalizer to the referenced template (lazy pattern — only when active workspaces use it) |
| Access strategy finalizer | Adds a finalizer to the referenced access strategy to prevent deletion while in use |

## Lazy finalizers

The webhook adds finalizers to templates and access strategies **at mutation time**, not when those resources are first created. This means:
- A template with no workspaces carries no finalizer and can be freely deleted.
- The first workspace that references a template triggers the finalizer addition.
- The controller removes the finalizer when the last workspace stops using the template.

The same pattern applies to access strategies.
