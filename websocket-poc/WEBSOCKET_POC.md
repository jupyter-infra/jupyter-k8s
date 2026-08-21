# WebSocket Remote Connection POC

## Overview

This POC proves that WebSocket can be used to tunnel remote connections (SSH/VS Code) to workspace pods through existing Kubernetes infrastructure (load balancer + Traefik ingress).

**Goal**: Demonstrate feasibility before building full operator integration.

**Time**: 30-60 minutes

---

## What This POC Proves

| Aspect | What We're Testing |
|--------|-------------------|
| **WebSocket tunneling** | Can WebSocket tunnel TCP connections? |
| **Protocol compatibility** | Does SSH (complex protocol) work through WebSocket? |
| **Infrastructure compatibility** | Does our load balancer + Traefik handle WebSocket? |
| **Latency** | Is the latency acceptable for interactive use? |
| **Stability** | Can connections stay open during sustained use? |
| **Production path** | Does the full internet → LB → Traefik → pod path work? |

---

## Architecture

### POC Architecture
```
Your Laptop                    Kubernetes Pod
┌──────────────┐              ┌─────────────────────┐
│              │  WebSocket   │                     │
│  websocat    │ ──────────>  │  websocat (proxy)   │
│  (client)    │              │         ↓           │
│              │              │    SSH Server       │
│  SSH client  │              │    (port 2222)      │
└──────────────┘              └─────────────────────┘
```

### Production Architecture (What This Represents)
```
User's Laptop                                    Workspace Pod
┌──────────────┐                                ┌─────────────────────┐
│              │                                │                     │
│  VS Code     │  WebSocket                    │  websocket-proxy    │
│  Extension   │ ────────────────────────────> │  (Go sidecar)       │
│              │                                │         ↓           │
│              │                                │  Remote Access      │
│              │                                │  Server (port 2222) │
└──────────────┘                                └─────────────────────┘
       │                                                  ↑
       │                                                  │
       └──> Load Balancer ──> Traefik ──> IngressRoute ─┘
```

---

## Phase 1: Basic WebSocket Tunnel (Port-Forward)

### Step 1: Create All-in-One Test Pod

#### What This Represents in Real Architecture

This represents a **Workspace pod** with the websocket-proxy sidecar. We're combining both into one container for simplicity, but in production they'd be separate containers in the same pod. The SSH server stands in for the remote access server that already exists in workspace pods.

#### What's Missing/Simplified

**Missing from POC**:
- No operator creating this pod - we're doing it manually
- No WorkspaceAccessStrategy template
- No authentication or JWT validation
- No metrics or logging
- No resource limits or security policies
- No persistent storage

**Simplified**:
- Single container instead of sidecar pattern
- Standard SSH instead of custom remote access server
- Simple password auth instead of proper authentication
- No health checks or readiness probes

#### Migration Path to Production

**To make this production-ready**:
1. Split into two containers (main + websocket-proxy sidecar)
2. Replace SSH with actual remote access server
3. Add JWT authentication to websocket-proxy
4. Add Prometheus metrics and structured logging
5. Have operator generate this from WorkspaceAccessStrategy template
6. Add owner references for automatic cleanup

#### Create the Pod

Create file `websocket-poc-pod.yaml`:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: websocket-poc
  labels:
    app: websocket-poc
spec:
  containers:
  - name: all-in-one
    image: alpine:latest
    command: ["/bin/sh", "-c"]
    args:
    - |
      # Install dependencies
      apk add --no-cache openssh-server curl
      
      # Configure SSH server
      ssh-keygen -A
      echo "root:testpass" | chpasswd
      echo "PermitRootLogin yes" >> /etc/ssh/sshd_config
      echo "PasswordAuthentication yes" >> /etc/ssh/sshd_config
      
      # Start SSH server in background on port 2222
      /usr/sbin/sshd -D -p 2222 &
      
      # Install websocat
      curl -L https://github.com/vi/websocat/releases/download/v1.12.0/websocat.x86_64-unknown-linux-musl -o /usr/local/bin/websocat
      chmod +x /usr/local/bin/websocat
      
      # Start WebSocket proxy (forwards ws:8080 -> tcp:2222)
      echo "WebSocket proxy starting on port 8080..."
      websocat -s 8080 tcp:localhost:2222
    ports:
    - containerPort: 8080
      name: websocket
---
apiVersion: v1
kind: Service
metadata:
  name: websocket-poc-service
