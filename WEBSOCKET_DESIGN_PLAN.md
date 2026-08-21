# WebSocket Remote Connection - Design Document Plan

**Document Version**: 1.0  
**Last Updated**: 2026-03-03  
**Status**: Planning Phase  
**Purpose**: Comprehensive plan for the WebSocket remote connection design document

---

## Document Overview

This document outlines the structure, content requirements, and specifications for the WebSocket Remote Connection design document. The design doc itself will be written based on this plan.

### Target Audience
- **Engineers**: Technical team members who will review and provide feedback on the design
- **Stakeholders**: Leadership and decision-makers who need to understand the approach and rationale

### Writing Guidelines
- **Human-readable**: Minimize code examples, focus on prose explanations
- **Direct and concise**: No analogies, avoid over-explanation
- **Accessible**: Technical but understandable, avoid unnecessary jargon
- **Focused**: Explain "what" and "why" over implementation "how"

---

## Document Structure

### 1. Context & Motivation




**Purpose**: Explain why we need WebSocket support and what problems it solves

**Content Requirements**:

#### 1.1 Problems with Current SSM-Only Approach
- **AWS Dependency**: SSM only works in AWS environments
  - Cannot be used in GKE, AKS, on-premise Kubernetes clusters
  - Locks customers into AWS infrastructure
  - Limits adoption for multi-cloud or non-AWS customers

- **Operational Complexity**: SSM requires significant setup
  - IAM roles and policies configuration
  - SSM activation management
  - Managed instance registration and cleanup
  - Additional AWS service dependencies

- **Performance Overhead**: SSM adds latency
  - Traffic routes through AWS SSM Service (relay/proxy)
  - Additional network hops increase latency
  - Registration process takes 10-15 seconds per pod
  - Not suitable for latency-sensitive workloads

- **Cost Implications**: SSM incurs AWS charges
  - SSM Session Manager usage costs
  - Data transfer costs through AWS
  - Additional infrastructure overhead

#### 1.2 Customer Pain Points
- **Multi-Cloud Customers**: Cannot use remote development features outside AWS
- **On-Premise Deployments**: No remote access option for air-gapped or on-prem clusters
- **Latency-Sensitive Workloads**: SSM latency impacts developer experience
- **Cost-Conscious Teams**: Want to minimize AWS service dependencies
- **Simplified Operations**: Teams want less infrastructure complexity

#### 1.3 Why WebSocket
- **Cloud-Agnostic**: Works on any Kubernetes cluster (AWS, GKE, AKS, on-prem)
- **Lower Latency**: Direct routing through cluster ingress, no external relay
- **Simpler Architecture**: No registration, no cleanup, stateless connections
- **Standard Protocol**: WebSocket is widely supported and well-understood
- **Instant Connections**: No registration overhead, sub-second connection establishment
- **Cost Effective**: No additional cloud service charges

---

### 2. Goals & Non-Goals

**Purpose**: Clearly define what this project will and will not accomplish

#### 2.1 Goals

**Primary Objectives**:
- Add WebSocket as an alternative remote connection method
- Support VSCode Remote-SSH over WebSocket tunneling
- Enable remote development on non-AWS Kubernetes clusters
- Reduce connection latency compared to SSM
- Maintain backward compatibility with existing SSM workspaces

