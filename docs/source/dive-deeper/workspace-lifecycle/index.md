# Workspace Lifecycle

A workspace moves through a series of states from creation to availability (and optionally to stopped). The controller drives these transitions by reconciling the `workspace.spec` against the actual resource state.

## Condition types

| Condition | Meaning |
|-----------|---------|
| `Available` | The workspace is fully functional — pod running, access probe passed, ready to accept connections |
| `Progressing` | Resources are being created, updated, or stopped; while the pod starts, the reason names the step (`WaitingForNode`, `PullingImage`, `StartingContainer`, see [Start steps](#start-steps)) |
| `Degraded` | The workspace failed to reach or maintain its desired state: a container the kubelet cannot start, a start past the progress deadline (`ComputeStalled`), an access probe past its failure threshold, or a resource that could not be created |
| `Stopped` | The workspace has been stopped; the pod is removed but storage is preserved |

Each condition's status is one of `True`, `False`, or `Unknown`.

## Typical progression

1. User creates or starts a workspace (`desiredStatus: Running`).
2. Controller sets `Progressing=True` while creating the deployment, service, and access resources; while the pod starts, the condition carries the step and the recorded message (see below).
3. If the workspace references an access strategy with an [access startup probe](access-probes), the controller waits for it to pass.
4. On probe success: `Available=True`, `Progressing=False`.
5. On probe failure (threshold exceeded): `Degraded=True`, `Available=False`.

## Start steps

While the workspace pod is not ready, `Progressing=True` names the step the start is in and copies the message the cluster recorded for it, so a user can tell a start that is making progress from one that is waiting:

| Reason | When | Message |
|--------|------|---------|
| `WaitingForNode` | The pod has no node yet | The newest event a component other than the scheduler recorded on the pod, for example Karpenter's `Pod should schedule on: nodeclaim/...` or `Failed to schedule pod, all available instance types exceed limits for nodepool "gpu"`; the scheduler's `PodScheduled` message (`0/3 nodes are available: ...`) when there is none |
| `PullingImage` | The kubelet is pulling the image | The kubelet's `Pulling image "..."` event |
| `StartingContainer` | The pod has a node and its container is not ready | The kubelet's newest event for the pod (`Successfully pulled image ...`, `Started container ...`, `Readiness probe failed: ...`), or the waiting container's reason and message (`ContainerCreating`, `PodInitializing`) while the kubelet has recorded no event |

The operator does not interpret these messages; it copies them, cut at 1024 bytes. The step appears on `Available=False` with the same reason. Events are read for a starting pod at most every 5 seconds, and a few times per second across all workspaces, so many workspaces starting at once do not crowd out the operator's other API calls; the status is written only when the message changes.

```yaml
conditions:
  - type: Progressing
    status: "True"
    reason: WaitingForNode
    message: "Pod should schedule on: nodeclaim/workspace-gpu-x7k2m"
```

## Failed starts

A container the kubelet cannot start as specified turns the workspace `Degraded` at once, instead of after the progress deadline below. The kubelet's waiting reason and message are copied onto `Degraded`, `Available` and `Progressing`:

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

The reasons that count are those of an image that cannot be pulled, an invalid image name, a missing Secret or ConfigMap and a command that keeps failing: `ErrImagePull`, `ImagePullBackOff`, `ErrImageNeverPull`, `InvalidImageName`, `CreateContainerConfigError`, `CrashLoopBackOff`, and any other reason starting with `Err` or ending in `Error` or `BackOff`, the rule Argo CD's health check applies. The operator's own reasons `ComputeError` and `ServiceError`, set when a Deployment or Service write fails, are not kubelet reasons and are not part of it.

A container that keeps exiting is a crash loop even while the kubelet shows it as running or terminated between restarts, which on Kubernetes 1.37 is most of the time. Such a container is reported as `CrashLoopBackOff` with the exit code and the restart count as the message, until the kubelet's own back-off message is seen.

A Warning event with reason `WorkspaceStartFailed` is recorded on the workspace when the condition first appears; the kubelet alternating between related reasons (`ErrImagePull`, `ImagePullBackOff`) or a container restarting does not record another. The condition clears once the workspace is `Available`. A missing Secret that appears later resolves on its own, since the kubelet retries; a wrong image name or a failing command is fixed with a stop and a start, as for stalled starts.

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

The message is the start step's message above (the scheduler's or the autoscaler's verdict while the pod has no node, otherwise the kubelet's newest event, for example `Failed to create pod sandbox ...` or `Readiness probe failed: ...`), or the Deployment's own message when the workspace has no pod. A Warning event with reason `WorkspaceComputeStalled` is recorded on the workspace when the condition first appears, so `kubectl describe workspace` shows it too.

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