spec:
  selector:
    app: websocket-poc
  ports:
  - name: websocket
    port: 8080
    targetPort: 8080
  type: ClusterIP
```

**Deploy**:
```bash
kubectl apply -f websocket-poc-pod.yaml
```

**Verify pod is running**:
```bash
kubectl get pod websocket-poc
# Should show STATUS: Running after ~30 seconds

# Check logs to verify both services started
kubectl logs websocket-poc
# Should see: "WebSocket proxy starting on port 8080..."
```

---

### Step 2: Install WebSocket Client on Your Laptop

#### What This Represents in Real Architecture

This represents the **VS Code extension** that would run on the user's laptop. The websocat tool is doing what the extension would do - establishing a WebSocket connection and tunneling data through it. In production, this would be TypeScript code in the VS Code extension using the `ws` library.

#### What's Missing/Simplified

**Missing from POC**:
- No VS Code extension UI
- No connection API call to get workspace URL
- No JWT token in the connection URL
- No user-friendly error messages
- No automatic reconnection logic
- No workspace selection interface

**Simplified**:
- Command-line tool instead of VS Code integration
- Manual URL construction instead of API call
- Using SSH protocol instead of VS Code remote protocol
- No authentication flow

#### Migration Path to Production

**To make this production-ready**:
1. Build TypeScript VS Code extension
2. Add "Connect to Workspace" command to VS Code
3. Call operator API to list workspaces and get connection URL
4. Use `ws` library to establish WebSocket connection
5. Integrate with VS Code's remote connection API
6. Add authentication flow (AWS credentials or OAuth)
7. Package and publish extension to marketplace

#### Install websocat

**On macOS**:
```bash
brew install websocat
```

**On Linux**:
```bash
curl -L https://github.com/vi/websocat/releases/download/v1.12.0/websocat.x86_64-unknown-linux-musl -o websocat
chmod +x websocat
sudo mv websocat /usr/local/bin/
```

**Verify installation**:
```bash
websocat --version
# Should show: websocat 1.12.0
```

---

### Step 3: Test WebSocket Connection via Port-Forward

#### What This Represents in Real Architecture

This represents **nothing in production** - it's purely a testing shortcut. Port-forward bypasses all the networking layers (load balancer, ingress) to give us a direct path to the pod. This lets us test the WebSocket tunnel in isolation before adding ingress complexity.

#### What's Missing/Simplified

**Missing from POC**:
- No load balancer
- No Traefik ingress controller
- No TLS encryption
- No DNS hostname
- No authentication

**Simplified**:
- Using localhost instead of real domain
- Requires kubectl access (real users won't have this)
- Single user only (can't share the connection)

#### Migration Path to Production

**To make this production-ready**:
1. Remove port-forward entirely (not used in production)
2. Users connect through ingress path instead
3. This step only exists for POC testing

#### Start Port-Forward

```bash
kubectl port-forward pod/websocket-poc 8080:8080
```

**Leave this running** in the terminal. You should see:
```
Forwarding from 127.0.0.1:8080 -> 8080
Forwarding from [::1]:8080 -> 8080
```

---

### Step 4: Test Basic WebSocket Connection

#### What We're Testing

Can the websocat client on your laptop connect to the websocat server in the pod via WebSocket?

#### Test Command

Open a **new terminal** (keep port-forward running) and run:

```bash
echo "hello world" | websocat ws://localhost:8080
```

**Expected output**: Nothing, or connection closes immediately.

**Why**: The SSH server receives "hello world" but doesn't know what to do with it (it's not valid SSH protocol), so it closes the connection.

**What this proves**: WebSocket connection establishes successfully. Data flows from your laptop → WebSocket → pod.

---

### Step 5: Test SSH Through WebSocket

#### What We're Testing

Can the SSH protocol work through the WebSocket tunnel? This is the real test - if SSH works, we know complex bidirectional protocols can tunnel through WebSocket.

#### Test Command

```bash
ssh -o ProxyCommand='websocat ws://localhost:8080' root@dummy
```

**When prompted for password, enter**: `testpass`

**Expected result**: You get a shell prompt that looks like:
```
Welcome to Alpine!
websocket-poc:~#
```

**What this proves**:
- WebSocket tunnel works for complex protocols
- Bidirectional data flow works (SSH sends data both ways constantly)
- The connection is stable enough for interactive use

---

### Step 6: Test Interactive Commands

#### What We're Testing

Does the connection work for real interactive use? Can you run commands and see output?

#### Test Commands

While connected via SSH (from Step 5), run:

```bash
# Check where you are
pwd
# Output: /root

