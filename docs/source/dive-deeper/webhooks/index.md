# Webhooks

**Jupyter K8s** registers admission webhooks with the Kubernetes API server to default and validate resources before persisting them.

## Overview

| Webhook | Resource | Type | Failure Policy | Verbs |
|---------|----------|------|----------------|-------|
| [Workspace defaults](workspace-defaults) | `workspaces` | Mutating | `Fail` | create, update |
| [Workspace validation](workspace-validation) | `workspaces` | Validating | `Fail` | create, update, delete |
| [Template validation](template-validation) | `workspacetemplates` | Validating | `Ignore` | update |
| Pod exec | `pods/exec` | Validating | `Ignore` | connect |

## Pod exec webhook

A validating webhook intercepts `pods/exec` requests (`connect` verb) to restrict the controller's service account:

- **Non-controller users** — the webhook allows all exec requests without further checks.
- **Controller service account** — the webhook only allows exec into pods that carry the workspace label.

This prevents users from using the controller as a vector to exec into arbitrary pods.

```{toctree}
:hidden:

workspace-defaults
workspace-validation
template-validation
```
