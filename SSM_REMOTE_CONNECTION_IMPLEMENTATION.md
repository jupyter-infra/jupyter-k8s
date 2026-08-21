# SSM Remote Connection Implementation Details

**Document Version**: 1.0  
**Last Updated**: 2026-02-10  
**Purpose**: Comprehensive technical documentation of the AWS Systems Manager (SSM) based remote connection implementation for VSCode remote development in Kubernetes workspaces.

---

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Component Details](#component-details)
3. [Registration Flow](#registration-flow)
4. [Connection Flow](#connection-flow)
5. [Data Flow](#data-flow)
6. [Token and Credential Management](#token-and-credential-management)
7. [State Management](#state-management)
8. [Cleanup and Lifecycle](#cleanup-and-lifecycle)
9. [Configuration](#configuration)
10. [Performance Characteristics](#performance-characteristics)
11. [Failure Modes and Recovery](#failure-modes-and-recovery)
12. [Security Model](#security-model)

---

## Architecture Overview

### High-Level Design

The SSM remote connection architecture creates a **tunneling system** where AWS SSM Service acts as a **relay/proxy** between the user's VSCode client and the workspace pod. The critical design principle is that **no direct network connection** exists between user and pod - all traffic routes through AWS SSM Service.

```
┌─────────────────┐         ┌─────────────────┐         ┌─────────────────┐
│  VSCode Client  │ ◄─WS──► │  AWS SSM        │ ◄─WS──► │  SSM Agent      │
│  (User Machine) │         │  Service        │         │  (Pod Sidecar)  │
└─────────────────┘         └─────────────────┘         └─────────────────┘
                                                                  ↓ TCP
                                                         ┌─────────────────┐
                                                         │ Remote Access   │
                                                         │ Server :2222    │
                                                         └─────────────────┘
```

### Key Components

1. **AWS SSM Service** (External to Kubernetes)
   - Managed AWS service acting as session broker and relay
   - Maintains registry of "managed instances" (our pods)
   - Creates secure tunneling sessions between clients and instances

2. **SSM Agent Sidecar** (In Pod)
   - AWS-provided daemon running in sidecar container
   - Maintains persistent WebSocket connection to AWS SSM
   - Receives session commands and creates local port forwards

3. **Remote Access Server** (In Workspace Container)
   - SSH-like server listening on port 2222
   - Provides shell access for VSCode Remote-SSH extension
   - Runs in main workspace container

4. **Operator/Controller** (Kubernetes)
   - Orchestrates SSM registration process
   - Creates activations via AWS API
   - Triggers registration scripts via kubectl exec
   - Manages cleanup on pod deletion

5. **Extension API** (Kubernetes)
   - Handles connection requests from users
   - Finds SSM managed instances
   - Starts SSM sessions
   - Returns VSCode connection URLs

### Network Architecture

**Critical Design Point**: No ingress to pod required. All connections tunnel out.

```
Pod Network Requirements:
  ✅ Outbound to ssmmessages.{region}.amazonaws.com:443 (WebSocket)
  ✅ Outbound to ssm.{region}.amazonaws.com:443 (API calls)
  ❌ No inbound connections required
  ❌ No LoadBalancer needed
  ❌ No Ingress needed
```

This design works in locked-down enterprise environments where ingress is prohibited.

---

## Component Details

### 1. AWS SSM Service

**Service Endpoints**:
- `ssm.{region}.amazonaws.com` - Control plane API (CreateActivation, StartSession, etc.)
- `ssmmessages.{region}.amazonaws.com` - Data plane WebSocket (agent connections, session data)

**Hybrid Activation System**:
- Allows non-EC2 instances (like Kubernetes pods) to register as "managed instances"
- Originally designed for on-premise servers, adapted for containers
- Each activation is single-use with configurable expiration (we use 5 minutes)

**Session Manager**:
- Creates secure tunneling sessions between clients and managed instances
- Supports port forwarding via SSM documents
- Maintains session state and routing tables
- Enforces concurrent session limits (10 per instance)

**Managed Instance Registry**:
- Stores metadata about registered instances
- Tracks instance status (Online, Offline, Connection Lost)
- Maintains tags for instance lookup
- Records last ping time (agents ping every 15 seconds)

### 2. SSM Agent Sidecar Container

**Container Image**: `amazonlinux:2023` with `amazon-ssm-agent` package

**Binary Location**: `/usr/bin/amazon-ssm-agent`

**Configuration**:
- Config file: `/etc/amazon/ssm/amazon-ssm-agent.json`
- Credentials: `/var/lib/amazon/ssm/Vault/Store/RegistrationKey`
- Logs: `/var/log/amazon/ssm/amazon-ssm-agent.log`

**Lifecycle**:
1. Container starts, runs `sleep infinity` (waits for operator)
2. Operator triggers registration via kubectl exec
3. Agent registers with AWS SSM, gets instance ID
4. Agent starts daemon with persistent WebSocket to AWS
5. Agent polls for commands every 15 seconds
6. On session start, agent creates port forward to localhost:2222

**Health Checks**:
- Startup probe: Checks if agent process running (`pgrep amazon-ssm-agent`)
- Liveness probe: Same as startup
- Readiness probe: Checks for marker file `/tmp/ssm-registered`

**Resource Requirements**:
```yaml
resources:
  requests:
    cpu: "100m"
    memory: "1Gi"
  limits:
    cpu: "500m"
    memory: "1Gi"
```

### 3. Remote Access Server

**Binary**: `/opt/amazon/sagemaker/workspace/remote-access/remote-access-server`

**Port**: 2222 (non-privileged, can run as non-root)

**Protocol**: SSH-like (compatible with VSCode Remote-SSH)

**Startup**:
- Triggered by operator via kubectl exec in workspace container
- Script: `/opt/amazon/sagemaker/workspace/remote-access/start-remote-access-server.sh`
- Attempts supervisor integration first, falls back to direct execution
- Logs to: `/var/log/studio/remoteAccess/start-remote-access-server.log`

**Features**:
- Shell access for remote development
- File system access
- Port forwarding capabilities
- Process management

### 4. Shared Volume Pattern

**Volume**: `workspace-base` (emptyDir, 1Gi size limit)

**Mount Points**:
- Init container: `/workspace_base_dir` (write-only, copies scripts)
- SSM sidecar: `/opt/amazon/sagemaker/workspace` (read-write)
- Workspace container: `/opt/amazon/sagemaker/workspace` (read-write)

**Contents**:
```
/opt/amazon/sagemaker/workspace/
├── bin/
│   ├── entrypoint-workspace-init-container
│   ├── entrypoint-workspace-sidecar-container
│   └── entrypoint-workspace-{jupyterlab,code-editor}
├── remote-access/
│   ├── remote-access-server (binary)
│   └── start-remote-access-server.sh
└── .ssm-registration-state.json (state file)
```

**Purpose**:
- Share scripts between containers
- Persist state across container restarts
- Enable operator to execute scripts via kubectl exec

---

## Registration Flow

### Overview

Registration is the process of making a Kubernetes pod known to AWS SSM as a "managed instance". This is a **one-time setup** per pod (unless container restarts).

### Trigger

**Event**: Pod enters `Running` phase with all containers ready

**Detection**: Pod event handler watches pod events, filters for workspace pods

**Code Path**:
```
PodEventHandler.HandleWorkspacePodEvents()
  → handlePodRunning()
  → Check accessStrategy.Spec.PodEventsHandler == "aws"
  → ssmRemoteAccessStrategy.SetupContainers()
```

### Step-by-Step Process

#### Step 1: Create SSM Activation

**Actor**: Operator (Extension API or Controller)

**AWS API Call**:
```go
ssm.CreateActivation(
  Description: "Activation for {namespace}/{workspace} (pod: {pod-uid})",
  IamRole: "workspace-ssm-managed-node-role",  // From accessStrategy
  RegistrationLimit: 1,                         // Single-use
  ExpirationDate: time.Now().Add(5 * time.Minute),
  DefaultInstanceName: "workspace-{pod-uid}",
  Tags: [
    { Key: "managed-by", Value: "jupyter-k8s-operator" },
    { Key: "workspace-name", Value: "{workspace-name}" },
    { Key: "namespace", Value: "{namespace}" },
    { Key: "workspace-pod-uid", Value: "{pod-uid}" },
    { Key: "sagemaker.amazonaws.com/managed-by", Value: "amazon-sagemaker-spaces" },
    { Key: "sagemaker.amazonaws.com/eks-cluster-arn", Value: "{cluster-arn}" }
  ]
)
```

**Returns**:
```go
{
  ActivationId: "abc-123-def-456",      // Public identifier
  ActivationCode: "xyz789secretcode"    // Secret credential (like password)
}
```

**Why Needed**: SSM requires proof of authorization before allowing registration. Activation is a "one-time registration token" that proves the operator authorized this pod.

**Security Notes**:
- Activation code is secret, treated like password
- Single-use: can only register one instance
- Short-lived: expires in 5 minutes
- Passed via stdin to avoid command-line exposure

#### Step 2: Execute Registration Script

**Actor**: Operator

**Method**: `kubectl exec` into sidecar container

**Command**:
```bash
kubectl exec -n {namespace} {pod-name} -c ssm-agent-sidecar -- bash -c "
  read ACTIVATION_ID && read ACTIVATION_CODE && \
  env ACTIVATION_ID=\"\$ACTIVATION_ID\" \
      ACTIVATION_CODE=\"\$ACTIVATION_CODE\" \
      REGION={region} \
  /usr/local/bin/register-ssm.sh
"
# Credentials passed via stdin (not command line)
echo -e "{activation-id}\n{activation-code}" | kubectl exec ...
```

**Why kubectl exec**: Operator has AWS credentials, pod doesn't. Operator creates activation, passes credentials securely to pod.

#### Step 3: SSM Agent Registration

**Script**: `/usr/local/bin/register-ssm.sh`

**Process**:

1. **Validate Environment**
   - Check ACTIVATION_ID, ACTIVATION_CODE, REGION env vars present
   - Validate region format (e.g., us-west-2)
   - Check amazon-ssm-agent binary exists
   - Check if agent already running (skip if yes)

2. **Configure SSM Agent**
   ```bash
   cp /etc/amazon/ssm/amazon-ssm-agent.json.template \
      /etc/amazon/ssm/amazon-ssm-agent.json
   
   sed -i "s|\"SessionHandshakeTimeoutSeconds.*|\"SessionHandshakeTimeoutSeconds\" : 60,|" \
      /etc/amazon/ssm/amazon-ssm-agent.json
   ```

3. **Register with AWS SSM**
   ```bash
   amazon-ssm-agent -register \
     -id $ACTIVATION_ID \
     -code $ACTIVATION_CODE \
     -region $REGION
   ```
   
   **What happens internally**:
   - Agent contacts `ssm.{region}.amazonaws.com/RegisterManagedInstance`
   - Sends activation credentials for validation
   - SSM validates: not expired, not already used, IAM role valid
   - SSM creates managed instance record with ID: `mi-{random}`
   - SSM returns instance credentials (asymmetric key pair)
   - Agent stores credentials in `/var/lib/amazon/ssm/Vault/Store/RegistrationKey`
   - Agent extracts instance ID from output, writes to `/tmp/ssm-instance-id`

4. **Start SSM Agent Daemon**
   ```bash
   nohup amazon-ssm-agent >> /var/log/ssm-registration.log 2>&1 &
   echo $! > /var/run/ssm-agent.pid
   ```
   
   **Agent startup sequence**:
   - Loads credentials from vault
   - Establishes WebSocket to `wss://ssmmessages.{region}.amazonaws.com/v1/control-channel`
   - Authenticates using instance credentials
   - Begins polling for commands every 15 seconds
   - Sends heartbeat pings to maintain connection

5. **Create Marker File** (backward compatibility)
   ```bash
   touch /tmp/ssm-registered
   ```

**Logs**: All output written to `/var/log/ssm-registration.log`

**Success Indicators**:
- Script exits with code 0
- Instance ID written to `/tmp/ssm-instance-id`
- Agent process running (visible in `ps aux`)
- Marker file exists

#### Step 4: Start Remote Access Server

**Actor**: Operator

**Method**: `kubectl exec` into workspace container

**Command**:
```bash
kubectl exec -n {namespace} {pod-name} -c workspace -- \
  /opt/amazon/sagemaker/workspace/remote-access/start-remote-access-server.sh \
  --port 2222
```

**Script Logic**:
1. Check if server already running (skip if yes)
2. Try supervisor integration:
   - Create supervisor config for remote-access-server
   - Add include to JupyterLab/Code Editor supervisor configs
   - Reload supervisor
3. Fallback to direct execution:
   ```bash
   nohup /opt/amazon/sagemaker/workspace/remote-access/remote-access-server \
     -port 2222 > /dev/null 2>&1 &
   ```

**Why Supervisor**: Provides auto-restart, log management, graceful shutdown

#### Step 5: Update State File

**Location**: `/opt/amazon/sagemaker/workspace/.ssm-registration-state.json`

**Content**:
```json
{
  "sidecarRestartCount": 0,
  "workspaceRestartCount": 0,
  "setupInProgress": false,
  "setupStartedAt": "2026-02-10T11:00:00Z"
}
```

**Purpose**: Track container restart counts to detect when re-registration needed

### Registration State Machine

```
┌─────────────────────────────────────────────────────────────┐
│ Initial State: No state file exists                         │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ Action: Full setup                                          │
│  - Register SSM agent                                       │
│  - Start remote access server                               │
│  - Write state file with current restart counts            │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ Steady State: State file exists, counts match               │
│  - No action needed                                         │
└─────────────────────────────────────────────────────────────┘
                          ↓ (sidecar restarts)
┌─────────────────────────────────────────────────────────────┐
│ Detected: sidecarRestartCount < current count               │
│ Action: Re-register SSM agent                               │
│  - Cleanup old managed instance                             │
│  - Create new activation                                    │
│  - Register agent                                           │
│  - Update state file                                        │
└─────────────────────────────────────────────────────────────┘
                          ↓ (workspace restarts)
┌─────────────────────────────────────────────────────────────┐
│ Detected: workspaceRestartCount < current count             │
│ Action: Restart remote access server only                   │
│  - No SSM re-registration needed                            │
│  - Start server                                             │
│  - Update state file                                        │
└─────────────────────────────────────────────────────────────┘
```

### Concurrency Protection

**Problem**: Multiple pod events can trigger setup simultaneously

**Solution**: Random delay + setup-in-progress flag

```go
// Random delay 0-2 seconds to spread out concurrent events
delay := time.Duration(rand.Intn(2000)) * time.Millisecond
time.Sleep(delay)

// Check if setup already in progress
if state.SetupInProgress {
  return nil  // Another event is handling it
}

// Mark as in progress
state.SetupInProgress = true
writeState(state)

// Perform setup...

// Mark as complete
state.SetupInProgress = false
writeState(state)
```

**Note**: This is best-effort. For stronger guarantees, distributed mutex would be needed.

---

## Connection Flow

### Overview

Connection flow is triggered when a user requests a VSCode remote connection. The operator finds the SSM managed instance, starts a session, and returns a connection URL.

### User Action

**UI**: User clicks "Connect with VSCode" button

**API Request**:
```http
POST /apis/connection.workspace.jupyter.org/v1alpha1/namespaces/default/workspaceconnections
Content-Type: application/json

{
  "apiVersion": "connection.workspace.jupyter.org/v1alpha1",
  "kind": "WorkspaceConnection",
  "metadata": {
    "name": "my-workspace-connection"
  },
  "spec": {
    "workspaceName": "my-workspace",
    "workspaceConnectionType": "vscode-remote"
  }
}
```

### Step-by-Step Process

#### Step 1: Authorization Check

**Actor**: Extension API

**Process**:
1. Extract user from request headers (set by auth middleware)
2. Get workspace object from Kubernetes
3. Check if workspace is private (accessType: Private)
4. If private, perform SubjectAccessReview:
   ```go
   SubjectAccessReview{
     User: "{user}",
     Verb: "connect",
     Resource: "workspaces",
     Name: "{workspace-name}",
     Namespace: "{namespace}"
   }
   ```
5. Return 403 if not authorized

#### Step 2: Validate Connection Readiness

**Checks**:
1. Workspace has `Available` condition set to `True`
2. AccessStrategy exists and has SSM configured:
   ```yaml
   spec:
     createConnectionContext:
       ssmDocumentName: "AWS-StartSSHSession"
       ssmManagedNodeRole: "workspace-ssm-managed-node-role"
   ```
3. CLUSTER_ID environment variable set in operator

**Error Responses**:
- 400 if workspace not available
- 400 if SSM not configured
- 500 if CLUSTER_ID not set

#### Step 3: Find SSM Managed Instance

**Actor**: Extension API

**AWS API Call**:
```go
ssm.DescribeInstanceInformation(
  Filters: [{
    Key: "tag:workspace-pod-uid",
    Values: ["{pod-uid}"]
  }]
)
```

**Response**:
```go
{
  InstanceInformationList: [{
    InstanceId: "mi-abc123def456",
    PingStatus: "Online",
    LastPingDateTime: "2026-02-10T11:00:00Z",
    PlatformType: "Linux",
    PlatformName: "Amazon Linux",
    IPAddress: "",  // Not applicable for hybrid instances
    ComputerName: "workspace-{pod-uid}",
    RegistrationDate: "2026-02-10T10:00:00Z"
  }]
}
```

**Error Handling**:
- If no instances found: 404 "No managed instance found"
- If multiple instances found: Log warning, select most recent by RegistrationDate
- If instance offline: 503 "Instance not available"

**Why This Works**: Tags from activation are attached to managed instance, enabling lookup by pod UID.

#### Step 4: Check Session Limits

**Actor**: Extension API

**AWS API Call**:
```go
ssm.DescribeSessions(
  State: "Active",
  Filters: [{
    Key: "Target",
    Value: "mi-abc123def456"
  }],
  MaxResults: 10
)
```

**Limit**: Maximum 10 concurrent sessions per managed instance

**Error**: If 10 sessions already active, return 429 "Too many active sessions"

#### Step 5: Start SSM Session

**Actor**: Extension API

**AWS API Call**:
```go
ssm.StartSession(
  Target: "mi-abc123def456",
  DocumentName: "AWS-StartSSHSession",  // Or custom document
  Parameters: {
    "portNumber": ["2222"]
  }
)
```

**SSM Document**: Defines session behavior

```json
{
  "schemaVersion": "1.0",
  "description": "SSM document for remote access",
  "sessionType": "Port",
  "parameters": {
    "portNumber": {
      "type": "String",
      "default": "22"
    }
  },
  "inputs": {
    "idleSessionTimeout": 60,      // Minutes
    "maxSessionDuration": 720      // Minutes (12 hours)
  },
  "properties": {
    "portNumber": "{{ portNumber }}"
  }
}
```

**Response**:
```go
{
  SessionId: "session-xyz-789-abc",
  TokenValue: "AQECAHi...very-long-token...==",
  StreamUrl: "wss://ssmmessages.us-west-2.amazonaws.com/v1/data-channel/session-xyz-789-abc?stream"
}
```

**What SSM Does**:
1. Creates session record in database
2. Generates session token (JWT-like, ~1 hour expiry)
3. Prepares to relay data between client and agent
4. Sends command to agent via existing WebSocket:
   ```json
   {
     "MessageType": "start_session",
     "SessionId": "session-xyz-789-abc",
     "DocumentName": "AWS-StartSSHSession",
     "Parameters": { "portNumber": "2222" }
   }
   ```

**Agent Response**:
- Agent receives command over control channel WebSocket
- Agent creates port forward: WebSocket ↔ TCP localhost:2222
- Agent sends acknowledgment to SSM

#### Step 6: Generate VSCode URL

**Actor**: Extension API

**URL Format**:
```
vscode://amazonwebservices.aws-toolkit-vscode/connect/workspace
  ?sessionId=session-xyz-789-abc
  &sessionToken=AQECAHi...token...==
  &streamUrl=wss://ssmmessages.us-west-2.amazonaws.com/v1/data-channel/session-xyz-789-abc?stream
  &workspaceName=my-workspace
  &namespace=default
  &eksClusterArn=arn:aws:eks:us-west-2:123456789:cluster/my-cluster
```

**URL Components**:
- `vscode://` - VSCode URL scheme (opens VSCode)
- `amazonwebservices.aws-toolkit-vscode` - AWS Toolkit extension ID
- `/connect/workspace` - Extension command path
- Query parameters:
  - `sessionId` - SSM session identifier
  - `sessionToken` - Authentication token for WebSocket
  - `streamUrl` - WebSocket endpoint for data channel
  - `workspaceName`, `namespace`, `eksClusterArn` - Metadata

**API Response**:
```json
{
  "apiVersion": "connection.workspace.jupyter.org/v1alpha1",
  "kind": "WorkspaceConnection",
  "metadata": {
    "name": "my-workspace-connection"
  },
  "spec": {
    "workspaceName": "my-workspace",
    "workspaceConnectionType": "vscode-remote"
  },
  "status": {
    "workspaceConnectionType": "vscode-remote",
    "workspaceConnectionUrl": "vscode://amazonwebservices.aws-toolkit-vscode/..."
  }
}
```

#### Step 7: VSCode Connection Establishment

**Actor**: User's VSCode application

**Process**:

1. **URL Handler**: VSCode receives `vscode://` URL, routes to AWS Toolkit extension

2. **Parse Parameters**: Extension extracts sessionId, sessionToken, streamUrl

3. **WebSocket Connection**: Extension opens WebSocket to streamUrl
   ```javascript
   const ws = new WebSocket(streamUrl);
   ws.binaryType = 'arraybuffer';
   
   // Add session token to headers or URL params for auth
   ```

4. **SSM Authentication**: SSM validates sessionToken
   - Checks token signature
   - Checks token not expired
   - Checks session exists and is active
   - Returns 403 if invalid

5. **SSH Handshake**: VSCode sends SSH protocol handshake over WebSocket
   ```
   VSCode → WebSocket binary frame: SSH-2.0-OpenSSH_8.0
   ```

6. **SSM Relay**: SSM forwards frame to agent's WebSocket

7. **Agent Port Forward**: Agent writes data to TCP socket localhost:2222

8. **Remote Access Server**: Server receives SSH handshake, responds
   ```
   Server → TCP: SSH-2.0-RemoteAccessServer_1.0
   ```

9. **Reverse Path**: Response flows back through agent → SSM → VSCode

10. **SSH Authentication**: VSCode and server complete SSH key exchange

11. **Session Established**: VSCode Remote-SSH extension connects, user has shell access

### Connection Sequence Diagram

```
User          VSCode        Extension API    AWS SSM       SSM Agent     Remote Server
 │               │                │              │              │              │
 │ Click Connect │                │              │              │              │
 ├──────────────>│                │              │              │              │
 │               │ POST /connect  │              │              │              │
 │               ├───────────────>│              │              │              │
 │               │                │ FindInstance │              │              │
 │               │                ├─────────────>│              │              │
 │               │                │<─────────────┤              │              │
 │               │                │ StartSession │              │              │
 │               │                ├─────────────>│              │              │
 │               │                │              │ StartPortFwd │              │
 │               │                │              ├─────────────>│              │
 │               │                │              │<─────────────┤              │
 │               │                │<─────────────┤              │              │
 │               │<───────────────┤              │              │              │
 │               │ (VSCode URL)   │              │              │              │
 │<──────────────┤                │              │              │              │
 │               │                │              │              │              │
 │ Open URL      │                │              │              │              │
 ├──────────────>│                │              │              │              │
 │               │ WebSocket      │              │              │              │
 │               ├───────────────────────────────>│              │              │
 │               │                │              │ SSH Data     │              │
 │               │                │              ├─────────────>│              │
 │               │                │              │              │ TCP :2222    │
 │               │                │              │              ├─────────────>│
 │               │                │              │              │<─────────────┤
 │               │                │              │<─────────────┤              │
 │               │<───────────────────────────────┤              │              │
 │               │                │              │              │              │
 │ Shell Access  │                │              │              │              │
 │<═════════════════════════════════════════════════════════════════════════>│
```

---

## Data Flow

### Detailed Packet Flow

#### Outbound (User → Workspace)

```
1. User types command in VSCode terminal: ls -la

2. VSCode Remote-SSH extension:
   - Wraps command in SSH protocol
   - Serializes to bytes: [0x6c, 0x73, 0x20, 0x2d, 0x6c, 0x61, 0x0a]

3. AWS Toolkit extension:
   - Wraps bytes in WebSocket binary frame
   - Sends over WebSocket connection

4. AWS SSM Service:
   - Receives WebSocket frame on data channel
   - Looks up session: session-xyz → mi-abc123
   - Forwards frame to agent's WebSocket (control channel)

5. SSM Agent in pod:
   - Receives frame on control channel WebSocket
   - Extracts binary data from frame
   - Writes bytes to TCP socket: localhost:2222

6. Remote Access Server:
   - Reads bytes from TCP socket
   - Parses as SSH protocol
   - Executes command: ls -la
   - Generates output
```

#### Inbound (Workspace → User)

```
1. Remote Access Server:
   - Command output: "total 48\ndrwxr-xr-x..."
   - Wraps in SSH protocol
   - Writes to TCP socket

2. SSM Agent:
   - Reads bytes from TCP socket
   - Wraps in WebSocket binary frame
   - Sends to AWS SSM on control channel

3. AWS SSM Service:
   - Receives frame from agent
   - Looks up session routing
   - Forwards to client's data channel WebSocket

4. AWS Toolkit extension:
   - Receives WebSocket frame
   - Extracts binary data

5. VSCode Remote-SSH:
   - Parses SSH protocol
   - Displays output in terminal
```

### Frame Structure

**WebSocket Binary Frame**:
```
┌────────────────────────────────────────────────────────┐
│ WebSocket Header (2-14 bytes)                          │
│  - FIN bit: 1 (final frame)                            │
│  - Opcode: 0x2 (binary)                                │
│  - Mask bit: 1 (client to server)                      │
│  - Payload length: variable                            │
├────────────────────────────────────────────────────────┤
│ Masking Key (4 bytes, if masked)                       │
├────────────────────────────────────────────────────────┤
│ Payload Data (SSH protocol bytes)                      │
│  - SSH packet header                                   │
│  - SSH packet payload                                  │
│  - SSH packet MAC                                      │
└────────────────────────────────────────────────────────┘
```

**SSH Protocol Inside**:
```
┌────────────────────────────────────────────────────────┐
│ SSH Packet                                             │
│  - Packet Length (4 bytes)                             │
│  - Padding Length (1 byte)                             │
│  - Payload (variable)                                  │
│    - Message Type (1 byte)                             │
│    - Message Data (variable)                           │
│  - Padding (variable)                                  │
│  - MAC (variable, if encryption enabled)               │
└────────────────────────────────────────────────────────┘
```

### Bandwidth and Latency

**Typical Command Execution**:
```
User types: ls -la
  ↓ ~10 bytes SSH payload
  ↓ ~20 bytes WebSocket overhead
  ↓ ~30 bytes total

Round trip time:
  User → AWS SSM: ~20-50ms (internet latency)
  AWS SSM → Pod: ~10-30ms (AWS internal network)
  Processing: ~1-5ms
  Return path: ~30-80ms
  
Total: ~60-165ms (noticeable but acceptable)
```

**Large File Transfer** (e.g., 1MB file):
```
1MB file = 1,048,576 bytes
WebSocket frame size: ~1KB default
Frames needed: ~1024

Transfer time:
  Bandwidth: ~10-100 Mbps (varies by connection)
  Latency per frame: ~60-165ms
  Total: ~10-60 seconds (slower than direct connection)
```

### Connection Persistence

**WebSocket Keep-Alive**:
- SSM agent sends ping frames every 15 seconds
- AWS SSM responds with pong frames
- If no pong received, agent attempts reconnect
- If reconnect fails, instance marked "Connection Lost"

**Session Timeout**:
- Idle timeout: 60 minutes (configurable in SSM document)
- Max duration: 720 minutes (12 hours)
- After timeout, session terminated, WebSocket closed

**Reconnection Behavior**:
- If agent WebSocket drops, agent reconnects automatically
- Existing sessions may fail, require new StartSession call
- VSCode detects connection loss, prompts user to reconnect

---

## Token and Credential Management

### Activation Credentials

**Purpose**: Authorize pod to register with AWS SSM

**Lifecycle**:
```
1. Operator creates activation via AWS API
   ↓
2. AWS returns ActivationId + ActivationCode
   ↓
3. Operator passes to pod via kubectl exec stdin
   ↓
4. Pod uses credentials to register
   ↓
5. AWS validates, creates managed instance
   ↓
6. Activation marked as "used", cannot be reused
   ↓
7. Activation expires after 5 minutes (unused or not)
```

**Security Properties**:
- **Single-use**: Each activation can only register one instance
- **Short-lived**: 5-minute expiration window
- **Secret**: ActivationCode treated like password
- **Scoped**: Tied to specific IAM role

**Storage**:
- Operator: In-memory only, never persisted
- Pod: Passed via stdin, not visible in process list
- AWS: Stored encrypted, marked as used after registration

### Instance Credentials

**Purpose**: Authenticate agent to AWS SSM after registration

**Format**: Asymmetric key pair (similar to SSH keys)

**Location**: `/var/lib/amazon/ssm/Vault/Store/RegistrationKey`

**Lifecycle**:
```
1. Agent registers with activation credentials
   ↓
2. AWS generates instance key pair
   ↓
3. AWS returns private key to agent
   ↓
4. Agent stores in encrypted vault
   ↓
5. Agent uses key to authenticate WebSocket connections
   ↓
6. Key valid until instance deregistered
```

**Security Properties**:
- **Long-lived**: Valid for instance lifetime
- **Asymmetric**: Public key stored in AWS, private key in pod
- **Encrypted**: Stored in SSM agent vault
- **Non-exportable**: Cannot be extracted from pod

### Session Token

**Purpose**: Authenticate client WebSocket connection to AWS SSM

**Format**: JWT-like token with signature

**Lifecycle**:
```
1. Operator calls StartSession API
   ↓
2. AWS generates session token
   ↓
3. Token returned in API response
   ↓
4. Operator includes in VSCode URL
   ↓
5. VSCode sends token in WebSocket connection
   ↓
6. AWS validates token signature and expiry
   ↓
7. Token expires after ~1 hour or session ends
```

**Security Properties**:
- **Short-lived**: ~1 hour expiration
- **Session-scoped**: Only valid for specific session
- **Signed**: Cannot be forged without AWS signing key
- **Revocable**: Session can be terminated, invalidating token

**Token Structure** (conceptual):
```json
{
  "header": {
    "alg": "HS256",
    "typ": "JWT"
  },
  "payload": {
    "sessionId": "session-xyz-789",
    "instanceId": "mi-abc123",
    "exp": 1707598800,
    "iat": 1707595200
  },
  "signature": "..."
}
```

### IAM Role Assumption

**Operator Role**:
```yaml
Role: workspace-operator-role
Trust Policy:
  - Service: pods.eks.amazonaws.com
Permissions:
  - ssm:CreateActivation
  - ssm:AddTagsToResource
  - ssm:DescribeInstanceInformation
  - ssm:DeregisterManagedInstance (scoped to workspace tags)
  - ssm:StartSession
  - iam:PassRole (scoped to managed node role)
```

**Managed Node Role**:
```yaml
Role: workspace-ssm-managed-node-role
Trust Policy:
  - Service: ssm.amazonaws.com
  - AWS: arn:aws:iam::{account}:root
Permissions:
  - AmazonSSMManagedInstanceCore
```

**Role Flow**:
```
1. Operator pod assumes workspace-operator-role via EKS Pod Identity
   ↓
2. Operator creates activation, specifies managed node role
   ↓
3. SSM agent registers using activation
   ↓
4. AWS SSM assumes managed node role on behalf of agent
   ↓
5. Agent uses role credentials for SSM API calls
```

---

## State Management

### Registration State File

**Location**: `/opt/amazon/sagemaker/workspace/.ssm-registration-state.json`

**Why Shared Volume**: Survives container restarts (emptyDir persists across container restarts within same pod)

**Schema**:
```json
{
  "sidecarRestartCount": 0,
  "workspaceRestartCount": 0,
  "setupInProgress": false,
  "setupStartedAt": "2026-02-10T11:00:00Z"
}
```

**Fields**:
- `sidecarRestartCount`: Last known restart count of SSM sidecar
- `workspaceRestartCount`: Last known restart count of workspace container
- `setupInProgress`: Flag to prevent concurrent setup attempts
- `setupStartedAt`: Timestamp when setup began (for debugging)

**Read/Write Operations**:

**Read**:
```bash
# In sidecar container
cat /opt/amazon/sagemaker/workspace/.ssm-registration-state.json
```

**Write** (atomic):
```bash
# Write to temp file, then move (atomic operation)
echo '{"sidecarRestartCount":1,...}' > /opt/amazon/sagemaker/workspace/.ssm-registration-state.json.tmp
mv /opt/amazon/sagemaker/workspace/.ssm-registration-state.json.tmp \
   /opt/amazon/sagemaker/workspace/.ssm-registration-state.json
```

### Restart Detection Logic

**Get Current Restart Counts**:
```go
func getCurrentRestartCounts(pod *corev1.Pod) (sidecar, workspace int32) {
  for _, cs := range pod.Status.ContainerStatuses {
    if cs.Name == "ssm-agent-sidecar" {
      sidecar = cs.RestartCount
    }
    if cs.Name == "workspace" {
      workspace = cs.RestartCount
    }
  }
  return
}
```

**Compare with Stored State**:
```go
currentSidecar, currentWorkspace := getCurrentRestartCounts(pod)
state := readState(pod)

if state == nil {
  // First time setup
  needSidecarSetup = true
  needWorkspaceSetup = true
} else {
  needSidecarSetup = (currentSidecar > state.SidecarRestartCount)
  needWorkspaceSetup = (currentWorkspace > state.WorkspaceRestartCount)
}
```

**Selective Setup**:
```go
if needSidecarSetup {
  cleanupOldSSMInstance()  // Deregister old mi-xxx
  createNewActivation()
  registerSSMAgent()
}

if needWorkspaceSetup {
  startRemoteAccessServer()  // No SSM changes needed
}

// Update state with current counts
updateState(currentSidecar, currentWorkspace)
```

### State Corruption Handling

**Scenario**: State file exists but is corrupted (invalid JSON)

**Detection**:
```go
state, err := readState(pod)
if err != nil {
  // File exists but corrupted
  needCleanup = true
  needFullSetup = true
}
```

**Recovery**:
1. Cleanup old SSM resources (may be orphaned)
2. Perform full setup (register + start server)
3. Write new state file

---

## Cleanup and Lifecycle

### Pod Deletion Flow

**Trigger**: Pod deleted (user deletes workspace, or pod evicted)

**Detection**: Pod event handler watches for `DeletionTimestamp != nil`

**Process**:

#### Step 1: Find Managed Instance

```go
ssm.DescribeInstanceInformation(
  Filters: [{
    Key: "tag:workspace-pod-uid",
    Values: ["{pod-uid}"]
  }]
)
```

**Handling Multiple Instances**:
- If multiple found, log error (shouldn't happen)
- Deregister all found instances
- Continue with cleanup

#### Step 2: Deregister Managed Instance

```go
ssm.DeregisterManagedInstance(
  InstanceId: "mi-abc123"
)
```

**What This Does**:
- Removes instance from SSM registry
- Terminates any active sessions
- Agent loses connection to SSM
- Instance ID becomes invalid

**Error Handling**:
- If instance not found: Log warning, continue
- If API error: Log error, continue with activation cleanup
- Don't fail pod deletion on cleanup errors

#### Step 3: Delete Activation

```go
ssm.DescribeActivations(
  Filters: [{
    Key: "DefaultInstanceName",
    Values: ["workspace-{pod-uid}"]
  }]
)

for activation in activations {
  ssm.DeleteActivation(
    ActivationId: activation.ActivationId
  )
}
```

**Why Delete Activation**:
- Cleanup AWS resources
- Prevent activation limit exhaustion (1000 per account)
- Remove unused records

**Error Handling**:
- If activation not found: Already deleted, continue
- If API error: Log error, don't block pod deletion

### Cleanup Timing

**When Cleanup Happens**:
- ✅ Pod deleted (DeletionTimestamp set)
- ❌ NOT when workspace stopped (pod still exists)
- ❌ NOT when container restarts (pod still exists)

**Why Lazy Cleanup**:
- Stopped workspaces can be restarted quickly
- Avoid re-registration overhead
- SSM resources are cheap (no cost for stopped instances)

### Orphaned Resource Detection

**Scenario**: Operator crashes during cleanup, resources not deleted

**Detection**:
- Periodic scan for instances with old registration dates
- Check if corresponding pod still exists
- If pod gone, instance is orphaned

**Cleanup**:
```go
// Pseudo-code for orphan cleanup job
instances := ssm.DescribeInstanceInformation()
for instance in instances {
  podUID := instance.Tags["workspace-pod-uid"]
  pod := k8s.GetPod(podUID)
  if pod == nil && instance.RegistrationDate < 24h ago {
    ssm.DeregisterManagedInstance(instance.InstanceId)
  }
}
```

**Note**: Not currently implemented, manual cleanup required

---

## Configuration

### WorkspaceAccessStrategy CRD

**Example**:
```yaml
apiVersion: workspace.jupyter.org/v1alpha1
kind: WorkspaceAccessStrategy
metadata:
  name: hyperpod-access-strategy
spec:
  displayName: "HyperPod Access Strategy"
  
  # Connection handler
  createConnectionHandler: "aws"
  podEventsHandler: "aws"
  
  # SSM configuration
  createConnectionContext:
    ssmManagedNodeRole: "workspace-ssm-managed-node-role"
    ssmDocumentName: "AWS-StartSSHSession"
  
  # Pod modifications
  deploymentModifications:
    podModifications:
      additionalContainers:
        - name: ssm-agent-sidecar
          image: {registry}/ssm-sidecar:latest
          command:
            - /opt/amazon/sagemaker/workspace/bin/entrypoint-workspace-sidecar-container
          resources:
            requests:
              cpu: "100m"
              memory: "1Gi"
            limits:
              cpu: "500m"
              memory: "1Gi"
          volumeMounts:
            - name: workspace-base
              mountPath: /opt/amazon/sagemaker/workspace
          startupProbe:
            exec:
              command: ["/usr/local/bin/ssm-agent-health-check.sh"]
            initialDelaySeconds: 1
            periodSeconds: 1
            failureThreshold: 300
          livenessProbe:
            exec:
              command: ["/usr/local/bin/ssm-agent-health-check.sh"]
            periodSeconds: 10
            failureThreshold: 6
          readinessProbe:
            exec:
              command: ["test", "-f", "/tmp/ssm-registered"]
            initialDelaySeconds: 2
            periodSeconds: 2
      
      volumes:
        - name: workspace-base
          emptyDir:
            sizeLimit: "1Gi"
      
      initContainers:
        - name: workspace-init-container
          image: {registry}/ssm-sidecar:latest
          command:
            - /opt/amazon/sagemaker/workspace/bin/entrypoint-workspace-init-container
          volumeMounts:
            - name: workspace-base
              mountPath: /workspace_base_dir
      
      primaryContainerModifications:
        volumeMounts:
          - name: workspace-base
            mountPath: /opt/amazon/sagemaker/workspace
```

### Environment Variables

**Operator/Controller**:
- `CLUSTER_ID`: EKS cluster ARN (required for SSM)
- `AWS_REGION`: AWS region (auto-detected from EKS Pod Identity)

**SSM Sidecar** (set by registration script):
- `ACTIVATION_ID`: SSM activation ID (passed via stdin)
- `ACTIVATION_CODE`: SSM activation code (passed via stdin)
- `REGION`: AWS region for SSM agent

### SSM Document Configuration

**Default Document**: `AWS-StartSSHSession` (AWS-provided)

**Custom Document**: Can create custom document for different behavior

**Example Custom Document**:
```json
{
  "schemaVersion": "1.0",
  "description": "Custom SSH session with longer timeout",
  "sessionType": "Port",
  "parameters": {
    "portNumber": {
      "type": "String",
      "default": "2222"
    }
  },
  "inputs": {
    "idleSessionTimeout": 120,     // 2 hours
    "maxSessionDuration": 1440     // 24 hours
  },
  "properties": {
    "portNumber": "{{ portNumber }}"
  }
}
```

**Document Creation**:
```go
ssm.CreateDocument(
  Name: "SageMaker-SpaceSSHSessionDocument",
  DocumentType: "Session",
  Content: documentJSON,
  Tags: [...]
)
```

---

## Performance Characteristics

### Connection Establishment

**Timeline**:
```
User clicks "Connect"
  ↓ 0ms
Extension API receives request
  ↓ 50ms - Authorization check (K8s API)
  ↓ 100ms - Find instance (AWS SSM API)
  ↓ 200ms - Check session limits (AWS SSM API)
  ↓ 500ms - Start session (AWS SSM API)
  ↓ 600ms - Return URL to user
VSCode opens URL
  ↓ 800ms - WebSocket handshake
  ↓ 1500ms - SSH handshake
  ↓ 2000ms - Shell ready

Total: ~2-3 seconds
```

**Breakdown**:
- API calls: ~600ms (3 AWS API calls)
- WebSocket setup: ~700ms
- SSH handshake: ~500ms
- Network latency: ~200ms

**Comparison**:
- Direct SSH: ~500ms (no AWS hops)
- kubectl port-forward: ~1000ms (K8s API overhead)
- SSM: ~2000ms (AWS relay overhead)

### Runtime Latency

**Interactive Shell**:
```
User types character
  ↓ 20-50ms - User → AWS SSM
  ↓ 10-30ms - AWS SSM → Pod
  ↓ 1-5ms - Processing
  ↓ 10-30ms - Pod → AWS SSM
  ↓ 20-50ms - AWS SSM → User

Total: 60-165ms per keystroke
```

**Perceived Performance**:
- <100ms: Feels instant
- 100-200ms: Noticeable but acceptable
- >200ms: Feels laggy

**SSM typically**: 60-165ms (acceptable for most users)

### Throughput

**WebSocket Frame Size**: ~1KB default

**Theoretical Maximum**:
- Bandwidth: Limited by slowest link (usually user's internet)
- Typical: 10-100 Mbps
- Frame overhead: ~2% (WebSocket headers)

**Practical Limits**:
- Small files (<1MB): Works well
- Large files (>10MB): Noticeably slower than direct
- Streaming data: May experience buffering

**Recommendations**:
- Use SSM for interactive shell, editing
- Use S3/EFS for large file transfers
- Avoid streaming video over SSM

### Resource Usage

**SSM Agent Sidecar**:
- CPU: 50-200m (idle to active)
- Memory: 256-512Mi (steady state)
- Network: 1-10 Mbps (during active session)

**Per-Pod Overhead**:
- 1 sidecar container
- 1Gi shared volume
- Persistent WebSocket connection

**Cluster-Wide**:
- 100 workspaces = 100 sidecars
- Total: 5-20 CPU cores, 25-50Gi memory

### Scalability Limits

**AWS SSM Limits** (per account, per region):
- Activations: 1000 total
- Concurrent sessions: 10 per instance
- API rate limits: 100 TPS (CreateActivation, StartSession)

**Practical Limits**:
- ~500 concurrent workspaces (2 activations per workspace for restarts)
- ~5000 concurrent sessions (10 per workspace)
- API throttling at ~100 workspace creations/minute

**Mitigation**:
- Use multiple AWS accounts
- Implement activation cleanup
- Add exponential backoff for API calls

---

## Failure Modes and Recovery

### 1. SSM Agent Crashes

**Symptoms**:
- Liveness probe fails
- Existing sessions disconnect
- New connections fail

**Detection**:
```
Kubernetes liveness probe:
  exec: /usr/local/bin/ssm-agent-health-check.sh
  → Checks if amazon-ssm-agent process running
  → Fails if process not found
```

**Recovery**:
```
1. Kubernetes restarts sidecar container
   ↓
2. Operator detects restart (restart count increased)
   ↓
3. Operator reads state file
   ↓
4. Operator performs cleanup:
   - Deregister old managed instance
   - Delete old activation
   ↓
5. Operator creates new activation
   ↓
6. Operator triggers registration script
   ↓
7. Agent registers, gets new instance ID
   ↓
8. Agent starts, establishes WebSocket
   ↓
9. Pod ready for new connections
```

**Downtime**: ~30-60 seconds (registration + startup)

**User Impact**: Active sessions lost, must reconnect

### 2. AWS SSM Service Outage

**Symptoms**:
- All connections fail
- Agent shows "Connection Lost"
- StartSession API returns errors

**Detection**:
- Agent WebSocket connection fails
- API calls return 503 Service Unavailable
- AWS Health Dashboard shows outage

**Recovery**:
- **Automatic**: None (external dependency)
- **Manual**: Wait for AWS to restore service
- **Workaround**: None (no fallback)

**Mitigation**:
- Multi-region deployment (different region for SSM)
- Alternative connection method (WebSocket proxy)

**User Impact**: Complete loss of remote connection capability

### 3. Activation Expires Before Registration

**Symptoms**:
- Registration script fails
- Error: "Activation not found or expired"
- Pod stuck in non-ready state

**Root Causes**:
- Registration took >5 minutes
- Network issues delayed registration
- Operator crashed after creating activation

**Detection**:
```
Registration script logs:
  "Failed to register: InvalidActivation"
```

**Recovery**:
```
1. Operator detects registration failure
   ↓
2. Operator creates new activation
   ↓
3. Operator retries registration
   ↓
4. Success or fail after 3 attempts
```

**Prevention**:
- Increase activation expiry (not recommended, security risk)
- Faster registration process
- Retry logic with exponential backoff

### 4. Session Limit Reached

**Symptoms**:
- 11th connection attempt fails
- Error: "Too many active sessions"
- Existing sessions continue working

**Root Cause**:
- User has 10+ VSCode windows open
- Sessions not properly closed
- Idle sessions not timing out

**Detection**:
```go
ssm.DescribeSessions(State: "Active", Target: "mi-abc123")
→ Returns 10 sessions
→ StartSession fails with LimitExceeded
```

**Recovery**:
```
1. User closes existing VSCode windows
   ↓
2. Sessions terminate
   ↓
3. New connection succeeds
```

**Automatic Cleanup**:
- Idle timeout: 60 minutes (configurable)
- Max duration: 12 hours
- Sessions auto-terminate after timeout

**Mitigation**:
- Lower idle timeout (trade-off: user inconvenience)
- Increase limit (not possible, AWS hard limit)
- Monitor session count, warn users

### 5. Network Partition

**Scenario**: Pod loses connectivity to AWS SSM

**Symptoms**:
- Agent WebSocket disconnects
- Ping status changes to "Connection Lost"
- Existing sessions fail
- New connections fail

**Agent Behavior**:
```
1. WebSocket connection drops
   ↓
2. Agent detects disconnect
   ↓
3. Agent attempts reconnect with exponential backoff:
   - Attempt 1: Immediate
   - Attempt 2: 1 second
   - Attempt 3: 2 seconds
   - Attempt 4: 4 seconds
   - ...
   - Max backoff: 30 seconds
   ↓
4. If reconnect succeeds:
   - Agent resumes normal operation
   - Existing sessions may be lost
   ↓
5. If reconnect fails after 5 minutes:
   - Instance marked "Connection Lost" in SSM
```

**Recovery**:
- **Automatic**: Agent reconnects when network restored
- **Manual**: Restart pod if agent can't reconnect

**User Impact**: 
- Active sessions lost
- New connections fail until reconnected
- Downtime: Duration of network issue + reconnect time

### 6. Workspace Container Restarts

**Symptoms**:
- Remote access server stops
- Existing sessions disconnect
- New connections fail (can't reach port 2222)

**Detection**:
```go
currentWorkspaceRestarts > state.WorkspaceRestartCount
```

**Recovery**:
```
1. Operator detects workspace restart
   ↓
2. Operator executes start script in workspace
   ↓
3. Remote access server starts on port 2222
   ↓
4. Pod ready for new connections
```

**Note**: SSM agent unaffected (different container)

**Downtime**: ~5-10 seconds (server startup)

**User Impact**: Active sessions lost, must reconnect

### 7. Orphaned SSM Resources

**Scenario**: Operator crashes during cleanup, resources not deleted

**Symptoms**:
- Managed instances exist for deleted pods
- Activations accumulate
- Approaching AWS limits

**Detection**:
```go
// Manual check
instances := ssm.DescribeInstanceInformation()
for instance in instances {
  podUID := instance.Tags["workspace-pod-uid"]
  pod := k8s.GetPod(podUID)
  if pod == nil {
    // Orphaned instance
  }
}
```

**Recovery**:
```bash
# Manual cleanup script
for instance in $(aws ssm describe-instance-information \
  --filters "Key=tag:workspace-pod-uid,Values=*" \
  --query 'InstanceInformationList[*].InstanceId' \
  --output text); do
  
  pod_uid=$(aws ssm describe-instance-information \
    --instance-id $instance \
    --query 'InstanceInformationList[0].Tags[?Key==`workspace-pod-uid`].Value' \
    --output text)
  
  if ! kubectl get pod -A -o json | grep -q $pod_uid; then
    echo "Orphaned instance: $instance (pod: $pod_uid)"
    aws ssm deregister-managed-instance --instance-id $instance
  fi
done
```

**Prevention**:
- Implement periodic cleanup job
- Add finalizers to workspace CRD
- Improve operator error handling

---

## Security Model

### Threat Model

**Assets**:
- Workspace data (code, files, credentials)
- AWS credentials (operator role, managed node role)
- SSM activation codes
- Session tokens

**Threats**:
1. Unauthorized access to workspace
2. Credential theft
3. Session hijacking
4. Man-in-the-middle attacks
5. Privilege escalation

### Authentication Flow

**User → Extension API**:
```
1. User authenticated by OAuth/OIDC (Dex, Cognito, etc.)
   ↓
2. Auth middleware sets user header
   ↓
3. Extension API extracts user from header
   ↓
4. Extension API performs SubjectAccessReview (K8s RBAC)
   ↓
5. If authorized, proceed with connection
```

**VSCode → AWS SSM**:
```
1. Extension API generates session token via StartSession
   ↓
2. Token included in VSCode URL
   ↓
3. VSCode sends token in WebSocket connection
   ↓
4. AWS SSM validates token signature and expiry
   ↓
5. If valid, establish connection
```

**SSM Agent → AWS SSM**:
```
1. Agent uses instance credentials (from registration)
   ↓
2. Credentials include asymmetric key pair
   ↓
3. Agent signs requests with private key
   ↓
4. AWS SSM verifies signature with public key
   ↓
5. If valid, accept connection
```

### Encryption

**In Transit**:
- User → AWS SSM: TLS 1.2+ (WebSocket over HTTPS)
- AWS SSM → Agent: TLS 1.2+ (WebSocket over HTTPS)
- Agent → Server: Unencrypted (localhost TCP)

**At Rest**:
- Instance credentials: Encrypted in SSM agent vault
- Activation codes: Never persisted
- Session tokens: Not stored

**SSH Layer**:
- SSH protocol provides additional encryption
- Key exchange: Diffie-Hellman
- Cipher: AES-256-GCM (typical)
- MAC: HMAC-SHA2-256

### Authorization

**Workspace Access**:
```yaml
# Kubernetes RBAC
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: workspace-user
rules:
  - apiGroups: ["workspace.jupyter.org"]
    resources: ["workspaces"]
    verbs: ["connect"]  # Custom verb for connection
    resourceNames: ["my-workspace"]
```

**SSM Permissions**:
```yaml
# Operator role (via EKS Pod Identity)
- ssm:CreateActivation
- ssm:StartSession
- ssm:DescribeInstanceInformation
- ssm:DeregisterManagedInstance (scoped to workspace tags)

# Managed node role (assumed by SSM agent)
- ssm:UpdateInstanceInformation
- ssm:ListAssociations
- ssm:DescribeAssociation
- ssm:GetDocument
- ssm:DescribeDocument
- ssmmessages:CreateControlChannel
- ssmmessages:CreateDataChannel
- ssmmessages:OpenControlChannel
- ssmmessages:OpenDataChannel
```

### Audit Trail

**AWS CloudTrail**:
- CreateActivation: Who created activation, when, for which pod
- StartSession: Who started session, when, for which instance
- DeregisterManagedInstance: Who deregistered, when

**Kubernetes Audit Logs**:
- WorkspaceConnection creation: Who requested connection, when
- SubjectAccessReview: Authorization decisions

**SSM Session Logs** (optional):
- Session start/end times
- Commands executed (if session logging enabled)
- Data transferred

### Security Best Practices

**Activation Management**:
- ✅ Short expiry (5 minutes)
- ✅ Single-use
- ✅ Passed via stdin (not command line)
- ✅ Never logged or persisted

**Session Management**:
- ✅ Short-lived tokens (~1 hour)
- ✅ Session timeouts (idle: 60 min, max: 12 hours)
- ✅ Concurrent session limits (10 per instance)

**IAM Roles**:
- ✅ Least privilege (scoped permissions)
- ✅ Separate roles (operator vs managed node)
- ✅ Condition keys (tag-based access control)

**Network Security**:
- ✅ No ingress required (tunnels out)
- ✅ TLS encryption (in transit)
- ✅ Private subnets (pods don't need public IPs)

**Monitoring**:
- ✅ CloudTrail logging (API calls)
- ✅ Session logging (optional, for compliance)
- ✅ Metrics (session count, duration)

---

## Appendix

### Code References

**Key Files**:
- `internal/aws/ssm_client.go` - SSM API client
- `internal/aws/ssm_remote_access_strategy.go` - Registration and connection logic
- `internal/controller/pod_event_handler.go` - Pod event handling
- `internal/extensionapi/serverroute_connection.go` - Connection API endpoint
- `config/samples/workspace_access_strategy.yaml` - Configuration examples

**Sidecar Scripts**:
- `sidecar/register-ssm.sh` - SSM agent registration
- `sidecar/start-remote-access-server.sh` - Server startup
- `sidecar/ssm-agent-health-check.sh` - Health check

### AWS API References

**SSM APIs Used**:
- `CreateActivation` - Create registration token
- `DescribeInstanceInformation` - Find managed instances
- `StartSession` - Create session
- `DescribeSessions` - Check session limits
- `DeregisterManagedInstance` - Cleanup
- `DeleteActivation` - Cleanup

**Documentation**:
- SSM API: https://docs.aws.amazon.com/systems-manager/latest/APIReference/
- Session Manager: https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager.html
- Hybrid Activations: https://docs.aws.amazon.com/systems-manager/latest/userguide/sysman-managed-instance-activation.html

### Troubleshooting Guide

**Problem**: Registration fails with "InvalidActivation"
- **Cause**: Activation expired or already used
- **Solution**: Create new activation, retry registration

**Problem**: Connection fails with "Instance not found"
- **Cause**: Agent not registered or deregistered
- **Solution**: Check agent logs, re-register if needed

**Problem**: Connection fails with "Too many sessions"
- **Cause**: 10 concurrent sessions limit reached
- **Solution**: Close existing sessions, retry

**Problem**: High latency (>200ms)
- **Cause**: Network issues or AWS SSM congestion
- **Solution**: Check network, try different region

**Problem**: Agent shows "Connection Lost"
- **Cause**: Network partition or AWS SSM issue
- **Solution**: Check network, wait for reconnect

### Glossary

- **Activation**: One-time registration token for SSM agent
- **Managed Instance**: Pod registered with AWS SSM
- **Session**: Active connection between client and instance
- **Control Channel**: WebSocket for agent commands
- **Data Channel**: WebSocket for session data
- **Hybrid Instance**: Non-EC2 instance (like Kubernetes pod)
- **Port Forwarding**: Tunneling TCP traffic through WebSocket

---

**End of Document**