# List files
ls -la
# Output: Shows directory contents

# Create a file
echo "WebSocket POC test" > /tmp/test.txt

# Read it back
cat /tmp/test.txt
# Output: WebSocket POC test

# Check system info
uname -a
# Output: Linux websocket-poc ...

# Exit
exit
```

**Expected result**: All commands execute normally and you see output immediately.

**What this proves**:
- Terminal interaction works (command input, output display)
- Data flows both directions reliably
- Latency is acceptable (commands feel responsive)
- The connection doesn't drop during use

---

### Step 7: Test Connection Stability

#### What We're Testing

Can the connection handle sustained use without dropping?

#### Test Command

Connect via SSH again and run:

```bash
ssh -o ProxyCommand='websocat ws://localhost:8080' root@dummy
# Password: testpass

# Generate some load
for i in $(seq 1 100); do
  echo "Test $i"
  sleep 0.1
done
```

**Expected result**: You see "Test 1" through "Test 100" without disconnection.

**What this proves**: The WebSocket connection is stable under sustained use.

---

### Phase 1 Complete ✓

**What you've proven**:
- ✅ WebSocket can tunnel TCP connections
- ✅ SSH protocol works through WebSocket
- ✅ Interactive terminal sessions work
- ✅ Connection is stable

**What you haven't tested yet**:
- ❌ Connection through load balancer
- ❌ Connection through Traefik ingress
- ❌ TLS/WSS (secure WebSocket)
- ❌ Real hostname (DNS resolution)

---

## Phase 2: WebSocket Through Ingress (Production Path)

### Step 8: Create IngressRoute

#### What This Represents in Real Architecture

This is the **IngressRoute resource** that the operator would create for each workspace. In production, the WorkspaceAccessStrategy would have a template for this, and the operator would generate it with the workspace's unique hostname. Traefik watches for IngressRoute resources and automatically updates its routing table.

#### What's Missing/Simplified

**Missing from POC**:
- No operator generating this - we're creating it manually
- No dynamic hostname generation - we're hardcoding it
- No owner references - won't auto-delete with workspace
- No access control validation
- No workspace-specific path routing

**Simplified**:
- Static hostname instead of per-workspace dynamic hostnames
- No namespace isolation
- No resource quotas or rate limiting

#### Migration Path to Production

**To make this production-ready**:
1. Add Go template in WorkspaceAccessStrategy for IngressRoute
2. Operator renders template with variables: `{{ .WorkspaceName }}`, `{{ .Namespace }}`, `{{ .Domain }}`
3. Add owner reference pointing to Workspace resource
4. Generate unique hostname: `workspace-abc123-namespace.workspaces.example.com`
5. Add path-based routing for `/ssh-ws` prefix
6. Validate TLS certificate exists before marking workspace Ready
7. Integrate with Workspace ACL for access control

#### Prerequisites

You need:
- Traefik installed in your cluster
- A load balancer for Traefik (should already exist)
- DNS configured (we'll set this up)
- cert-manager for TLS (optional but recommended)

**Check if Traefik is installed**:
```bash
kubectl get service -n kube-system | grep traefik
# or
kubectl get service -A | grep traefik
```

You should see a service with `TYPE: LoadBalancer` and an `EXTERNAL-IP`.

#### Get Your Load Balancer IP

```bash
kubectl get service -A | grep LoadBalancer
```

Look for the Traefik service and note its `EXTERNAL-IP`. Example:
```
kube-system   traefik   LoadBalancer   10.96.0.1   203.0.113.100   80:30080/TCP,443:30443/TCP
```

The `EXTERNAL-IP` (203.0.113.100 in this example) is what you need.

#### Configure DNS

You need to point a hostname to your load balancer IP.

**Option A: Use /etc/hosts (Testing Only)**

Edit `/etc/hosts` on your laptop:
```bash
sudo nano /etc/hosts
```

Add this line (replace with your actual load balancer IP):
```
203.0.113.100  poc.workspaces.example.com
```

**Option B: Use Real DNS (Recommended)**

In your DNS provider (Route53, Cloudflare, etc.), create an A record:
```
poc.workspaces.example.com  →  203.0.113.100
```

**Verify DNS works**:
```bash
ping poc.workspaces.example.com
# Should show your load balancer IP
```

#### Create IngressRoute

Create file `websocket-poc-ingress.yaml`:

**For HTTP (no TLS)**:
```yaml
apiVersion: traefik.io/v1alpha1
kind: IngressRoute
metadata:
  name: websocket-poc-route
