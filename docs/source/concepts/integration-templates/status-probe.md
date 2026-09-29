# Status probe

`spec.statusProbe` is an optional probe the operator runs on a fixed cadence to check that the integration is reachable from the workspace pod's real runtime context. It is report-only: the verdict is written to `workspace.status.integrationStatuses[]`. Unlike a `readinessProbe`, it never changes the pod's readiness, so it never adds or removes the pod from its `Service` endpoints (a failing probe cannot cut the workspace off from traffic); and unlike a `livenessProbe`, it never restarts the pod. It is named `statusProbe` for that reason.

The probe currently supports a single transport, `exec`. The command always runs inside the workspace container, so it sees the pod's actual network and auth context and catches data-plane failures that a control-plane status check would miss (for example a reconnect loop while the referenced object's `.status` still reads ready). `timeoutSeconds` bounds a single attempt (default 5, minimum 1).

```yaml
statusProbe:
  exec:
    command: ["ray", "status"]   # runs in the workspace container; exit 0 means the Ray session is reachable
  timeoutSeconds: 5
```

## Cadence

The re-probe period is an operator-level setting, not per-template: `--integration-probe-period`, default 5 minutes, clamped up to a 1 second floor. Only `timeoutSeconds` above is authored on the template.

The cadence is flat, with no per-verdict backoff — a persistently failing probe execs into the workspace container every period indefinitely. Status events are edge-triggered, so a stuck integration does not spam events, but the exec load is constant. Keep the probe command cheap.
