# Workspace Lifecycle

A workspace moves through a series of states from creation to availability (and optionally to stopped). The controller drives these transitions by reconciling the `workspace.spec` against the actual resource state.

## Condition types

| Condition | Meaning |
|-----------|---------|
| `Available` | The workspace is fully functional — pod running, access probe passed, ready to accept connections |
| `Progressing` | Resources are being created, updated, or stopped; during a start the reason names the step, see [Start steps](#start-steps) |
| `Degraded` | The workspace cannot reach or keep its desired state; the reason says why, see [Failed starts](#failed-starts) and [Stalled starts](#stalled-starts) |
| `Stopped` | The workspace has been stopped; the pod is removed but storage is preserved |

Each condition's status is one of `True`, `False`, or `Unknown`.

## Typical progression

1. User creates or starts a workspace (`desiredStatus: Running`).
2. Controller sets `Progressing=True` while creating the deployment, service, and access resources; while the pod starts, the reason names the step (see below).
3. If the workspace references an access strategy with an [access startup probe](access-probes), the controller waits for it to pass.
4. On probe success: `Available=True`, `Progressing=False`.
5. On probe failure (threshold exceeded): `Degraded=True`, `Available=False`.

## Start steps

While the workspace pod is not ready, `Progressing=True` names the step the start is in and carries the message the cluster recorded for it:

| Reason | When | Message |
|--------|------|---------|
| `WaitingForNode` | The pod has no node yet | The scheduler's or the autoscaler's latest message about the pod |
| `PullingImage` | The kubelet is pulling the image, or waiting to retry a failed pull | The kubelet's pull message, or the failed attempt's reason and error |
| `StartingContainer` | The pod has a node and a container is not ready yet | The kubelet's latest message about the pod or that container, or the container's waiting reason |

The operator copies these messages unchanged, cut at 1024 bytes; `Available=False` carries the same reason.

```yaml
conditions:
  - type: Progressing
    status: "True"
    reason: WaitingForNode
    message: "Pod should schedule on: nodeclaim/workspace-gpu-x7k2m"
```

## Failed starts

A container the kubelet cannot start as specified turns the workspace `Degraded` at once instead of at the progress deadline below. The kubelet's waiting reason becomes the reason of `Degraded`, `Available` and `Progressing`, and `Progressing` is `False` while the failure lasts:

```yaml
conditions:
  - type: Degraded
    status: "True"
    reason: ImagePullBackOff
    message: 'Back-off pulling image "jupyter/notebook:typo"'
  - type: Available
    status: "False"
    reason: ImagePullBackOff
  - type: Progressing
    status: "False"
    reason: ImagePullBackOff
```

A reason counts as a failure when it starts with `Err` or ends in `Error` or `BackOff`, the rule Argo CD's health check applies, plus `InvalidImageName`. The reasons the kubelet retries after a back-off, `ImagePullBackOff`, `CreateContainerError` and `RunContainerError`, which a registry blip or a loaded node can produce once, count from the kubelet's second failed attempt, about ten seconds after the first; until then the pull or the start is a step carrying the kubelet's error. The kubelet's reasons are an open set; a reason the CRD rejects as a condition reason is written as `ContainerStartFailed`, with the kubelet's reason opening the message.

A missing Secret or ConfigMap referenced from `env` reads `CreateContainerConfigError`. Mounted as a volume it keeps the pod in `ContainerCreating`, a `StartingContainer` step with the kubelet's `FailedMount` message until the progress deadline.

A pod the cluster refuses to create, over a `ResourceQuota` or rejected by an admission webhook or Pod Security admission, never reaches a kubelet. The ReplicaSet controller reports it on the Deployment as `ReplicaFailure=True` with reason `FailedCreate`, and the workspace copies that reason with the API server's message, for example `pods "ws-abc" is forbidden: exceeded quota: gpu, ...`. The ReplicaSet controller keeps retrying, so the failure clears once the pod exists.

A crash loop is a restarted container that exits again. The kubelet restarts a first exit without back-off, so a container that exits once and comes back never reads `Degraded`. The message names the container, the exit code and reason, and the restart count:

```yaml
conditions:
  - type: Degraded
    status: "True"
    reason: CrashLoopBackOff
    message: "container workspace exited with code 1 (Error), restart count 4"
```

A container leaves the loop when its last exit is ten minutes old or ended a run of ten minutes, the point at which the kubelet drops its back-off.

The operator records one Warning event, `WorkspaceStartFailed`, per failed start. It reads the pod again every fifteen seconds while the failure lasts and clears the condition when the kubelet drops the failure, at once when the pod becomes ready or the workspace stops. A missing Secret that appears later resolves on its own; a wrong image or a failing command needs a stop and a start.

## Stalled starts

The operator does not time a start itself. The Deployment it creates carries the Kubernetes default `progressDeadlineSeconds` of 600; when no replica is ready by then, the Deployment controller marks the Deployment `Progressing=False` with reason `ProgressDeadlineExceeded`, and the workspace reports:

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

The message is the start step's message, or the Deployment's own message when the workspace has no pod. The operator records one Warning event, `WorkspaceComputeStalled`, when the condition first appears.

The Deployment controller counts only ready replicas and keeps the verdict until one is ready, so the operator applies it only while the pod has made no progress since the verdict: a pod that is scheduled, or whose container starts, after the deadline is reported by its step, and a pull in progress is always a step, since the kubelet gives up on a stuck pull itself and the back-off then counts as a failed start. A GPU node that arrives after ten minutes therefore reads `PullingImage`, not `ComputeStalled`, while the image downloads.

The verdict is not final: once the pod is scheduled and ready, for example after an autoscaler adds a node, the workspace returns to `Available=True` with `Degraded=False`, and stopping the workspace also clears it. A workspace that keeps stalling needs a spec or template change, applied with a stop and a start, since the operator applies spec changes to the Deployment only while the workspace is `Available`.

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
