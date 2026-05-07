# Workspace Lifecycle

A workspace moves through a series of states from creation to availability (and optionally to stopped). The controller drives these transitions by reconciling the `workspace.spec` against the actual resource state.

## Condition types

| Condition | Meaning |
|-----------|---------|
| `Available` | The workspace is fully functional — pod running, access probe passed, ready to accept connections |
| `Progressing` | Resources are being created, updated, or stopped |
| `Degraded` | The workspace failed to reach or maintain its desired state (e.g. access probe exceeded failure threshold) |
| `Stopped` | The workspace has been stopped; the pod is removed but storage is preserved |

Each condition's status is one of `True`, `False`, or `Unknown`.

## Typical progression

1. User creates or starts a workspace (`desiredStatus: Running`).
2. Controller sets `Progressing=True` while creating the deployment, service, and access resources.
3. If the workspace references an access strategy with an [access startup probe](access-probes), the controller waits for it to pass.
4. On probe success: `Available=True`, `Progressing=False`.
5. On probe failure (threshold exceeded): `Degraded=True`, `Available=False`.

## Status fields

Beyond conditions, the workspace status includes:

| Field | Purpose |
|-------|---------|
| `status.deploymentName` | Name of the managed Deployment |
| `status.serviceName` | Name of the managed Service |
| `status.accessURL` | URL at which the workspace can be reached (when routing is configured) |
| `status.accessResources` | Status of each resource created from the access strategy templates |
| `status.observedAccessStrategyVersion` | Identity and version of the access strategy last evaluated; the controller resets probe state when this changes |
| `status.accessStartupProbeSucceeded` | Whether the access probe has passed |
| `status.accessStartupProbeFailures` | Consecutive probe failure count |

```{toctree}
:hidden:

access-probes
idle-shutdown
```
