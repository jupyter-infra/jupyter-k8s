# Access Probes

An access startup probe verifies that the workspace's routing resources are serving traffic before the controller marks the workspace as `Available`. A `WorkspaceAccessStrategy` configures it to act as a one-shot gate — once the probe passes, it is never re-checked until the workspace restarts or the access strategy spec changes.

## Configuration

An access strategy configures its probe in the `spec.accessStartupProbe` attribute:

```yaml
apiVersion: workspace.jupyter.org/v1alpha1
kind: WorkspaceAccessStrategy
metadata:
  name: my-strategy
spec:
  accessStartupProbe:
    httpGet:
      urlTemplate: "https://{{ .Service.Name }}.{{ .Service.Namespace }}.svc.cluster.local:443/healthz"
    initialDelaySeconds: 5
    periodSeconds: 2
    timeoutSeconds: 5
    failureThreshold: 30
```

## Parameters

| Field | Default | Description |
|-------|---------|-------------|
| `initialDelaySeconds` | `0` | Seconds after access resources are created before probing starts |
| `periodSeconds` | `2` | How often to perform the probe |
| `timeoutSeconds` | `5` | Seconds before the probe times out |
| `failureThreshold` | `30` | Consecutive failures before marking the workspace `Degraded` |

## URL template

The `urlTemplate` is a Go `text/template` with access to the same variables available in access resource templates:

- `.Workspace` — the Workspace object
- `.AccessStrategy` — the WorkspaceAccessStrategy object
- `.Service` — the workspace's Service

## Behavior

1. After the workspace's access resources are created and `initialDelaySeconds` has elapsed, the controller begins probing.
2. An HTTP GET is sent to the resolved URL. Status codes 200–399 are considered success (additional codes can be allowed via `additionalSuccessStatusCodes`).
3. On the first success, `status.accessStartupProbeSucceeded` is set to `true` and the workspace transitions to `Available`.
4. If failures reach `failureThreshold`, the workspace is marked `Degraded`. It must be stopped and restarted to retry.

## Probe reset

The probe state resets when:
- The workspace is stopped and restarted.
- The access strategy specs change (detected via `status.observedAccessStrategyVersion`).