**Success Criteria**:
- WebSocket connections establish in <1 second
- Connection latency <100ms (vs SSM's 200-500ms)
- Zero impact on existing SSM workspaces
- Works on GKE, AKS, and on-premise clusters
- Users can switch between SSM and WebSocket via configuration

#### 2.2 Non-Goals

**Explicitly Out of Scope**:

- **NOT Replacing SSM**: Both connection methods will coexist
  - SSM remains fully supported
  - Customers can choose based on their requirements
  - No deprecation or migration forced

- **NOT Developing Custom WebSocket Protocol**: Using proven tools
  - Leverage existing WebSocket libraries and tools
  - No custom protocol implementation
  - Reuse battle-tested components (websocat or equivalent)

- **NOT Reinventing Authentication**: Reusing existing JWT system
  - Extension API already has JWT token generation
  - Token validation middleware already exists
  - No new authentication mechanism needed

- **NOT Adding New Workspace Features**: Just connection method
  - No changes to workspace lifecycle
  - No new workspace capabilities
  - Pure infrastructure change for connectivity

---

### 3. Considerations

**Purpose**: Key factors to keep in mind during design and implementation

#### 3.1 Backward Compatibility
- **Critical Requirement**: Existing SSM workspaces must continue working unchanged
- No breaking changes to APIs or CRDs
- Existing WorkspaceAccessStrategy configurations remain valid
- SSM connection flow completely unaffected
- Users can continue using SSM indefinitely

#### 3.2 Performance Implications
- **Latency**: WebSocket should provide <100ms latency for interactive use
- **Throughput**: Must support typical SSH traffic patterns (commands, file transfers)
- **Resource Usage**: WebSocket proxy should be lightweight (<64Mi memory, <50m CPU)
- **Connection Overhead**: Minimal overhead per connection (<10ms)

#### 3.3 Scalability
- **Concurrent Connections**: How many WebSocket connections can a single pod handle?
  - Target: 10-50 concurrent connections per workspace pod
  - Each connection = 2 goroutines + ~32KB buffer
  - Memory scaling: ~2MB per connection
  - CPU scaling: Minimal when idle, spikes during data transfer

- **Cluster-Wide Scale**: How many total workspaces with WebSocket?
  - No centralized bottleneck (each pod has own proxy)
  - Scales horizontally with workspace count
  - Limited only by cluster resources and ingress capacity

#### 3.4 Operational Complexity
- **Deployment**: Additional sidecar container per workspace
- **Monitoring**: Need metrics for WebSocket connections (active, errors, throughput)
- **Debugging**: Need visibility into connection failures
- **Configuration**: Additional fields in WorkspaceAccessStrategy

#### 3.5 Infrastructure Requirements
- **Ingress Controller**: Requires Traefik or equivalent with WebSocket support
- **Load Balancer**: Must support WebSocket upgrade and long-lived connections
- **DNS**: Wildcard DNS for host-based routing (*.workspaces.domain.com)
- **TLS Certificates**: Wildcard certificate for secure WebSocket (wss://)
- **Network Policies**: May need adjustments to allow ingress to workspace pods

#### 3.6 Failure Modes and Reliability
- **Connection Drops**: How to handle network interruptions?
  - VSCode extension handles reconnection
  - WebSocket proxy should fail gracefully
  - Clear error messages to users

- **Pod Restarts**: What happens when workspace pod restarts?
  - Connections terminate (expected behavior)
  - Users reconnect after pod is ready
  - No state to recover (stateless design)

- **Proxy Failures**: What if WebSocket proxy crashes?
  - Kubernetes restarts container automatically
  - Existing connections drop, users reconnect
  - No impact on workspace application

---

### 4. Design

**Purpose**: Evaluate alternatives and present recommended approach for each critical component

#### 4.1 Connection Methods

**Evaluation Criteria**: Cloud portability, latency, complexity, cost

**Alternatives**:

1. **WebSocket Tunneling** (RECOMMENDED)
   - **How it works**: WebSocket connection from client → Ingress → WebSocket proxy sidecar → Remote Access Server
   - **Pros**:
     * Cloud-agnostic (works anywhere)
     * Low latency (direct routing)
     * Simple architecture (no registration)
     * Standard protocol (widely supported)
     * Instant connections (no setup time)
   - **Cons**:
     * Requires ingress/load balancer
     * Requires DNS configuration
     * Requires TLS certificate management
   - **Use Cases**: Non-AWS clusters, latency-sensitive workloads, simplified operations

2. **AWS SSM Tunneling** (EXISTING, KEEPING)
   - **How it works**: Client → AWS SSM Service → SSM Agent sidecar → Remote Access Server
   - **Pros**:
     * No ingress required (tunnels outbound)
     * Works in locked-down networks
     * AWS-managed security and audit
   - **Cons**:
     * AWS-only (doesn't work on GKE, AKS, on-prem)
     * Higher latency (relay through AWS)
     * Registration overhead (10-15 seconds)
     * Additional AWS costs
   - **Use Cases**: AWS-only deployments, compliance requirements, no-ingress environments

3. **Direct SSH** (NOT RECOMMENDED)
   - **How it works**: Direct SSH connection to workspace pod via LoadBalancer
   - **Pros**:
     * Simple, no proxy needed
     * Standard SSH protocol
   - **Cons**:
     * Requires LoadBalancer per workspace (expensive)
     * No authentication integration
     * Security concerns (direct pod exposure)
     * Port management complexity
   - **Why Not**: Cost prohibitive, security risks, doesn't scale

**Recommendation**: Implement WebSocket, keep SSM, allow users to choose based on infrastructure

---

#### 4.2 WebSocket Proxy Implementation

**Evaluation Criteria**: Separation of concerns, resource efficiency, maintainability, reusability

**How WebSocket Works**:
1. User requests connection via Extension API
2. Extension API generates JWT token with workspace claims
3. Extension API returns WebSocket URL: `wss://workspace-namespace.domain.com/ssh-ws?token=JWT`
4. VSCode extension establishes WebSocket connection to URL
5. Traefik ingress routes request to workspace pod
6. Traefik middleware validates JWT token
7. WebSocket proxy sidecar accepts connection
8. Proxy upgrades HTTP to WebSocket protocol
9. Proxy establishes TCP connection to Remote Access Server (localhost:2222)
10. Proxy bidirectionally copies data: WebSocket frames ↔ TCP bytes

**New Components Needed**:
- **WebSocket Proxy Sidecar**: Container that handles WebSocket protocol
- **Websocat Tool** (or equivalent): Handles WebSocket protocol details
  - Why needed: WebSocket protocol is complex (framing, masking, control frames)
  - Websocat is battle-tested, handles edge cases
  - Alternative: Custom Go implementation using gorilla/websocket library
- **IngressRoute Configuration**: Traefik routing rules for WebSocket
- **JWT Middleware**: Token validation (already exists, reuse)

**Alternatives**:

1. **Sidecar Container** (RECOMMENDED)
   - **How it works**: Separate container in workspace pod runs WebSocket proxy
   - **Pros**:
     * Separation of concerns (proxy ≠ workspace app)
     * Independent lifecycle (proxy can restart without affecting workspace)
     * Resource isolation (separate resource limits)
     * Reusable across workspace types (JupyterLab, VSCode Server, etc.)
     * Security isolation (proxy runs with minimal privileges)
   - **Cons**:
     * Additional container per pod (resource overhead)
     * Slightly more complex pod configuration
   - **Resource Requirements**:
     * Memory: 64Mi request, 128Mi limit
     * CPU: 50m request, 200m limit
     * Disk: ~15MB container image
   - **Implementation Details**:
     * Container listens on port 8080
     * Connects to localhost:2222 (Remote Access Server in main container)
     * Uses shared network namespace (localhost communication)
     * No shared volumes needed (stateless)

2. **In-Process with Workspace**
   - **How it works**: WebSocket proxy embedded in workspace application
   - **Pros**:
     * No additional container
     * Slightly lower resource usage
   - **Cons**:
     * Couples proxy to workspace image (must rebuild all images)
     * Cannot update proxy independently
     * Workspace app must handle WebSocket protocol
     * Different implementation per workspace type
     * Security: proxy runs with workspace privileges
   - **Why Not**: Tight coupling, maintenance burden, not reusable

3. **Standalone Service**
   - **How it works**: Centralized WebSocket proxy service, routes to workspace pods
   - **Pros**:
     * Single proxy for all workspaces
     * Easier to update (one deployment)
   - **Cons**:
     * Centralized bottleneck (all traffic through one service)
     * Complex routing (must route to correct pod)
     * Single point of failure
     * Doesn't scale horizontally with workspaces
   - **Why Not**: Scalability concerns, complexity, single point of failure

**Recommendation**: Sidecar container approach for separation of concerns and reusability

---

#### 4.3 Authentication

**Approach**: Reuse existing JWT token system from Extension API

**Why This Works**:
- Extension API already generates JWT tokens for web UI authentication
- Token signing infrastructure already exists (HS256 or AWS KMS)
- Token validation middleware already deployed (Traefik ForwardAuth)
- Same token format works for WebSocket authentication
- No new key management or rotation needed

**Token Flow**:
1. User requests connection via Extension API
2. API validates user has access to workspace (existing authorization)
3. API generates JWT token with claims:
   - User identity
   - Workspace name and namespace
   - Connection type (vscode-remote)
   - Expiration time (configurable, default 1 hour)
4. Token included in WebSocket URL as query parameter
5. Traefik middleware validates token before proxying to pod
6. Invalid/expired tokens rejected at ingress (never reach pod)

**No New Development Required**:
- JWT generation: Already implemented
- Token signing: Already configured
- Token validation: Already deployed
- Key rotation: Already automated

---

### 5. Rollout & Migration

**Purpose**: Explain how to adopt WebSocket without disrupting existing users

#### 5.1 No Impact on Existing Workspaces

**Existing SSM Workspaces**:
- Continue working exactly as before
- No configuration changes required
- No API changes
- No performance impact
- Can remain on SSM indefinitely

**Why No Impact**:
- WebSocket is additive, not replacement
- SSM code path completely unchanged
- WorkspaceAccessStrategy with `createConnectionHandler: "aws"` continues working
- No breaking changes to CRDs or APIs

#### 5.2 Adopting WebSocket

**For New Workspaces**:
1. Create WorkspaceAccessStrategy with `createConnectionHandler: "websocket"`
2. Configure domain in `createConnectionContext`
3. Deploy workspace referencing new access strategy
4. Workspace automatically gets WebSocket proxy sidecar
5. Users connect via WebSocket URL from Extension API

**For Existing Workspaces** (Optional Migration):
1. Create new WorkspaceAccessStrategy with WebSocket configuration
2. Update workspace to reference new access strategy
3. Workspace recreated with WebSocket proxy sidecar
4. Users can now connect via WebSocket

**Configuration Example**:
```yaml
apiVersion: workspace.jupyter.org/v1alpha1
kind: WorkspaceAccessStrategy
metadata:
  name: websocket-access
spec:
  displayName: "WebSocket Remote Access"
  createConnectionHandler: "websocket"
  createConnectionContext:
    domain: "workspaces.example.com"
  deploymentModifications:
    podModifications:
      additionalContainers:
        - name: websocket-proxy
          image: websocket-proxy:v1.0.0
          ports:
            - containerPort: 8080
              name: websocket
```

**User Experience**:
- No changes to VSCode extension
- Extension API returns appropriate URL based on access strategy
- Users don't need to know which method is used
- Transparent switching between SSM and WebSocket

#### 5.3 Rollback Strategy

**If Issues Arise**:
1. Update WorkspaceAccessStrategy back to SSM configuration
2. Restart affected workspaces
3. Users automatically use SSM again
4. No data loss (workspaces are stateless for connections)

**Gradual Adoption**:
- Start with test/dev workspaces
- Monitor metrics and user feedback
- Gradually expand to more workspaces
- Keep SSM as fallback option

---

## Diagrams Specification

### Diagram 1: Component Architecture (SSM + WebSocket)

**Type**: Component diagram showing both connection methods in one view

**Components to Include**:

1. **User** (external actor)
   - Represents developer using VSCode

2. **Kubernetes Cluster** (main boundary)
   
   **kube-system namespace**:
   - API Server
   - Traefik Ingress Controller
   - Load Balancer (external to cluster, connected to Traefik)

   **Operator namespace** (jupyter-k8s-system):
   - Extension API (REST API service)
   - Controller (workspace reconciliation)
   - Auth Middleware (JWT validation service)

   **Workspace namespace** (user workspaces):
   - Workspace Deployment
   - Workspace Service (ClusterIP)
   - Workspace Pod containing:
     * Main Container (JupyterLab/workspace application)
       - Includes Remote Access Server on port 2222
     * SSM Agent Sidecar
     * WebSocket Proxy Sidecar

3. **External Services** (outside cluster):
   - AWS SSM Service (for SSM connection path)

**Connection Flows to Show**:

**SSM Path** (existing):
- User → AWS SSM Service (WebSocket)
- AWS SSM Service → SSM Agent Sidecar (WebSocket)
- SSM Agent Sidecar → Remote Access Server in Main Container (TCP localhost:2222)

**WebSocket Path** (new):
- User → Load Balancer (HTTPS/WSS)
- Load Balancer → Traefik Ingress (in kube-system)
- Traefik → Auth Middleware (JWT validation)
- Traefik → Workspace Service
- Workspace Service → WebSocket Proxy Sidecar (in pod)
- WebSocket Proxy Sidecar → Remote Access Server in Main Container (TCP localhost:2222)

**Visual Requirements**:
- Clean layout, not too minimal
- Show namespace boundaries clearly
- Show pod internal structure (3 containers)
- Show both connection paths with arrows
- Container-level detail for workspace pod

**Format**: draw.io XML

---

### Diagram 2: Connection Flow Sequence

**Type**: Sequence diagram showing both SSM and WebSocket connection establishment

**Participants**:
1. User (VSCode)
2. Extension API
3. AWS SSM Service (for SSM flow)
4. Traefik Ingress (for WebSocket flow)
5. Auth Middleware (for WebSocket flow)
6. SSM Agent Sidecar (for SSM flow)
7. WebSocket Proxy Sidecar (for WebSocket flow)
8. Remote Access Server

**SSM Connection Flow** (left side or top):
1. User → Extension API: Request connection (workspace name)
2. Extension API → Extension API: Validate user access
3. Extension API → AWS SSM: Find managed instance by pod UID
4. Extension API → AWS SSM: Start session with port forwarding
5. Extension API → User: Return VSCode URL with session info
6. User → AWS SSM Service: Establish WebSocket connection
7. AWS SSM Service → SSM Agent Sidecar: Forward connection
8. SSM Agent Sidecar → Remote Access Server: TCP connection to localhost:2222
9. Remote Access Server → User: SSH session established

**WebSocket Connection Flow** (right side or bottom):
1. User → Extension API: Request connection (workspace name)
2. Extension API → Extension API: Validate user access
3. Extension API → Extension API: Generate JWT token
4. Extension API → User: Return WebSocket URL with JWT
5. User → Traefik Ingress: WebSocket connection request with JWT
6. Traefik → Auth Middleware: Validate JWT token
7. Auth Middleware → Traefik: Token valid
8. Traefik → WebSocket Proxy Sidecar: Forward WebSocket connection
9. WebSocket Proxy Sidecar → Remote Access Server: TCP connection to localhost:2222
10. Remote Access Server → User: SSH session established

**Detail Level**:
- Higher level than every HTTP request/response
- Show key decision points (validation, token generation)
- Show both flows in one diagram for comparison
- Emphasize differences (AWS SSM vs direct routing)

**Visual Requirements**:
- Clean sequence diagram
- Both flows visible for comparison
- Clear participant labels
- Numbered steps

**Format**: draw.io XML

---

## Next Steps

1. **Review this plan** with team and stakeholders
2. **Generate diagrams** based on specifications above
3. **Write design document** following this structure
4. **Iterate on design** based on feedback
5. **Finalize and approve** design document
6. **Begin implementation** based on approved design

---

## Document Metadata

**Reviewers**: [To be filled]  
**Approval Status**: Draft  
**Related Documents**:
- WEBSOCKET_IMPLEMENTATION_PLAN.md (detailed technical implementation)
- SSM_REMOTE_CONNECTION_IMPLEMENTATION.md (existing SSM documentation)