spec:
  entryPoints:
    - web
  routes:
  - match: Host(`poc.workspaces.example.com`)
    kind: Rule
    services:
    - name: websocket-poc-service
      port: 8080
```

**For HTTPS/TLS (if you have cert-manager)**:
```yaml
apiVersion: traefik.io/v1alpha1
kind: IngressRoute
metadata:
  name: websocket-poc-route
spec:
  entryPoints:
    - websecure
  routes:
  - match: Host(`poc.workspaces.example.com`)
    kind: Rule
    services:
    - name: websocket-poc-service
      port: 8080
  tls:
    certResolver: letsencrypt  # Or whatever your cert resolver is named
```

**Deploy**:
```bash
kubectl apply -f websocket-poc-ingress.yaml
```

**Verify IngressRoute was created**:
```bash
kubectl get ingressroute websocket-poc-route
```

---

### Step 9: Test Connection Through Ingress

#### What This Represents in Real Architecture

This is the **production connection path**: User → Internet → Load Balancer → Traefik → Service → Pod. This is exactly how users would connect to workspaces in production. We're testing the full networking stack to ensure WebSocket works through all the layers.

#### What's Missing/Simplified

**Missing from POC**:
- No JWT authentication in URL
- No user identity validation
- No Workspace ACL checks
- No connection metadata or audit logging
- No rate limiting or DDoS protection

**Simplified**:
- Using a single static hostname instead of per-workspace hostnames
- No path-based routing (production would use `/ssh-ws` prefix)
- No connection pooling or load balancing across multiple workspace replicas

#### Migration Path to Production

**To make this production-ready**:
1. Add JWT token to WebSocket URL: `wss://workspace.example.com/ssh-ws?token=<jwt>`
2. Websocket-proxy validates token before accepting connection
3. Operator generates unique hostname per workspace
4. Add path prefix `/ssh-ws` to distinguish from web UI traffic
5. Add connection metadata (user, timestamp, workspace ID) to logs
6. Implement rate limiting per user/workspace
7. Add monitoring and alerting for connection failures

#### Stop Port-Forward

Go back to the terminal running port-forward and press `Ctrl+C` to stop it.

We're now testing through the real ingress path, not port-forward.

#### Test Connection Through Load Balancer

**If using HTTP (no TLS)**:
```bash
ssh -o ProxyCommand='websocat ws://poc.workspaces.example.com' root@dummy
# Password: testpass
```

**If using HTTPS/TLS**:
```bash
ssh -o ProxyCommand='websocat wss://poc.workspaces.example.com' root@dummy
# Password: testpass
```

**Expected result**: You get a shell prompt, just like in Phase 1.

**What this proves**:
- Load balancer passes WebSocket traffic correctly
- Traefik routes based on hostname correctly
- TLS termination works (if using WSS)
- The full production path works end-to-end

---

### Step 10: Test from Different Network

#### What We're Testing

Does this work from outside your local network? This simulates a real user connecting from the internet.

#### Test Command

**Option A: Use your phone's hotspot**

1. Connect your laptop to your phone's hotspot (different network)
2. Run the same SSH command:
```bash
ssh -o ProxyCommand='websocat wss://poc.workspaces.example.com' root@dummy
```

**Option B: Use a different machine**

1. SSH into a different machine (EC2 instance, home server, etc.)
2. Install websocat on that machine
3. Run the same SSH command

**Expected result**: Connection works from any network.

**What this proves**: The solution works for real users connecting from the internet, not just from your local network.

---

### Step 11: Measure Latency

#### What We're Testing

Is the latency acceptable for interactive use? How much overhead does WebSocket add?

#### Test Commands

Connect via SSH and run:

```bash
ssh -o ProxyCommand='websocat wss://poc.workspaces.example.com' root@dummy

# Inside the pod, measure command execution time
time ls
time echo "test"
time cat /etc/os-release
```

**Expected result**: Commands complete in < 100ms.

**Compare to direct connection** (if you have direct SSH access to a pod):
```bash
# Direct SSH (for comparison)
kubectl exec -it websocket-poc -- sh
time ls
```

**What this proves**: WebSocket doesn't add significant latency. The user experience is acceptable.

