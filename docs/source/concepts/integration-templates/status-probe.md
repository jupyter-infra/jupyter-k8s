# Status probe

`spec.statusProbe` is an optional probe the operator runs on its poll cadence to check that the integration is reachable from the workspace pod's real runtime context. It is report-only: the verdict is written to `workspace.status.integrationStatuses[]`. Unlike a `readinessProbe`, it never changes the pod's readiness, so it never adds or removes the pod from its `Service` endpoints (a failing probe cannot cut the workspace off from traffic); and unlike a `livenessProbe`, it never restarts the pod. It is named `statusProbe` for that reason.

The probe currently supports a single transport, `exec`. The command always runs inside the workspace container, so it sees the pod's actual network and auth context and catches data-plane failures that a control-plane status check would miss (for example a reconnect loop while the referenced object's `.status` still reads ready). `timeoutSeconds` bounds a single attempt (default 5, minimum 1).

```yaml
statusProbe:
  exec:
    command: ["ray", "status"]   # runs in the workspace container; exit 0 means the Ray session is reachable
  timeoutSeconds: 5
```
