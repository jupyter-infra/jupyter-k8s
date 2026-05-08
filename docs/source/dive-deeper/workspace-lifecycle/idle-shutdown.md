# Idle Shutdown

Idle shutdown automatically stops workspaces that have been inactive for a configurable duration, freeing cluster resources while preserving storage.

## Configuration

Configure idle shutdown directly in the `workspace.spec`:

```yaml
apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: my-notebook
spec:
  idleShutdown:
    enabled: true
    idleTimeoutInMinutes: 60
    detection:
      httpGet:
        path: /api/status
        port: 8888
```

## Detection methods

The controller determines idle state by polling an HTTP endpoint inside the workspace pod.

### HTTP GET

```yaml
detection:
  httpGet:
    path: /api/status
    port: 8888
```

The application signals whether it is idle through its response. [JupyterLab](../../applications/jupyterlab), for example, reports active kernels through its `/api/status` endpoint.

## Template defaults and bounds

Templates can provide a default idle shutdown configuration and enforce bounds:

```yaml
apiVersion: workspace.jupyter.org/v1alpha1
kind: WorkspaceTemplate
spec:
  defaultIdleShutdown:
    enabled: true
    idleTimeoutInMinutes: 60
    detection:
      httpGet:
        path: /api/status
        port: 8888
  idleShutdownOverrides:
    allow: true
    minIdleTimeoutInMinutes: 15
    maxIdleTimeoutInMinutes: 480
```

| Field | Description |
|-------|-------------|
| `defaultIdleShutdown` | Applied when the workspace does not set its own `idleShutdown` |
| `idleShutdownOverrides.allow` | Whether workspaces may override the template's idle shutdown config |
| `idleShutdownOverrides.minIdleTimeoutInMinutes` | Minimum allowed timeout (validated by webhook) |
| `idleShutdownOverrides.maxIdleTimeoutInMinutes` | Maximum allowed timeout (validated by webhook) |

## Behavior

1. When the workspace reaches `Available` status, the controller begins polling the detection endpoint at regular intervals.
2. If the endpoint indicates idle state and `idleTimeoutInMinutes` has elapsed since the last active signal, the controller sets `spec.desiredStatus` to `Stopped`.
3. The workspace shuts down gracefully — the pod is removed and storage is preserved.
4. The user can restart the workspace at any time by setting `desiredStatus: Running`.
