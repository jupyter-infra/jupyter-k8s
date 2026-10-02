# Workspace Lifecycle

A workspace moves through a series of states from creation to availability (and optionally to stopped). The controller drives these transitions by reconciling the `workspace.spec` against the actual resource state.

## Condition types

| Condition | Meaning |
|-----------|---------|
| `Available` | The workspace is fully functional — pod running, access probe passed, ready to accept connections |
| `Progressing` | Resources are being created, updated, or stopped |
| `Degraded` | The workspace failed to reach or maintain its desired state: the Deployment's progress deadline passed (`ComputeStalled`), the access probe exceeded its failure threshold, or a resource could not be created |
| `Stopped` | The workspace has been stopped; the pod is removed but storage is preserved |

Each condition's status is one of `True`, `False`, or `Unknown`.

## Typical progression

1. User creates or starts a workspace (`desiredStatus: Running`).
2. Controller sets `Progressing=True` while creating the deployment, service, and access resources.
3. If the workspace references an access strategy with an [access startup probe](access-probes), the controller waits for it to pass.
4. On probe success: `Available=True`, `Progressing=False`.
5. On probe failure (threshold exceeded): `Degraded=True`, `Available=False`.

## Stalled starts

The operator does not time a start itself. The Deployment it creates carries the Kubernetes default `progressDeadlineSeconds` of 600; when no replica has become ready by then, the Deployment controller marks the Deployment `Progressing=False` with reason `ProgressDeadlineExceeded`, and the workspace reports:

```yaml
conditions:
  - type: Degraded
    status: "True"
    reason: ComputeStalled
    message: "0/3 nodes are available: 3 Insufficient nvidia.com/gpu."
  - type: Available
    status: "False"
    reason: ComputeStalled
  - type: Progressing
    status: "False"
    reason: ComputeStalled
```

The message is what the pod reports: the scheduler's verdict while the pod has no node, otherwise the reason and message of the first container that is waiting (for example `ImagePullBackOff: Back-off pulling image ...`), otherwise the Deployment's own message. A Warning event with reason `WorkspaceComputeStalled` is recorded on the workspace when the condition first appears, so `kubectl describe workspace` shows it too.

The verdict is not final. The pod stays pending, and once it is scheduled and ready, for example after an autoscaler adds a node, the workspace returns to `Available=True` with `Degraded=False`. Stopping the workspace also clears the condition. A workspace that keeps stalling needs a change to its spec or template, applied with a stop and a start: the operator applies spec changes to the Deployment only while the workspace is `Available`.

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
