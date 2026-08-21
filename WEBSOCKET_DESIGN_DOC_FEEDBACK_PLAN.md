# WebSocket Remote Connection — Implementation Plan & Session Handoff

**Last updated:** 2026-07-28
**Design doc (Quip):** https://quip-amazon.com/72OzARFCXng2/Parker-WebSocket
**Status:** Tasks 1–3 merged. **Task 4a + 4b are implemented and unmerged** — a code change (Option B — see the decision log), split out of Task 4 after a code review found a pre-existing bug in the Service update path. Task 4c was considered and **dropped** (rationale kept under Task 4c). Tasks 5–9 remain; **Task 5's NetworkPolicy half is now the blocker** — until it lands, 4b is a silent no-op on any cluster with network policies enabled.

> **Read this first — before touching any code.**
> This is an open-source, cloud-agnostic alternative to AWS SSM for remote IDE connections (VS Code, Kiro, Cursor) into workspace pods in the `jupyter-k8s` operator. Before starting any task, review the three repos and the reference files listed under **"Repositories — review before starting"** and **"Key reference files"** so you understand the existing patterns. Do not invent new mechanisms; the operator already supports almost everything through existing CRD fields and the access-strategy template system. Follow existing patterns in the package, or conventional Go/Kubernetes practice where no local pattern exists. When adding code, add unit tests; note where e2e tests are needed.

---

## 1. What we are building (the big picture)

Remote connections let a user attach a desktop IDE to a workspace pod's remote access server (SSH-like, listening on `localhost:2222` inside the pod). Today this works **only** via AWS SSM: it requires IAM, an SSM agent sidecar, instance registration on pod start, and cleanup on pod delete — and it only works on AWS.

We are adding **WebSocket** as an alternative that:

1. Routes through the cluster's existing ingress: **Load Balancer → Traefik → pod** (no AWS services).
2. Uses a **lightweight custom Go proxy sidecar** (our `workspace-websocket-proxy`) instead of the SSM agent.
3. Authenticates via **JWT** (the same system the web UI already uses) instead of AWS IAM.
4. Is **stateless** — no registration, no cleanup, no state files, no `podEventsHandler`.
5. Works on **any** Kubernetes cluster (EKS, GKE, AKS, on-prem).

### Background: the two deployment flavors, and why this work matters

There are two Helm-chart flavors of the product, and they are **deployment modes**, not features:

| | `charts/aws-hyperpod` | `charts/aws-oidc` |
|---|---|---|
| User login | AWS-managed | **Dex** (OIDC provider) federating to **GitHub OAuth**, self-hosted in-cluster |
| Remote IDE access | AWS SSM, via the `aws:createSession` plugin | **nothing today — this is what we are building** |
| Plugins | uses `jupyter-k8s-plugin` | none (`charts/aws-oidc/AGENT.md`: "It does NOT use `--plugin-endpoints` — there are no plugins") |

**OIDC** (OpenID Connect) is the standard "log in with an existing identity provider" protocol — the thing behind every "Sign in with Google" button. Three parties: the user in a browser, the **identity provider** that actually knows who they are and can prove it, and the **application** that trusts the IdP's signed answer (a JWT ID token). The app never sees a password. So `aws-oidc` is not "the OIDC feature" — it is the **fully open-source deployment flavor**, named for how it authenticates.

Per the `jupyter-deploy` docs (`docs/source/templates/aws-eks-oidc-template/architecture.md`), that stack is: NLB → **Traefik** (ingress/reverse proxy) → **Dex** (federates to GitHub) → **oauth2-proxy** (drives browser sign-in) → **authmiddleware** (validates the JWT session cookie on every request via Traefik ForwardAuth) → the operator → the workspace pod.

**This is the reason the WebSocket work exists:** `aws-oidc` is the cloud-agnostic path and it has **no remote-IDE story at all**, because SSM is AWS-proprietary and lives behind a plugin. Our ws-proxy is the OSS replacement. It is also why Jonathan's Slack guidance is "stick to pre-release till we integrate it in at least one deployment (e.g. the **eks-oidc** template)" — `eks-oidc` is the intended first customer. (`jupyter-k8s-ui` is the in-cluster browser console that lists workspaces and asks the Extension API for connection URLs; it is what would eventually surface a "connect with VS Code" button.)

### End-to-end request flow

```
User runs a connect script / Toolkit
  1. POST {"workspaceConnectionType":"ssh-over-websocket","workspaceName":"my-ws"} → Extension API
  2. Extension API: authz check → workspace Available? → look up access strategy
  3. connectionType == ssh-over-websocket → generateWebSocketConnectionURL
  4. Generate bootstrap JWT (signerFactory.CreateSigner), render BearerAuthURLTemplate,
     swap scheme to wss://, strip /bearer-auth
  5. Returns: wss://my-ws-<b32ns>.example.com/ssh-ws?token=<jwt>
  6. Client extracts token, runs:
       websocat --binary -H="Authorization: Bearer <jwt>" asyncstdio: wss://my-ws-<b32ns>.example.com/ssh-ws
  7. Load Balancer → Traefik matches the WebSocket IngressRoute → ForwardAuth validates the JWT
     ONCE at the HTTP upgrade
  8. Traefik proxies to the ws-proxy Service on port 8080
  9. ws-proxy sidecar bridges WebSocket <-> TCP localhost:2222 (remote access server)
```

Key property: JWT is validated **only** at the WebSocket upgrade. Once the pipe is established, the token is never re-checked; the session persists until max duration, dead-connection detection, or disconnect. Proactive mid-session revocation is future work (scaffolded, not implemented).

