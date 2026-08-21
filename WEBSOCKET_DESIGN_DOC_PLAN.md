# WebSocket Remote Connection — Design Document Plan

**Purpose**: Plan for the design doc. Each section below describes what to cover when writing the actual content.

---

## Section 1: Context & Motivation

- Explain the current state: remote connections today are SSM-only
- List the problems with SSM-only:
  - AWS lock-in (doesn't work on GKE, AKS, on-prem)
  - Operational complexity (IAM roles, activations, registration, cleanup, state management)
  - Latency overhead (traffic relays through AWS SSM service, ~2-3s connection establishment, 60-165ms per keystroke)
  - Cost (SSM session charges, data transfer)
- Customer pain points: multi-cloud customers, on-prem deployments, latency-sensitive workloads, teams wanting simpler ops
- Why WebSocket is the right answer: cloud-agnostic, direct routing through existing ingress, stateless (no registration/cleanup), sub-second connections, standard protocol, no additional cloud service costs

## Section 2: Goals & Non-Goals

- Goals: add WebSocket as an alternative connection method, sub-second connection establishment, <100ms interactive latency, works on any K8s cluster with an ingress controller, zero impact on existing SSM workspaces, users can switch via configuration
- Non-goals: not replacing SSM (both coexist), not building a custom protocol (using standard WebSocket + existing libraries), not reinventing auth (reusing existing JWT system), not changing workspace features or lifecycle

## Section 3: Architecture Overview

- Side-by-side diagram: SSM path vs WebSocket path
- Call out what's shared (Extension API, Remote Access Server on port 2222, workspace pod, JWT system) vs what's different (SSM agent + AWS SSM service vs websocket-proxy + Traefik ingress)
- Component diagram showing where the new websocket-proxy sidecar fits in the existing pod structure
- Emphasize: no CRD changes, no new services, minimal controller changes

## Section 4: How It Works

- End-to-end connection flow in prose (not code):
  1. User requests connection via Extension API (same endpoint)
  2. Extension API checks access strategy, sees "websocket" handler
  3. Generates JWT token, builds wss:// URL with token
  4. Returns URL to client
  5. Client opens WebSocket to URL
  6. Traefik validates JWT via ForwardAuth
  7. Traefik proxies to websocket-proxy sidecar in pod
  8. Proxy upgrades to WebSocket, dials localhost:2222, copies bytes bidirectionally
  9. SSH session established
- Sequence diagram
- Timing comparison: WebSocket (~sub-second) vs SSM (~2-3 seconds)
- Call out what's NOT needed compared to SSM: no FindInstance, no StartSession, no activation, no registration, no state file

## Section 5: WebSocket Proxy Sidecar

- What it is: a sidecar container that bridges WebSocket connections to TCP (localhost:2222)
- Why sidecar (vs in-process or standalone service): separation of concerns, independent lifecycle, resource isolation, reusable across workspace types, security isolation
- Follow the same pattern as the SSM sidecar: use an existing tool for the hard part, don't reinvent
- Two implementation options:
  - Option 1: websocat (recommended to start) — open-source single-binary tool, one-liner proxy command, no custom code, handles all WebSocket protocol details. Trade-off: no built-in Prometheus metrics, structured logging, or health endpoint.
  - Option 2: Custom Go binary (~200 LOC using gorilla/websocket) — full control over metrics, logging, health, graceful shutdown. Trade-off: custom code to write, test, and maintain.
  - Recommendation: start with Option 1 to ship fast, evaluate Option 2 later if observability gaps matter. Swapping is a container image change, not an operator code change.
- Resource footprint: ~10MB image, 50m CPU / 64Mi memory request, stateless
- Where the image lives: images/websocket-proxy/ in this repo, ships with the operator
- Contrast with SSM sidecar: no daemon, no registration, no state file, no cleanup, 64Mi vs 1Gi memory

## Section 6: Authentication & Security

- Approach: reuse existing JWT infrastructure entirely — no new auth system
- Token flow: Extension API generates JWT with workspace claims → token in WSS URL query param → Traefik ForwardAuth middleware validates before proxying → proxy itself never touches auth
- What's in the JWT: user, workspace name, namespace, path, domain, expiry
- TLS: wildcard cert for *.workspaces.domain.com, wss:// only (no plain ws://)
- Auth happens at the ingress layer — invalid/expired tokens never reach the pod
- Compare to SSM security: SSM has IAM + activation credentials + session tokens + instance credentials. WebSocket has JWT + TLS. Simpler, but relies on ingress being properly secured.
- Note: no new key management or rotation needed (existing JWT rotator handles it)

## Section 7: Ingress & Routing

- How routing works: host-based routing via Traefik IngressRoute, one subdomain per workspace ({workspace}-{namespace-b32}.{domain})
- IngressRoute template lives in the WorkspaceAccessStrategy (same pattern as existing access resource templates)
- JWT validation via Traefik ForwardAuth middleware (already deployed for web UI)
- Infrastructure requirements for cluster admins: ingress controller (Traefik), load balancer with WebSocket support, wildcard DNS, wildcard TLS cert
- Note: Traefik natively supports WebSocket upgrade — no special configuration needed

## Section 8: Connection Handler Architecture

- Current state: generateVSCodeURL() in Extension API is hardcoded to SSM
- New design: strategy/factory pattern — createConnectionHandler field on access strategy routes to the right handler
  - "aws" → SSM handler (existing code, wrapped in interface)
  - "websocket" → WebSocket handler (new, generates JWT + builds WSS URL)
- Interface: ConnectionHandler with GenerateConnectionURL() method
- What changes in Extension API: HandleConnectionCreate() uses factory instead of calling SSM directly. Validation, auth, response format all unchanged.
- No CRD changes: createConnectionHandler and createConnectionContext fields already exist and are generic enough

## Section 9: Configuration

- Example WorkspaceAccessStrategy YAML for WebSocket (show the full resource with createConnectionHandler: "websocket", createConnectionContext with domain, and deploymentModifications with the proxy sidecar container)
- Example Helm values for a guided chart
- What a cluster admin needs to set up: domain name, wildcard DNS pointing to LB, wildcard TLS cert (via cert-manager or manual), that's it — no IAM roles, no SSM documents, no managed node roles
- Contrast with SSM setup requirements

## Section 10: Failure Modes & Reliability

- Connection drops: client reconnects, proxy is stateless, no server-side state to recover
- Pod restarts: all connections terminate, users reconnect, no orphaned resources (contrast with SSM: must deregister instance, delete activation, handle state file)
- Proxy crashes: K8s restarts container, new connections work immediately, no registration needed
- Ingress/LB issues: standard K8s infrastructure concerns, not specific to this feature
- Key point: every SSM failure mode related to state (orphaned instances, expired activations, corrupted state files, concurrent setup races) simply doesn't exist with WebSocket

## Section 11: Rollout & Migration

- Zero impact on existing SSM workspaces — WebSocket is purely additive
- To adopt: create a new WorkspaceAccessStrategy with "websocket" handler, point workspaces at it
- To rollback: switch access strategy back to SSM, restart workspaces
- Gradual adoption: start with dev/test workspaces, monitor, expand
- Both methods can coexist in the same cluster (different access strategies)
- No data migration — workspaces are stateless for connections

## Section 12: Open Questions / Future Work

- VS Code extension: does the existing AWS Toolkit extension support WebSocket URLs, or do we need a separate extension/protocol?
- Connection limits: how many concurrent WebSocket connections per proxy? (goroutine-based, likely 100+ per pod, but needs load testing)
- Reconnection: should the proxy support transparent reconnection, or leave it to the client?
- WebSocket ping/pong: what idle timeout and keepalive settings?
- Guided chart: should we create a new guided chart (e.g., websocket-traefik) or extend existing ones?
- Non-Traefik ingress: does this work with nginx-ingress, Istio, etc.? (should, but needs testing)
