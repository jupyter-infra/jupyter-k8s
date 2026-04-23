# Access Startup Probes — Implementation Plan

Issue: [#363](https://github.com/jupyter-infra/jupyter-k8s/issues/363)

## Problem

When a workspace is created, the controller marks it `Available=True` as soon as the IngressRoute
object exists in etcd. But there's a propagation delay before Traefik actually serves the route,
causing users to see a **502 Bad Gateway** during "Create and Open" workflows.

## Design Decisions

### Naming: `accessStartupProbe`
Follows the Kubernetes `startupProbe` convention — a one-shot gate that runs during startup and
stops after the first success. This distinguishes it from a continuous health check
(`readinessProbe`/`livenessProbe`) and aligns with established k8s terminology.

### `failureThreshold` (same as `corev1.Probe`)
Keeping the Kubernetes field name for consistency. The original reviewer suggestion to rename to
`maxAttempts` was made when the parent field was `accessProbe` (ambiguous). Now that we've named
it `accessStartupProbe`, the k8s `startupProbe` convention is clear and `failureThreshold` is
the natural fit.

### Success rule: `200–399` baseline + `additionalSuccessStatusCodes`
Following `corev1.HTTPGetAction` precedent, the probe passes when it receives an HTTP response
with status `>= 200 and < 400`. Connection errors are always failures.
This handles the oauth2-proxy + Dex flow (302 redirect = route is live, 502 = not propagated).

For the bearer token flow, the auth middleware returns 401 on unauthenticated requests — which
means the route IS live, but would fail the 200-399 rule. To support this,
`AccessHTTPGetProbe` has an optional `additionalSuccessStatusCodes` field that extends the
baseline. Bearer token AccessStrategies set `additionalSuccessStatusCodes: [401]`.

### Failure count tracking via `WorkspaceStatus` field
Following Kubernetes precedent (Job controller tracks failures in `status.failed`, not annotations),
the probe attempt count is stored in a new `WorkspaceStatus` field:
```go
AccessStartupProbeFailures *int32 `json:"accessStartupProbeFailures,omitempty"`
```

The field is **transient** — only present while the probe is actively running. Lifecycle:
- `nil` → not probing (initial state, or probe complete)
- `*0` → probing initiated (after initial delay, before first attempt)
- `*1`, `*2`, ... → consecutive failures
- Back to `nil` on any of: probe success, workspace stops, failureThreshold exceeded,
  or workspace/access strategy update (which restarts the probe from scratch)

Thanks to `omitempty`, the field is absent from `kubectl get workspace -o yaml` output
unless the probe is actively in progress.

### Probe URL template reuses existing template engine
The `urlTemplate` uses the same `text/template` engine and `fullAccessResourceData`
(`.Workspace`, `.AccessStrategy`, `.Service`) already used for `accessURLTemplate` and
`accessResourceTemplates`.

### HTTP client — direct from controller pod
Unlike idle detection (which runs `curl` via pod exec inside the workspace), access probes
hit the external URL from the controller pod using `net/http`. The route must be reachable
from outside the workspace pod, and no pod exec is required.

### `initialDelaySeconds` via `RequeueAfter`
The initial delay is implemented by requeuing with `RequeueAfter: initialDelaySeconds` on the
first probe cycle (when `AccessStartupProbeFailures` is nil).

---

## Changes by File

### 1. API types — `api/v1alpha1/workspaceaccessstrategy_types.go`

Add `AccessStartupProbe` and `AccessHTTPGetProbe` types, and an `AccessStartupProbe` field
to `WorkspaceAccessStrategySpec`:

```go
type AccessStartupProbe struct {
    // +optional
    HTTPGet *AccessHTTPGetProbe `json:"httpGet,omitempty"`
    // +optional
    InitialDelaySeconds int32 `json:"initialDelaySeconds,omitempty"`
    // +optional — default 2, min 1
    PeriodSeconds int32 `json:"periodSeconds,omitempty"`
    // +optional — default 5, min 1
    TimeoutSeconds int32 `json:"timeoutSeconds,omitempty"`
    // +optional — default 30, min 1
    FailureThreshold int32 `json:"failureThreshold,omitempty"`
}

type AccessHTTPGetProbe struct {
    // URLTemplate is a Go text/template resolving to the URL to probe.
    // Available variables: .Workspace, .AccessStrategy, .Service
    URLTemplate string `json:"urlTemplate"`

    // AdditionalSuccessStatusCodes extends the default success range (200–399)
    // with extra HTTP status codes that indicate the route is live.
    // Example: [401] for bearer-token auth flows where the auth middleware
    // returns 401 on unauthenticated requests.
    // +optional
    AdditionalSuccessStatusCodes []int `json:"additionalSuccessStatusCodes,omitempty"`
}
```

### 2. Workspace status — `api/v1alpha1/workspace_types.go`

Add transient probe counter to `WorkspaceStatus`:
```go
    // +optional
    AccessStartupProbeFailures *int32 `json:"accessStartupProbeFailures,omitempty"`
```

### 3. Controller constants — `internal/controller/constants.go`

Add defaults (`DefaultAccessStartupProbePeriod = 2`, `DefaultAccessStartupProbeTimeout = 5`,
`DefaultAccessStartupProbeFailureThreshold = 30`) and condition reason
`ReasonAccessStartupProbeError`.

### 4. Access startup prober — `internal/controller/access_startup_prober.go` (new)

`AccessStartupProber` struct with `net/http` client. Two methods:
- `ResolveProbeURL` — resolves `urlTemplate` via the existing template engine
- `Probe` — single HTTP GET, returns `(ready bool, err error)`. Success = status 200–399
  or in `additionalSuccessStatusCodes`.

HTTP client config: timeout from `timeoutSeconds`, redirect following disabled (so 302 is
observed directly as a 3xx success, not followed), TLS `InsecureSkipVerify: true` for
self-signed certs.

### 5. State machine — `internal/controller/state_machine.go`

Add `accessStartupProber` field to `StateMachine`. After `ReconcileAccessForDesiredRunningStatus`
at the existing TODO (line 259), call `probeAccessStartup` if `AccessStartupProbe` is configured.
If probe not yet successful, return early with requeue. If successful, fall through to
`UpdateRunningStatus`.

### 6. Probe orchestration — `internal/controller/state_machine_access.go`

New `probeAccessStartup` method:
1. Read `workspace.Status.AccessStartupProbeFailures` (`nil` = first attempt)
2. First attempt + `initialDelaySeconds > 0` → set to `*0`, requeue after delay
3. Probe → success → clear to `nil`, return (caller sets Available)
4. Probe → failure → increment; if `>= failureThreshold` → clear to `nil`,
   `UpdateErrorStatus(AccessStartupProbeError)`; else → requeue after `periodSeconds`

Also clear `AccessStartupProbeFailures` to `nil` in `ReconcileAccessForDesiredStoppedStatus`
and in CASE 2 (no AccessStrategy) of `ReconcileAccessForDesiredRunningStatus`.

### 7. Workspace controller — `internal/controller/workspace_controller.go`

Wire `AccessStartupProber` into controller setup, pass to `NewStateMachine`.

### 8. Status manager — `internal/controller/status_manager.go`

No changes needed. Existing `UpdateStartingStatus` with `accessResourcesReady = false` produces
`Available=False, Progressing=True, reason=AccessNotReady`. Degraded case uses existing
`UpdateErrorStatus`.

### 9. Access resources builder — `internal/controller/access_resources_builder.go`

Extract shared `ResolveTemplateURL(urlTemplate, workspace, accessStrategy, service)` method.
Refactor `ResolveAccessURL` to call it internally.

### 10. CRD generation

`make helm-generate` — updates CRDs for both WorkspaceAccessStrategy and Workspace, plus
`dist/chart/`.

---

## Unit Tests

### `internal/controller/access_startup_prober_test.go` (new)
- URL template resolution, probe with `httptest.Server` for various status codes (200, 302,
  502, 503, 404, 401), timeout handling, redirect following disabled, 200-399 baseline,
  additionalSuccessStatusCodes extending the baseline

### `internal/controller/state_machine_test.go` (extend)
- No accessStartupProbe → straight to Available (regression check)
- First attempt requeues with initialDelaySeconds
- Successful probe clears status field, transitions to Running
- Failed probe increments status field, requeues with periodSeconds
- failureThreshold exceeded → Degraded
- Status field cleared on stop and on access strategy change

---

## E2E Tests

**Option A** (this repo): AccessStrategy with `accessStartupProbe` pointing at a URL that
returns 502. Verify `Progressing=True, reason=AccessNotReady` → `Degraded=True` after
failureThreshold. Tests controller behavior without a real ingress.

**Option B** (`jupyter-k8s-aws`): Full flow with Traefik. Verify AccessNotReady → Available.

Do both.

---

## Guided Charts (`jupyter-k8s-aws`)

### Traefik + Dex chart
Add `accessStartupProbe` to the AccessStrategy:
```yaml
accessStartupProbe:
  httpGet:
    urlTemplate: "https://<DOMAIN>/workspaces/{{ .Workspace.Namespace }}/{{ .Workspace.Name }}/"
  periodSeconds: 2
  failureThreshold: 30
```
oauth2-proxy flow: unauthenticated probe gets 302 (→ Dex), which is 3xx = success by default.
Bearer token flow: auth middleware returns 401, needs `additionalSuccessStatusCodes: [401]`.

### HyperPod chart
SSM tunneling, no IngressRoute — likely no probe needed. Confirm with testing.

---

## Work Split

### `jupyter-k8s`
1. API types (AccessStrategy + Workspace status)
2. CRD generation
3. Access startup prober implementation
4. State machine integration
5. Unit tests
6. E2e tests (option A)

### `jupyter-k8s-aws`
1. Add `accessStartupProbe` to Traefik+Dex AccessStrategy values/templates
2. Verify oauth probe behavior (expect 302 = pass)
3. Confirm HyperPod doesn't need a probe
4. E2e test with real Traefik

---

## Sequence Diagram

```
Workspace Created (desiredState=Running)
  │
  ├─ Deployment ready? Service ready?
  │   └─ No → Progressing=True, reason=ComputeNotReady/ServiceNotReady, requeue
  │
  ├─ Both ready → EnsureAccessResourcesExist (create IngressRoute in etcd)
  │
  ├─ AccessStartupProbe defined?
  │   └─ No → Available=True (current behavior, unchanged)
  │
  ├─ Yes → First attempt? (status.accessStartupProbeFailures == nil)
  │   ├─ initialDelaySeconds > 0 → set failures=0, requeue after delay
  │   └─ initialDelaySeconds = 0 → proceed to probe
  │
  ├─ HTTP GET urlTemplate
  │   ├─ Status 200–399 or ∈ additionalSuccessStatusCodes → clear failures, Available=True
  │   │
  │   └─ Status ≥ 400 (not in additional) or connection error → increment failures
  │       ├─ < failureThreshold → Progressing=True, reason=AccessNotReady, requeue
  │       └─ ≥ failureThreshold → Degraded=True, reason=AccessStartupProbeError, stop
  │
  └─ Handle idle shutdown (existing)
```