---

## 2. Repositories — review before starting

| Repo | Local path | Remote | Role |
|---|---|---|---|
| **Operator (main)** | `/Users/earaghbi/workplace/jkInfra/jupyter-k8s-infra` | `github.com/jupyter-infra/jupyter-k8s` | CRDs, controllers, Extension API, auth middleware, webhooks. **All remaining code changes land here** (Task 4) plus the sample YAML (Tasks 5–6). Review `internal/extensionapi/`, `internal/controller/`, `internal/authmiddleware/`, `api/v1alpha1/`, `api/connection/v1alpha1/`. |
| **WebSocket proxy sidecar** | `/Users/earaghbi/workplace/workspace-websocket-proxy` | `github.com/jupyter-infra/workspace-websocket-proxy` | Custom Go binary bridging WebSocket → TCP, runs as a pod sidecar. Review `cmd/ws-proxy/main.go`, `internal/proxy/`, `Dockerfile`, `test/e2e/`, `.github/workflows/`. **Not yet released.** |
| **AWS plugin** | `/Users/earaghbi/workplace/jupyter-k8s-aws` | `github.com/jupyter-infra/jupyter-k8s-aws` | AWS SSM remote access. **Not being modified** — reference only. Review `charts/aws-hyperpod/templates/hyperpod-access-strategy.yaml` (the single most important reference for how access strategies are wired in YAML). |
| **SSM sidecar image** (ref only) | `/Users/earaghbi/workplace/sidecar/src/SageMakerParkerSideCarImage` | — | The existing SSM sidecar source. Reference for understanding the SSM pattern; not modified. |

### Other docs in this repo
- `WEBSOCKET_DESIGN_DOC_PLAN.md` — section-by-section plan for the Quip design doc.
- `WEBSOCKET_IMPLEMENTATION_PLAN.md` — an earlier, aspirational v1.0 technical plan. **Partly superseded** — e.g. it names the handler `"websocket"` and paths under `images/websocket-proxy/`, but the shipped code uses handler `k8s-native`, connection type `ssh-over-websocket`, and a separate repo. Trust *this* file and the code over it.
- `SSM_REMOTE_CONNECTION_IMPLEMENTATION.md` — how SSM works today (reference).
- `websocket-poc/WEBSOCKET_POC.md` — POC runbook.

---

## 3. Task status