---

### Phase 2 Complete ✓

**What you've proven**:
- ✅ WebSocket works through load balancer
- ✅ Traefik routes WebSocket traffic correctly
- ✅ TLS/WSS works (if configured)
- ✅ DNS resolution works
- ✅ Connection works from any network
- ✅ Latency is acceptable
- ✅ **The full production path is viable**

---

## POC Results Summary

### What You've Proven

| Aspect | Status | Evidence |
|--------|--------|----------|
| **WebSocket tunneling** | ✅ Proven | SSH works through WebSocket |
| **Protocol compatibility** | ✅ Proven | Complex protocols (SSH) work |
| **Infrastructure compatibility** | ✅ Proven | Works with existing load balancer + Traefik |
| **Latency** | ✅ Proven | Measured and acceptable |
| **Stability** | ✅ Proven | Sustained connections work |
| **Production path** | ✅ Proven | Full internet → LB → Traefik → pod works |

### What You Haven't Built

- Operator integration (WorkspaceAccessStrategy templates)
- JWT authentication
- VS Code extension
- Metrics and logging
- Custom Go websocket-proxy (used websocat instead)
- Per-workspace hostnames
- Access control validation

### Next Steps

**If POC is successful**, you can proceed with:
1. Write custom Go websocket-proxy (~200 LOC)
2. Add JWT authentication
3. Create WorkspaceAccessStrategy template
4. Build VS Code extension
5. Integrate with operator

**Estimated effort**: 2-3 weeks for full implementation.

**If POC fails**, you know WebSocket isn't viable and can stick with SSM.

---

## Troubleshooting

### Pod Won't Start

**Check logs**:
```bash
kubectl logs websocket-poc
```

**Common issues**:
- SSH keygen failed: Check if `/dev/random` is available
- websocat download failed: Check internet connectivity from pod
- Port already in use: Delete and recreate pod

### Can't Connect via Port-Forward

**Check port-forward is running**:
```bash
# Should show "Forwarding from..." messages
```

**Try different port**:
```bash
kubectl port-forward pod/websocket-poc 9090:8080
# Then use ws://localhost:9090
```

### Can't Connect via Ingress

**Check IngressRoute exists**:
```bash
kubectl get ingressroute websocket-poc-route
```

**Check Traefik logs**:
```bash
kubectl logs -n kube-system -l app.kubernetes.io/name=traefik
```

**Verify DNS**:
```bash
nslookup poc.workspaces.example.com
# Should return your load balancer IP
```

**Check TLS certificate** (if using HTTPS):
```bash
kubectl get certificate -A
```

### Connection Drops Immediately

**Check websocat server logs**:
```bash
kubectl logs websocket-poc
```

**Try with verbose logging**:
```bash
websocat -v ws://localhost:8080
```

**Check if SSH server is running**:
```bash
kubectl exec -it websocket-poc -- ps aux | grep sshd
```

---

## Cleanup

When you're done with the POC:

```bash
# Delete pod and service
kubectl delete -f websocket-poc-pod.yaml

# Delete ingress route
kubectl delete -f websocket-poc-ingress.yaml

# Remove DNS entry (if using /etc/hosts)
sudo nano /etc/hosts
# Remove the line: 203.0.113.100  poc.workspaces.example.com
```

---

## Comparison: POC vs Production

| Component | POC | Production |
|-----------|-----|------------|
| **Pod Creation** | Manual kubectl apply | Operator generates from WorkspaceAccessStrategy template |
| **Remote Server** | Standard OpenSSH | Custom remote access server (already exists) |
| **WebSocket Proxy** | websocat binary | Custom Go sidecar (~200 LOC) with auth, metrics, logging |
| **Networking** | Port-forward for testing | Load Balancer → Traefik → IngressRoute → Service → Pod |
| **Authentication** | Password (testpass) | JWT token validation + Workspace ACL checks |
| **Client** | websocat CLI + SSH | VS Code extension with WebSocket client |
| **URL Generation** | Hardcoded | Operator API returns dynamic URL with token |
| **DNS** | localhost or manual | Wildcard DNS (*.workspaces.example.com) |
| **TLS** | None or manual | cert-manager with Let's Encrypt |
| **Cleanup** | Manual deletion | Automatic via owner references |

**The POC proves the core concept** (WebSocket can tunnel TCP connections) **without building the full production system** (operator integration, auth, VS Code extension).