| # | Task | Status | Where |
|---|---|---|---|
| 1 | WebSocket proxy sidecar | ✅ DONE | `workspace-websocket-proxy`, PR #1 + #2 merged |
| 2 | Auth middleware — `Authorization` header | ✅ DONE (merged, #442) | `internal/authmiddleware/serverroute_bearer_auth.go` |
| 3 | Extension API — WebSocket handler | ✅ DONE (merged, #449) | `internal/extensionapi/serverroute_connection.go` |
| 4a | **Fix Service `NeedsUpdate`/`UpdateServiceSpec` field scoping** (pre-existing bug) | ✅ DONE (unmerged) | `internal/controller/service_builder.go` |
| 4b | **Service port for proxy** | ✅ DONE (unmerged) | `internal/controller/service_builder.go` + reconcile wiring + tests |
| 4c | Admission validation for sidecar ports | ❌ DROPPED — see task | rationale kept in Task 4c |
| 5 | WebSocket IngressRoute template **+ NetworkPolicy ports** | TODO | Access-strategy YAML (both charts) |
| 6 | Sample access strategy | TODO | New YAML (location TBD, see task) |
| 7 | Client-side connect script | TODO | Shell script + docs |
| 8 | E2E tests (full path) | TODO | Depends on 4–6 deployed on a cluster |
| 9 | AWS plugin coexistence | TODO | Verify only, no changes |
| R | Proxy repo release hardening | TODO (from Slack w/ Jonathan) | `workspace-websocket-proxy` |

---

## 4. What is already done — verified against code

### Task 1 — proxy sidecar (`workspace-websocket-proxy`)
Custom Go binary. Confirmed current state:
- **Listen:** `:8080` (`internal/proxy/server.go:45`). **Target:** dials `127.0.0.1:2222` with a 5s dial timeout (`server.go:130`, `session.go:113`).
- **Bridge:** `gorilla/websocket`, two goroutines, 32KB buffer, **binary frames only** (other frame types dropped), writes serialized by a write mutex with a 10s deadline (`internal/proxy/bridge.go`).
- **Implemented lifecycle:** max concurrent connections (default 10, rejects with HTTP 503 `Service at capacity`), max session duration (default 12h, sends close frame + 5s grace), WebSocket ping/pong dead-connection detection (`PING_INTERVAL` 30s / `PING_TIMEOUT` 60s), read limit (64KB). (`internal/proxy/session.go`)
- **Scaffolded only:** `Revalidator` interface + `NoOpRevalidator` hardcoded (`internal/proxy/revalidator.go`, `server.go:36`). `REVALIDATION_INTERVAL` / `REVALIDATION_ENDPOINT` env vars are read but unused. Mid-session revalidation is deliberately deferred.
- **Endpoints:** `GET /health` (always 200, JSON `{status, activeConnections}`), `GET /metrics` (dedicated Prometheus registry), and `/` catch-all = WebSocket upgrade. **There is no `/ssh-ws` path inside the proxy** — any path other than `/health` and `/metrics` upgrades. So the IngressRoute `PathPrefix(/ssh-ws)` is purely a Traefik-side routing convention; the proxy accepts it as the catch-all.
- **Auth:** none. The proxy trusts Traefik ForwardAuth (`server.go:82,87`). Anything that can reach `:8080` in the pod network gets a raw pipe to `:2222`. `CheckOrigin` returns true unconditionally.
- **Config:** env-vars only (`internal/proxy/config.go`): `LISTEN_ADDR=:8080`, `TARGET_HOST=127.0.0.1`, `TARGET_PORT=2222`, `MAX_SESSION_DURATION=12h`, `PING_INTERVAL=30s`, `PING_TIMEOUT=60s`, `MAX_CONNECTIONS=10`, `READ_LIMIT=65536`, plus the unused revalidation vars. Malformed values silently fall back to defaults.
- **Image:** multi-stage, runtime `gcr.io/distroless/static:nonroot`, `USER 65532`, `EXPOSE 8080`, healthcheck via `ws-proxy --healthcheck` (HTTP GET `/health`, for exec probes). Built `linux/amd64,linux/arm64`.
- **Publishing:** staging→prod promotion. Staging `ghcr.io/jupyter-infra/staging/workspace-websocket-proxy`, prod `ghcr.io/jupyter-infra/workspace-websocket-proxy`, tags `:<version>`, `:sha-<short>`, `:latest`. Release is manual `workflow_dispatch` with a semver gate, main-branch gate, tag-must-not-exist gate.

> **Known latent bug to fix before/with the release** (found during review, `session.go:130`): defers run LIFO, so `defer <-pingDone` blocks before `defer s.cancel()`. On a normal client disconnect while the parent context is still live, `pingLoop` only exits on its next tick — so teardown can stall up to `PING_INTERVAL` (30s) while still holding a `SessionManager` slot. Under `MAX_CONNECTIONS=10` this can wedge capacity. Worth fixing before v0.1.0.

### Task 2 — auth middleware
`handleBearerAuth` extracts the token from `Authorization: Bearer <token>` first, falling back to the `?token=` query param (backward-compatible with the browser web-UI path). File: `internal/authmiddleware/serverroute_bearer_auth.go`.

### Task 3 — Extension API WebSocket handler
`internal/extensionapi/serverroute_connection.go`:
- Connection type constant `ConnectionTypeWebSocket = "ssh-over-websocket"` (`api/connection/v1alpha1/workspace_connection_types.go:22`).
- `HandleConnectionCreate` (`:329`) switches on connection type: `web-ui` → `generateBearerTokenURL`; `ssh-over-websocket` → `generateWebSocketConnectionURL` (`:339`); everything else matches the `{ide}-remote` regex and resolves via `CreateConnectionHandlerMap` → falls back to `CreateConnectionHandler`. **If a `*-remote` type resolves to the `k8s-native` handler, it also routes to `generateWebSocketConnectionURL`** (`:349-351`) — so both a dedicated `ssh-over-websocket` type and a `vscode-remote: k8s-native` handler-map entry work.
- `generateWebSocketConnectionURL` (`:163`) reuses `BearerAuthURLTemplate`, generates a bootstrap JWT (`TokenTypeBootstrap`, `skipRefresh=true`), parses domain/path for claims, swaps scheme to `wss://`, strips a trailing `/bearer-auth`, returns `<wss url>?token=<jwt>`.
- Request validation (`:398`) accepts `web-ui`, `ssh-over-websocket`, and any `{ide}-remote` pattern.

---

## 5. Decision log (why the summary you may have been handed is stale)

Earlier session notes described **Task 4 as "no code change — create a second Service via `accessResourceTemplates`."** That was reversed at the end of that session. Current decisions:

1. **Task 4 = Option B (code change), not the template approach.** The Service must reflect what the pod actually exposes; that is proper system design, not an "avoid-a-code-change" hack. **Verified feasible:** the reconcile call `EnsureServiceExists` (`internal/controller/state_machine.go:240`) already has `accessStrategy` in scope — it simply isn't threaded into `ServiceBuilder` yet. The sidecar's declared ports live at `accessStrategy.Spec.DeploymentModifications.PodModifications.AdditionalContainers[].Ports` (`api/v1alpha1/workspaceaccessstrategy_types.go:52`). Do **not** hardcode 8080 — expose the container ports the access strategy actually declares. See Task 4 below.
2. **Do not use "direct SSH" as the design reference for Task 4.** The hyperpod chart's direct-SSH Service (`hyperpod-access-strategy.yaml:61-79`) is `clusterIP: None` — **headless**. Its entire job is to give ExternalDNS a hostname that resolves to the pod's VPC IP so an SSH client connects to the pod *directly, bypassing Traefik*. It is a DNS-publishing device, not a routing target. It is not an example of "expose a sidecar port to Traefik"; it is an example of "skip Traefik." Different problem.
3. **SSM is not a precedent in either direction — verified.** The SSM sidecar (`hyperpod-access-strategy.yaml:126-157`) declares **no `ports:` block at all**, and there is no SSM Service anywhere in the chart. SSM inverts the connection: the agent **dials outbound** to AWS Systems Manager and registers as a managed node (hence `registrationScript`, `registrationMarkerFile`, `ssmManagedNodeRole`, and the `podEventsHandler` needed to re-register on pod events). The client then talks to the *AWS SSM API*, which relays down the tunnel the agent already opened. Nothing ever connects *toward* the pod → no listener → no Service port → no IngressRoute. The port in `remoteAccessServerPort` is passed as **context to code** (`createConnectionContext.port`, `podEventsContext.remoteAccessServerPort`), never as network plumbing; it is localhost-only inside the pod. So the earlier claim "the SSM service is always present, so create the WebSocket service the same way" is **false** — there is no SSM service. WebSocket is inbound on every axis and genuinely needs a Service port. (Useful corollary: under the Task 4b rule, SSM contributes zero extra ports automatically — not as a special case, but because it truly has nothing to expose.)
4. **Exposing the Service port is necessary but NOT sufficient — NetworkPolicy is a second, independent gate.** See Task 5. Both charts pin workspace ingress to port 8888 explicitly, so a Service declaring 8080 routes to a port the CNI drops. Task 4b alone is a silent no-op on any cluster with NetworkPolicies enabled.
5. **Sidecar ports are inferred from `containerPorts`, not from a new explicit CRD field.** This was a deliberate judgment call, recorded because it is reversible only cheaply *before* anyone depends on it. In Kubernetes `containerPort` is documented as **purely informational** — declaring it opens nothing, omitting it closes nothing — so using it as the trigger for Service exposure means reading intent from a field that carries none by contract. The rejected alternative was a `serviceModifications.additionalPorts` field sibling to `deploymentModifications` (explicit, typed, opt-in per port, no inference). We chose inference because it keeps a **single source of truth** — the port is written once, next to the container that listens on it, so drift is impossible — and because `kubectl expose` sets the same precedent. Revisit only before the behavior ships to users.
6. **~~Validation belongs in the admission webhook~~ — reversed; Task 4c dropped.** The principle (validate at the boundary once, not N times in a background loop) is sound but did not apply: 4b's skip-and-log already guarantees the operator never emits an invalid Service, so there is no retry loop to prevent and the webhook would only improve the error message. The length/charset half of the check was also based on a wrong constraint. Full reasoning kept under Task 4c.
7. **Connection type is a dedicated `ssh-over-websocket`**, not reused `vscode-remote`. WebSocket is IDE-agnostic.
8. **Token via `Authorization` header** on the client (`websocat -H`), query-param fallback retained for web UI.
9. **JWT validated only at upgrade** (Traefik ForwardAuth), confirmed behavior — token expiry does not interrupt an active session. See https://community.traefik.io/t/websocket-messages-are-not-routed-through-forward-auth/20660.
10. **ALB 60s idle timeout** is the real long-connection risk (Traefik `readTimeout` only applies to the HTTP handshake, not the live WebSocket). Mitigate with SSH keepalives (`ServerAliveInterval`) on the client and/or the proxy's ping loop.
11. **No CRD changes.** All existing `WorkspaceAccessStrategy` fields are reused: `bearerAuthURLTemplate`, `createConnectionHandler`, `createConnectionHandlerMap`, `accessResourceTemplates`, `deploymentModifications`. (Decision 5 above is what keeps this true — an explicit `serviceModifications` field would have been a CRD change.)
12. **Separate IngressRoute from web UI.** WebSocket and web UI are independent features that can be enabled/omitted independently.
13. **Custom Go proxy, not raw websocat, on the server side** — needed for max duration, ping/pong, concurrent limits, and the future revalidation hook, which websocat cannot do. websocat remains the *client* tool.

---

## 6. Remaining tasks — detailed

### Task 4a — Fix the Service update path (pre-existing bug, do this FIRST)

**This is not WebSocket work, but Task 4b depends on this code path being correct.** Found during review of Task 4.

**The bug.** `service_builder.go:83`:

```go
return !equality.Semantic.DeepEqual(existingService.Spec, desiredService.Spec), nil
```

`existingService` comes from the API server, so its `Spec` has been **defaulted**: `ClusterIP: 10.x.x.x`, `ClusterIPs`, `IPFamilies`, `IPFamilyPolicy`, `SessionAffinity: None`, `InternalTrafficPolicy: Cluster`. `desiredService` is freshly built in memory with all of those empty. `equality.Semantic` only registers custom equality for `resource.Quantity`, `metav1.Time`, and label/field selectors — it does **not** treat `""` as equal to a defaulted value. So `NeedsUpdate` returns `true` on every reconcile of every available workspace, and `UpdateServiceSpec` then does:

```go
existingService.Spec = desiredService.Spec   // ← throws away ClusterIP and every other defaulted field
```

and issues an `Update` with an empty `clusterIP` on an existing Service — an immutable field. So today this is either a hot reconcile loop or a repeating update error on **every** workspace.

**Why the existing test can't see it:** `service_builder_test.go:54` builds the "existing" Service with `BuildService` itself, so both sides have empty `ClusterIP` and compare equal. The test only ever exercises the in-memory-vs-in-memory case.

**Why this blocks 4b:** right now the bug is masked because the desired spec never changes, so a wrong-but-idempotent update is invisible. The moment the Service is *supposed* to change when a sidecar appears, this is the mechanism we depend on to deliver that change.

**Fix.** Compare and mutate only the fields the operator owns — `Type`, `Selector`, `Ports` — instead of the whole `Spec`. Leave every server-populated field untouched.

**Tests.** Add a case that builds the "existing" Service and then sets `ClusterIP` (plus `SessionAffinity`, `InternalTrafficPolicy`) the way the API server would, and assert `NeedsUpdate` is `false` and that `UpdateServiceSpec` **preserves** `ClusterIP`. Written against the current code this test must fail; that is the point.

**Note:** `pvc_builder.go` very likely has the same whole-`Spec` comparison problem. Out of scope here — flag it as a follow-up, do not fix it in this PR.

**Verify on a cluster** which of the two symptoms this actually produces today (hot loop vs. repeating `Update` error) — check operator logs for repeated `Updating Service` lines or `spec.clusterIP: Invalid value: ""` errors.

---

### Task 4b — Expose sidecar-declared ports on the workspace Service (CODE CHANGE, Option B)

**Problem.** The ws-proxy sidecar listens on port 8080 inside the pod. Traefik routes to Services, and **a Service only forwards ports it explicitly declares** — it is not a transparent pass-through to the pod. The workspace Service today declares **only** port 8888 (`service_builder.go:63-69`, `JupyterPort` in `constants.go:22`). With no Service port for 8080, the WebSocket IngressRoute has nothing to route to.

**The framing that makes the design obvious:** the workspace Service should describe the ports the workspace pod actually serves. Today it describes only the *primary* container's ports. The moment an access strategy is allowed to add a container that listens on a port, the Service is simply **wrong** — an incomplete description of the pod. That is the actual defect; the missing WebSocket route is a symptom. Hence the rule is generic and contains **no** knowledge of ws-proxy: no `if websocketEnabled`, no hardcoded 8080.

**Approach.**
1. Thread `accessStrategy` into the service builder. Change signatures of `BuildService`, `NeedsUpdate`, `UpdateServiceSpec`, and `buildServiceSpec` in `internal/controller/service_builder.go` to also accept `*workspacev1alpha1.WorkspaceAccessStrategy` (may be nil). This mirrors the deployment path exactly — `DeploymentBuilder.BuildDeploymentWithAccessStrategy` / `NeedsUpdate` already take it, for the same reason: it is building a pod whose shape the access strategy determines. `ServiceBuilder` builds the front door to that same pod.
2. In `buildServiceSpec`, after the base `http`/8888 port, iterate `accessStrategy.Spec.DeploymentModifications.PodModifications.AdditionalContainers` (`api/v1alpha1/workspaceaccessstrategy_types.go:52`); for each declared `ContainerPort`, append a matching `corev1.ServicePort` (`Port`/`TargetPort` = the container port, `Protocol` from the container port defaulting to TCP).
3. **Port naming.** A name is **required, not cosmetic**: `ValidateService` calls `validateServicePort` with `requireName = len(service.Spec.Ports) > 1` (upstream `pkg/apis/core/validation/validation.go:6359`), so the moment a second port appears *every* port must be named or the Service is rejected. `ContainerPort.Name` is optional on containers, so a fallback to the container name is required.

   **Length and charset need no handling** (an earlier draft of this plan claimed a 15-char DNS-1035 limit — that was wrong). `ServicePort.Name` is validated by `ValidateDNS1123Label` (:6519) → **up to 63 chars**. Both possible sources are already narrower: `ContainerPort.Name` is validated by `IsValidPortName` (≤15, `[-a-z0-9]`, ≥1 letter) and a container name by `ValidateDNS1123Label` (≤63). So a derived name is always legal and is used **verbatim** — truncating it would corrupt a valid name into a different string.

   **Uniqueness does need handling.** `validateContainerPorts` builds a fresh `allNames` set per container (:2650, called per-container at :3818), so two sidecars may each legally declare the same port name; copied into one Service that is a `field.Duplicate`. Apply a deterministic `-2`/`-3` suffix, and ensure the result never collides with `http`.
4. **Dedup.** Skip a container port whose number equals `JupyterPort`, and skip a port number a previous sidecar already claimed — logging both. Nothing upstream rejects a duplicate `containerPort` number: core Pod validation checks only `hostPort` conflicts (`checkHostPortConflicts`), and a CRD structural schema does not validate embedded `[]corev1.Container` at all. So such a strategy can legitimately exist, and skipping is what keeps the emitted Service valid — the API server *does* reject a Service with a duplicate `(protocol, port)` pair (:6437).
5. Thread it through the reconcile call sites: `createService` (`resource_manager.go:114`), `EnsureServiceExists` (`:267`), `ensureServiceUpToDate` (`:280`), `updateService` (`:299`). `accessStrategy` is already in scope at the caller (`state_machine.go:240`) — the deployment call one line up at `:227` already receives it. It must reach `NeedsUpdate`/`UpdateServiceSpec` too, not just `BuildService`, or a workspace already running when the strategy gains a sidecar would compare against a stale single-port desired spec and never acquire the port.
6. **Backward compatibility — the blast radius is every workspace, not just WebSocket ones.** When `accessStrategy` is nil or declares no additional container ports, the Service must be **byte-identical** to today (single 8888 port). This needs an explicit test, not an assumption. SSM workspaces are safe: verified that the SSM sidecar declares no `ports:` at all.
7. **Security posture (why the generic rule is safe).** Task 4b grants exactly one thing, at the least-privileged layer: in-cluster reachability on a `ClusterIP` Service. Reaching a sidecar from outside requires **three independent, admin-authored, default-closed gates** — (i) declare a `containerPort` on the sidecar, (ii) write an IngressRoute (behind ForwardAuth → authmiddleware → valid JWT), (iii) open the port in NetworkPolicy (Task 5). Nothing is exposed by inference or by default. *ARCC was not queried (MCP server unavailable); this is standard defense-in-depth practice.*
8. **Reconcile churn at scale.** `workspace_controller.go:256` watches `WorkspaceAccessStrategy` and `accessStrategyEventHandler` fans out to **every** referencing workspace. Editing one shared strategy enqueues all N workspaces, each of which now recomputes a Service diff. This is pre-existing behavior for Deployments and is fine *provided 4a is done* — each reconcile is then a no-op read rather than an `Update`. Without 4a, this change makes an existing hot loop hotter.
9. **Tests:** extend `internal/controller/service_builder_test.go` — (a) nil access strategy → exactly one port 8888 (the existing assertion at test:70-71 stays green); (b) access strategy present but no sidecar ports → still one port; (c) ws-proxy container on 8080 → two ports, correct name/target/protocol; (d) unnamed container port → deterministic fallback name; (e) sidecar declaring 8888 → not duplicated; (f) `NeedsUpdate` returns true when an existing single-port Service must gain 8080, and the update preserves `ClusterIP` (composes with 4a).

**Deliverable:** a PR in `jupyter-k8s-infra` (4a + 4b together are a coherent PR; 4c may be the same or a follow-up). Unit tests required; e2e covered in Task 8.

---

### Task 4c — Admission validation for sidecar ports — **DROPPED (considered, deferred)**

Recorded rather than deleted, because the reasoning that killed it is the useful part.

**The original argument.** An admin fat-fingers a **shared** access strategy so two sidecars both declare 8080. The object is accepted; `accessStrategyEventHandler` enqueues **every** referencing workspace (possibly hundreds); each builds an invalid Service spec, is rejected by the API server, and retries with backoff forever. One typo → cluster-wide, near-silent outage. A validating webhook would turn that into one immediate `kubectl apply` error.

**Why it does not hold.** That story requires the operator to *emit* an invalid Service. It doesn't — 4b step 4 skips duplicate and colliding port numbers and logs them, so the emitted Service is always valid and no workspace ever enters a retry loop. The webhook would not prevent an outage; it would only upgrade a log line into an `apply`-time error. That is real ergonomic value, but it is not the correctness gate the plan claimed, and it does not justify standing up the first `WorkspaceAccessStrategy` webhook (new registration, new marker, `make manifests`, RBAC) inside this PR.

Two supporting premises also turned out to be wrong or already-covered:
- **Name length/charset validation is redundant.** See 4b step 3 — `ServicePort.Name` allows 63 chars and every derived name is already bounded below that. There is nothing to reject.
- **A malformed `ContainerPort.Name` is a pre-existing, unrelated problem.** A CRD structural schema does not validate embedded `[]corev1.Container`, so a bad port name is accepted onto the access strategy today and fails when the **Deployment** is built — a path that exists independently of this work. Fixing it belongs with the deployment path, not here, and would be a behavior change for existing SSM users.

**Follow-up (not this PR):** if the ergonomics matter later, the cheapest version is a single check — duplicate `containerPort` numbers across `additionalContainers` — on a new `WorkspaceAccessStrategy` validating webhook. Reconcile-time skip+log must stay regardless, since objects created before the webhook can always carry duplicates.

---

### Task 5 — WebSocket IngressRoute template **+ NetworkPolicy port**

> **Do not skip the NetworkPolicy half.** Without it, Task 4b is a silent no-op on any cluster with network policies enabled.

#### 5a — NetworkPolicy: the second gate

A **NetworkPolicy** is a firewall rule enforced by the CNI. Two properties matter here: it is **allowlist-only** (once any policy selects a pod, anything not explicitly allowed is denied — there is no "deny" rule, absence *is* denial), and it operates on the **pod**, not the Service. Kubernetes networking is flat: the Service is a virtual IP that kube-proxy rewrites to a pod IP, and the policy is enforced on the resulting pod-to-pod packet. So **the Service and the NetworkPolicy are two independent gates and traffic must pass both.**

Every relevant policy in the repos hardcodes 8888:

| File | Scope | Allows |
|---|---|---|
| `jupyter-k8s-aws/charts/aws-oidc/templates/access-strategy/oauth-access-strategy.yaml:11-39` | per-workspace, as an `accessResourceTemplate` | Traefik → **8888** only |
| `jupyter-k8s-aws/charts/aws-oidc/templates/access-strategy/bearer-access-strategy.yaml:12-40` | same | Traefik → **8888** only |
| `jupyter-k8s-aws/charts/aws-hyperpod/templates/network-policies.yaml:31-49` | cluster-wide, selects `app: jupyter` | Traefik → **8888** only |
| `jupyter-k8s/test/e2e/static/access-strategy/access-strategy-with-resources.yaml:10-27` | e2e fixture | **8888** only |

So with Task 4b alone: `client → Traefik → Service:8080 ✓ → pod:8080 ✗ CNI drops the packet`. The Service routes to a port the firewall blocks; the connection hangs or resets and **nothing logs an error**, because from Kubernetes' point of view everything is configured correctly.

Add the WebSocket port to whichever policies the WebSocket sample depends on. Note the controller itself never touches NetworkPolicy — this is purely chart/template work, which is what lets the design work in both modes below.

**Scale caveat, already acknowledged in the repo** (`aws-oidc/values.yaml:146-150`): `createNetworkPolicy` may be set to `false` "when namespace-level network policies are managed externally … or **when large workspace counts make per-workspace netpols undesirable**." One NetworkPolicy object per workspace × N workspaces is real CNI load. The design must work with per-workspace policies *and* with a namespace-wide policy — verify both.

#### 5b — IngressRoute

A per-workspace Traefik `IngressRoute` that routes the WebSocket path to the proxy port on the workspace Service. Delivered as an `accessResourceTemplate` entry in the sample access strategy (Task 6), rendered by `internal/controller/access_resources_builder.go` (`BuildUnstructuredResource`, template funcs include `b32encode`; available template vars: `.Workspace`, `.AccessStrategy`, `.Service`).

Requirements, matching the operator's real patterns (see the hyperpod reference, **not** invented values):
- **Match:** `Host(<subdomain>) && PathPrefix(/ssh-ws)`. Subdomain pattern: `{{ .Workspace.Name }}-{{ b32encode .Workspace.Namespace }}.<domain>` (same shape the web-UI routes use).
- **entryPoint:** match the cluster's Traefik config. The hyperpod web-UI routes use `web`; confirm whether the target deployment terminates TLS at the LB (entryPoint `web`) or at Traefik (`websecure`) before hardcoding. **Open question — do not guess.**
- **Middleware:** the WebSocket upgrade must be authenticated. The hyperpod chart uses two middlewares: `authmiddleware-verify` (for already-authenticated requests) and `authmiddleware-bearer-auth` (bootstrap→session exchange, paired with `strip-bearer-auth-suffix`). **Decide which applies to the WS upgrade** by reading `internal/authmiddleware/serverroute_bearer_auth.go` and `serverroute_verify.go`: the client sends a bootstrap JWT in `Authorization: Bearer`, so the bearer-auth handler is the likely fit — but confirm it does not force a redirect/suffix-strip that breaks the upgrade. Middleware namespace is the operator's release namespace (hyperpod uses `{{ .Release.Namespace }}`), **not** a hardcoded `jupyter-k8s-system`.
- **Service target:** the workspace Service (`{{ .Service.Name }}` / `{{ .Service.Namespace }}`) on port 8080 — the port added in Task 4. (Because Task 4 puts 8080 on the *existing* Service, the IngressRoute references `.Service.Name`, not a separate `ws-proxy-*` Service.)
- **Priority:** higher than the web-UI catch-all so the `/ssh-ws` prefix wins (hyperpod uses 100 for the catch-all, 110 for the more-specific bearer-auth route; use ≥110).

---

### Task 6 — Sample WorkspaceAccessStrategy (Tasks 5 + 6 delivered together)

A complete, documented sample enabling WebSocket remote access. **No `podEventsHandler`, no `podEventsContext`, no init containers, no shared volumes, no state files** — the proxy is stateless. Contrast with hyperpod, which needs all of those for SSM.

Contents:
- `createConnectionHandler: "k8s-native"` **or** `createConnectionHandlerMap: { ssh-over-websocket: "k8s-native" }` (and optionally `vscode-remote: "k8s-native"` etc. to expose it under IDE-remote types). Document both and pick one for the sample.
- `bearerAuthURLTemplate` resolving to the WS host + `/ssh-ws` (the Extension API swaps scheme to `wss://`). Confirm the template path aligns with the IngressRoute `PathPrefix`.
- `accessResourceTemplates:` the Task 5 WebSocket IngressRoute. (No extra Service template — Task 4 handles the port on the main Service.)
- `deploymentModifications.podModifications.additionalContainers:` the ws-proxy sidecar:
  - image `ghcr.io/jupyter-infra/workspace-websocket-proxy:<version>` (use a pinned pre-release tag once Task R ships a release; avoid `:latest` in the committed sample).
  - `ports: [{ containerPort: 8080, name: ws-proxy }]` — **this is what Task 4 reads to expose the Service port**, so the sample and the code are coupled: the container must declare the port with a name.
  - env: `TARGET_PORT: "2222"`, `MAX_SESSION_DURATION: "12h"`, and any others you want non-default.
  - readiness/liveness probes: exec `["/ws-proxy","--healthcheck"]` (the image's built-in healthcheck) or `httpGet /health :8080`.
  - resource requests/limits (small — the proxy is lightweight).
- Consider an `accessStartupProbe` against the WS path so the workspace is only marked Available once the route serves (hyperpod does this for `/bearer-auth`).

**Decision needed — where the sample lives.** Options: (a) `config/samples/` in the operator (aligns with kubebuilder convention, easy to `kubectl apply`); (b) a guided Helm chart like hyperpod (better for real deployment, more work); (c) docs only. **Recommendation: (a) a committed sample now**, and fold it into a guided chart / the `eks-oidc` template later when integrating (Jonathan wants first integration in the `eks-oidc` template — see §7). Confirm with the user.

---

### Task 7 — Client-side connect script

`ws-connect` shell script:
1. Calls the Extension API via `kubectl` (create a `WorkspaceConnection` with `workspaceConnectionType: ssh-over-websocket`).
2. Parses the returned `wss://…?token=<jwt>` — extracts the token and the token-less URL.
3. Execs `websocat --binary -H="Authorization: Bearer $TOKEN" asyncstdio: <wss-url-without-token>` (keeps the token out of server-side URL logs).
4. Meant to be wired as an SSH `ProxyCommand` so IDEs' Remote-SSH connect over it.

Also: user docs (SSH config setup, `websocat` install, admin deploy guide) and coordinate the AWS Toolkit integration (separate team) to add `ssh-over-websocket` support.

---

### Task 8 — E2E tests (full path)

On a real/kind cluster once Tasks 4–6 are deployed together: create the WebSocket access strategy → create a workspace referencing it → assert the ws-proxy sidecar is injected → assert the Service exposes 8080 (Task 4) → assert the IngressRoute is created → Extension API returns a `wss://` URL → a WebSocket client reaches the remote access server through ingress → proxy → `:2222`.

---

### Task 9 — AWS plugin coexistence

No code changes. Verify the `aws-hyperpod` SSM access strategy and a WebSocket access strategy coexist on the same cluster without conflicting (distinct Services, distinct IngressRoutes, distinct handler routing).

---

### Task R — Proxy repo release hardening (from Slack with Jonathan Guinegagne, 2026)

Jonathan checked in on releasing `workspace-websocket-proxy`. Action items surfaced:
- **Versioning:** first release is a **pre-release**, not `0.1.0` final. Org practice is `0.1.0-rc.x` for images (ref `jupyter-k8s-ui` package). Jonathan explicitly recommended staying on pre-release **until it's integrated in at least one deployment (e.g. the `eks-oidc` template)**. Plan for `v0.1.0-rc.1` first.
- **Repo ownership:** confirmed — the module path and CI publish to `github.com/jupyter-infra`, but the local remote is a personal fork (`git@github.com:earaghbidikashani/workspace-websocket-proxy.git`). Eilia now has admin on the `jupyter-infra` repo; ensure work lands on the org repo, not the fork.
- **Wire e2e into the PR hook.** Currently `pull_request` triggers only `build.yml`, `lint.yml`, `test.yml` (unit only). The kind-based e2e (`make setup-test-e2e` / `test-e2e`, `-tags=e2e`) exists but has **no** workflow. Add an e2e GitHub Actions job on `pull_request`, matching the operator's PR-hook pattern; keep it light enough to run per-PR.
- **License fix:** `LICENSE` still reads `Copyright (c) 2026 Jupyter Infrastructure` → change to `Copyright (c) 2026 Amazon Web Services`. (The `.go` file headers already say Amazon Web Services; only `LICENSE` is stale.)
- **Fix the ping-loop teardown stall** (`session.go:130`, see §4) before cutting the release, since it can wedge the concurrent-connection slots.
- Add branch protection rules on the org repo (the PE offered to set these up).

---

## 7. Notes from Slack (Jonathan Guinegagne)

- Ordering per Eilia's own message: **new WebSocket connection type (done) → release the sidecar → the workspace Service change is the last change needed.** That matches this plan: Task 3 done, Task R (release) and Task 4 (service) are the near-term items.
- Jonathan is enthusiastic about VS Code apps with remote access over this. Target first integration in the **`eks-oidc` template** — that's the gate for promoting the proxy past pre-release.
- Context for viz (not blocking): Karpenter autoscaling landed (`jupyter-deploy#316`), template-aware UI landed (`jupyter-k8s-ui#49`); the next OSS/EKS release aims to approach Parker feature parity incl. console experience.

---

## 8. Key reference files

| File | What it does |
|---|---|
| `api/v1alpha1/workspaceaccessstrategy_types.go` | Access-strategy CRD types (`DeploymentModifications`, `AdditionalContainers`, `BearerAuthURLTemplate`, handler fields) |
| `api/connection/v1alpha1/workspace_connection_types.go` | Connection type constants (`ssh-over-websocket`) |
| `internal/extensionapi/serverroute_connection.go` | Connection routing + `generateWebSocketConnectionURL` |
| `internal/extensionapi/serverroute_connection_test.go` | Tests for the WebSocket handler |
| `internal/controller/service_builder.go` | **Task 4 target** — builds the workspace Service (port 8888 only today) |
| `internal/controller/resource_manager.go` | `createService` / `EnsureServiceExists` / update path — **Task 4 wiring** |
| `internal/controller/state_machine.go` (~L240) | Reconcile call site where `accessStrategy` is already in scope |
| `internal/controller/access_resources_builder.go` | Renders access-resource templates into K8s objects (IngressRoute) — Task 5 |
| `internal/controller/resource_manager_access.go` | Creates/deletes access resources (IngressRoutes) |
| `internal/controller/deployment_builder_access.go` | Applies deployment modifications (sidecar injection) |
| `internal/authmiddleware/serverroute_bearer_auth.go` | Bearer-auth handler (Authorization header) — Task 5 middleware decision |
| `internal/authmiddleware/serverroute_verify.go` | Verify handler — the other candidate middleware for Task 5 |
| `/Users/earaghbi/workplace/jupyter-k8s-aws/charts/aws-hyperpod/templates/hyperpod-access-strategy.yaml` | **Reference** for how access strategies are wired (entryPoints, middleware namespaces, priorities, deploymentModifications) |
| `/Users/earaghbi/workplace/workspace-websocket-proxy/internal/proxy/` | The proxy sidecar implementation |

---

## 9. What the next session should do (in order)

1. ~~**Task 4a**~~ — ✅ done. Owned-field-only compare/assign (`Type`, `Selector`, `Ports`) via `serviceSpecMatches`/`applyServiceSpec`; tests build a defaulted `ClusterIP` and were confirmed to fail against the pre-fix code.
2. ~~**Task 4b**~~ — ✅ done. `accessStrategy` threaded through `BuildService`/`NeedsUpdate`/`UpdateServiceSpec` and the four `resource_manager.go` call sites; `sidecarServicePorts` derives ports from declared `containerPorts` with skip-and-log dedup and verbatim naming + `-N` collision suffix.
3. ~~**Task 4c**~~ — ❌ dropped, see Task 4c for why. **Open item before the 4a+4b PR merges:** verify on a cluster which symptom the 4a bug produces today (hot reconcile loop vs. repeating `spec.clusterIP: Invalid value: ""` error) — grep operator logs for repeated `Updating Service` lines. Also flag the same whole-`Spec` comparison in `pvc_builder.go` as a follow-up (do **not** fix it in this PR).
4. **Task R** — LICENSE fix, ping-loop teardown fix, add e2e PR-hook workflow, cut `v0.1.0-rc.1` from the org repo.
5. **Tasks 5 + 6 — now the critical path.** WebSocket IngressRoute template **and the NetworkPolicy port** (both halves — see 5a), plus the full sample access strategy; resolve the entryPoint and middleware open questions against the code first; decide sample location (recommend `config/samples/`). Until 5a lands, 4b is a silent no-op wherever network policies are enabled.
6. **Task 7** — write the `ws-connect` client script + user/admin docs.
7. **Task 8** — full-path e2e on a cluster (blocked on 4–6 deployed together).
8. **Task 9** — verify AWS-plugin coexistence.
