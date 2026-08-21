# WebSocket Remote Connection Implementation Plan

**Document Version**: 1.0  
**Last Updated**: 2026-02-10  
**Purpose**: Comprehensive technical implementation plan for adding WebSocket-based remote connections alongside existing SSM implementation, maintaining dual support for both connection methods.

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Architecture Philosophy](#architecture-philosophy)
3. [Component Analysis](#component-analysis)
4. [WebSocket Proxy Implementation](#websocket-proxy-implementation)
5. [Connection Handler Architecture](#connection-handler-architecture)
6. [Ingress and Routing](#ingress-and-routing)
7. [JWT Token Management](#jwt-token-management)
8. [Controller Modifications](#controller-modifications)
9. [Configuration Schema](#configuration-schema)
10. [Testing Strategy](#testing-strategy)
11. [Deployment and Operations](#deployment-and-operations)
12. [Security Considerations](#security-considerations)
13. [Performance Analysis](#performance-analysis)
14. [Migration and Compatibility](#migration-and-compatibility)

---

## Executive Summary

### Objective

Add WebSocket-based remote connection capability to the jupyter-k8s operator **without removing or deprecating** the existing AWS SSM implementation. Both connection methods will coexist, allowing users to choose based on their infrastructure requirements.

### Key Principles

1. **Non-Breaking**: Existing SSM workspaces continue working unchanged
2. **Additive**: New code added alongside existing code, not replacing it
3. **Pluggable**: Connection handlers are swappable based on configuration
4. **Stateless**: WebSocket implementation has no registration or cleanup overhead
5. **Performant**: Lower latency than SSM due to direct routing

### High-Level Approach

```
┌─────────────────────────────────────────────────────────────────┐
│ Extension API (Unchanged Interface)                             │
│  POST /workspaceconnections                                     │
└─────────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────────┐
│ Connection Handler Factory (NEW)                                │
│  - Routes to SSM handler if createConnectionHandler: "aws"      │
│  - Routes to WebSocket handler if createConnectionHandler: "ws" │
└─────────────────────────────────────────────────────────────────┘
         ↓                                    ↓
┌──────────────────────┐          ┌──────────────────────────────┐
│ SSM Handler          │          │ WebSocket Handler (NEW)      │
│ (Existing)           │          │ - Generate JWT               │
│ - Find instance      │          │ - Build WSS URL              │
│ - Start session      │          │ - Return connection info     │
│ - Return VSCode URL  │          │                              │
└──────────────────────┘          └──────────────────────────────┘
```

### Success Criteria

- ✅ SSM workspaces continue functioning without changes
- ✅ New WebSocket workspaces can be created
- ✅ Connection establishment time <1 second for WebSocket
- ✅ Latency <100ms for WebSocket connections
- ✅ No additional AWS dependencies for WebSocket
- ✅ Works in non-AWS Kubernetes clusters (GKE, on-prem)

---

## Architecture Philosophy

### Design Decisions and Rationale

#### 1. Why Dual Support Instead of Migration?

**Decision**: Keep both SSM and WebSocket implementations active

**Rationale**:
- **Customer Choice**: Some customers require AWS SSM for compliance/audit
- **Risk Mitigation**: Gradual adoption reduces risk of breaking changes
- **Infrastructure Diversity**: SSM works in AWS-only, WebSocket works everywhere
- **Operational Safety**: Fallback option if one method has issues

**Implementation Impact**:
- Code complexity increases (two paths to maintain)
- Testing surface doubles (must test both methods)
- Documentation must cover both approaches
- Worth the trade-off for flexibility and safety

#### 2. Why Connection Handler Factory Pattern?

**Decision**: Use factory pattern to route to appropriate handler

**Rationale**:
- **Open/Closed Principle**: Open for extension (add new handlers), closed for modification (existing code unchanged)
- **Single Responsibility**: Each handler focuses on one connection method
- **Testability**: Handlers can be tested independently
- **Future-Proof**: Easy to add more connection methods (direct SSH, etc.)

**Alternative Considered**: If/else in existing code
- ❌ Violates single responsibility
- ❌ Makes existing code more complex
- ❌ Harder to test in isolation
- ❌ Difficult to add new methods

#### 3. Why Sidecar for WebSocket Proxy?

**Decision**: Run WebSocket proxy as sidecar container, not in main container

**Rationale**:
- **Separation of Concerns**: Proxy logic separate from workspace application
- **Lifecycle Independence**: Proxy can restart without affecting workspace
- **Resource Isolation**: Proxy has its own resource limits
- **Reusability**: Same proxy image works for all workspace types (JupyterLab, Code Editor, etc.)
- **Security**: Proxy runs with minimal privileges, workspace can run as any user

**Alternative Considered**: Embed proxy in workspace image
- ❌ Couples proxy to workspace image
- ❌ Requires rebuilding all workspace images
- ❌ Harder to update proxy independently
- ❌ Increases workspace image size

#### 4. Why JWT for Authentication?

**Decision**: Reuse existing JWT token system for WebSocket auth

**Rationale**:
- **Already Exists**: JWT system already implemented for web UI
- **Proven**: Battle-tested in production
- **Standard**: Industry-standard authentication method
- **Flexible**: Can add claims for fine-grained authorization
- **Stateless**: No session storage required

**Why Not Alternatives**:
- mTLS: Requires certificate management, more complex
- API Keys: Requires key storage and rotation
- OAuth2: Overkill for internal service-to-service auth

#### 5. Why Host-Based Routing?

**Decision**: Route by hostname (workspace-name.domain.com) not path

**Rationale**:
- **Isolation**: Each workspace gets unique subdomain
- **TLS**: Wildcard cert covers all workspaces (*.domain.com)
- **Simplicity**: No path rewriting needed
- **Compatibility**: Works with all HTTP clients
- **Scalability**: DNS-based load balancing possible

**Alternative Considered**: Path-based (domain.com/ws/namespace/workspace)
- ❌ Requires path rewriting in proxy
- ❌ More complex routing rules
- ❌ Harder to debug (path manipulation)
- ✅ Fewer DNS entries (only considered if DNS is bottleneck)

---

## Component Analysis

### Existing Components (Unchanged)

#### 1. WorkspaceAccessStrategy CRD

**Location**: `api/v1alpha1/workspaceaccessstrategy_types.go`

**Why Unchanged**: 
- CRD structure is generic enough to support both connection methods
- `createConnectionHandler` field already exists (currently only "aws")
- `createConnectionContext` is a map, can hold any key-value pairs
- `deploymentModifications` supports any container configuration

**What We Leverage**:
```go
type WorkspaceAccessStrategySpec struct {
    // This field determines which handler to use
    // Existing: "aws" → SSM handler
    // New: "websocket" → WebSocket handler
    CreateConnectionHandler string `json:"createConnectionHandler,omitempty"`
    
    // This map holds handler-specific configuration
    // SSM uses: ssmManagedNodeRole, ssmDocumentName
    // WebSocket uses: domain, proxyPort
    CreateConnectionContext map[string]string `json:"createConnectionContext,omitempty"`
    
    // This allows us to inject websocket-proxy sidecar
    DeploymentModifications *DeploymentModifications `json:"deploymentModifications,omitempty"`
}
```

**Why This Works**:
- No schema changes needed
- Backward compatible (existing SSM configs still valid)
- Forward compatible (can add more handlers in future)
- Type-safe (Go compiler enforces structure)

#### 2. Extension API Server

**Location**: `internal/extensionapi/server.go`

**Why Unchanged**:
- HTTP server setup remains the same
- Endpoint paths unchanged (`/workspaceconnections`)
- Request/response types unchanged (WorkspaceConnectionRequest/Response)
- Only internal routing logic changes (which handler to call)

**What We Leverage**:
```go
// Existing endpoint handler
func (s *ExtensionServer) HandleConnectionCreate(w http.ResponseWriter, r *http.Request) {
    // 1. Parse request (unchanged)
    // 2. Authorize user (unchanged)
    // 3. Validate workspace (unchanged)
    // 4. Generate connection URL (THIS CHANGES - route to handler)
    // 5. Return response (unchanged)
}
```

**Why This Works**:
- API contract preserved (clients don't need updates)
- Authorization logic reused (same RBAC checks)
- Error handling consistent (same error response format)
- Logging infrastructure reused

#### 3. JWT Token System

**Location**: `internal/jwt/`

**Why Unchanged**:
- Already generates signed JWT tokens
- Already supports KMS signing (optional)
- Already has token validation logic
- Already used for web UI authentication

**What We Leverage**:
```go
// Existing JWT signer interface
type Signer interface {
    GenerateToken(
        user string,
        groups []string,
        subject string,
        headers map[string][]string,
        path string,
        domain string,
        tokenType TokenType,
    ) (string, error)
}
```

**Why This Works**:
- Same token format works for WebSocket auth
- Same signing keys (no new key management)
- Same validation middleware (Traefik ForwardAuth)
- Same expiry logic (configurable TTL)

**What We Add**:
- New token type: `TokenTypeWebSocketConnection` (for audit trail)
- New claims: `workspace`, `namespace`, `connectionType`
- Same signature algorithm (HS256 or KMS)

#### 4. Traefik Ingress

**Location**: Existing Traefik deployment in cluster

**Why Unchanged**:
- Already handles HTTPS termination
- Already supports WebSocket upgrade
- Already has middleware system
- Already routes to services

**What We Leverage**:
- IngressRoute CRD (Traefik's custom routing)
- Middleware CRD (JWT validation)
- TLS certificate management
- Load balancing and health checks

**Why This Works**:
- Traefik natively supports WebSocket (no special config)
- Middleware can validate JWT before proxying
- Same TLS cert covers WebSocket (wss://)
- Same monitoring and logging

#### 5. Remote Access Server

**Location**: Workspace container, port 2222

**Why Unchanged**:
- Still provides SSH-like protocol
- Still listens on localhost:2222
- Still spawns shell for commands
- Still handles file operations

**What Changes**: Nothing in the server itself

**Why This Works**:
- Server doesn't care how traffic arrives (SSM tunnel vs WebSocket proxy)
- Same TCP protocol on localhost
- Same authentication mechanism (SSH keys)
- Same command execution

**Routing Difference**:
```
SSM:      VSCode → AWS SSM → SSM Agent → TCP:2222
WebSocket: VSCode → Traefik → WS Proxy → TCP:2222
```

Both end at the same TCP:2222 endpoint.

---

## WebSocket Proxy Implementation

### Overview

The WebSocket proxy is a **lightweight, stateless sidecar** that bridges WebSocket connections to TCP. It's the core new component that enables WebSocket-based remote connections.

### Design Requirements

1. **Lightweight**: <20MB image, <64Mi memory, <50m CPU at idle
2. **Stateless**: No persistent state, no registration, no cleanup
3. **Fast**: <10ms overhead for frame forwarding
4. **Reliable**: Automatic reconnection, graceful error handling
5. **Observable**: Prometheus metrics, structured logging
6. **Secure**: No privilege escalation, minimal attack surface

### Implementation Language: Go

**Why Go**:
- **Performance**: Compiled binary, low overhead, efficient goroutines
- **Concurrency**: Native goroutine support for bidirectional copying
- **Standard Library**: Excellent net/http and net packages
- **Small Binaries**: ~10-15MB with Alpine base
- **Ecosystem**: Gorilla WebSocket library (battle-tested)

**Why Not Alternatives**:
- Python: Slower, larger images (~100MB), higher memory
- Node.js: Larger images (~50MB), higher memory, callback complexity
- Rust: Steeper learning curve, longer compile times, overkill for this use case
- C/C++: Memory safety concerns, harder to maintain

### Core Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│ WebSocket Proxy (Sidecar Container)                            │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ HTTP Server (port 8080)                                  │  │
│  │  - /health → Health check endpoint                       │  │
│  │  - /metrics → Prometheus metrics                         │  │
│  │  - / → WebSocket upgrade handler                         │  │
│  └──────────────────────────────────────────────────────────┘  │
│                          ↓                                      │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ WebSocket Handler                                        │  │
│  │  1. Upgrade HTTP → WebSocket                             │  │
│  │  2. Connect to TCP target (localhost:2222)               │  │
│  │  3. Start bidirectional copy goroutines                  │  │
│  │  4. Handle errors and cleanup                            │  │
│  └──────────────────────────────────────────────────────────┘  │
│         ↓                                    ↓                  │
│  ┌──────────────────┐              ┌──────────────────┐        │
│  │ WS → TCP Copy    │              │ TCP → WS Copy    │        │
│  │ (goroutine)      │              │ (goroutine)      │        │
│  │                  │              │                  │        │
│  │ Read WS frame    │              │ Read TCP bytes   │        │
│  │ Extract bytes    │              │ Wrap in WS frame │        │
│  │ Write to TCP     │              │ Send WS frame    │        │
│  └──────────────────┘              └──────────────────┘        │
│         ↓                                    ↓                  │
└─────────────────────────────────────────────────────────────────┘
                      ↓
         ┌────────────────────────────┐
         │ Remote Access Server       │
         │ localhost:2222             │
         └────────────────────────────┘
```

### Detailed Implementation

#### File Structure

```
images/websocket-proxy/
├── main.go              # Entry point, HTTP server setup
├── handler.go           # WebSocket upgrade and proxy logic
├── copier.go            # Bidirectional copy implementation
├── metrics.go           # Prometheus metrics
├── config.go            # Configuration from env vars
├── Dockerfile           # Multi-stage build
├── go.mod               # Go module definition
├── go.sum               # Dependency checksums
└── README.md            # Usage documentation
```

#### main.go - Entry Point

**Purpose**: Initialize server, setup routes, handle graceful shutdown

**Key Concepts**:

1. **Graceful Shutdown**: Respond to SIGTERM/SIGINT, drain connections
2. **Health Checks**: Kubernetes probes need fast response
3. **Metrics Endpoint**: Prometheus scraping
4. **Configuration**: Environment variables for flexibility

**Implementation**:

```go
package main

import (
    "context"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"
    
    "github.com/prometheus/client_golang/prometheus/promhttp"
    log "github.com/sirupsen/logrus"
)

func main() {
    // Load configuration from environment
    config := LoadConfig()
    
    // Setup structured logging
    log.SetFormatter(&log.JSONFormatter{})
    log.SetLevel(log.InfoLevel)
    if config.Debug {
        log.SetLevel(log.DebugLevel)
    }
    
    // Initialize metrics
    InitMetrics()
    
    // Create HTTP server with timeouts
    // Why timeouts: Prevent resource exhaustion from slow clients
    mux := http.NewServeMux()
    
    // Health check endpoint
    // Why separate from /: Kubernetes probes shouldn't upgrade to WebSocket
    mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("OK"))
    })
    
    // Metrics endpoint for Prometheus
    mux.Handle("/metrics", promhttp.Handler())
    
    // WebSocket proxy endpoint
    // Why root path: Simplifies client configuration
    mux.HandleFunc("/", HandleWebSocket(config))
    
    server := &http.Server{
        Addr:         config.ListenAddr,
        Handler:      mux,
        ReadTimeout:  15 * time.Second,  // Prevent slow read attacks
        WriteTimeout: 15 * time.Second,  // Prevent slow write attacks
        IdleTimeout:  60 * time.Second,  // Close idle connections
    }
    
    // Start server in goroutine
    go func() {
        log.Infof("Starting WebSocket proxy on %s", config.ListenAddr)
        if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("Server failed: %v", err)
        }
    }()
    
    // Wait for interrupt signal
    // Why graceful shutdown: Allow in-flight connections to complete
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    <-sigChan
    
    log.Info("Shutting down gracefully...")
    
    // Give connections 30 seconds to complete
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    if err := server.Shutdown(ctx); err != nil {
        log.Errorf("Shutdown error: %v", err)
    }
    
    log.Info("Server stopped")
}
```

**Why This Design**:
- **Graceful Shutdown**: Kubernetes sends SIGTERM before killing pod, we drain connections
- **Separate Health Endpoint**: Probes don't interfere with WebSocket connections
- **Timeouts**: Prevent resource exhaustion from malicious/buggy clients
- **Structured Logging**: JSON format for log aggregation (CloudWatch, Splunk, etc.)

#### config.go - Configuration

**Purpose**: Load configuration from environment variables with sensible defaults

**Key Concepts**:

1. **12-Factor App**: Configuration via environment, not hardcoded
2. **Defaults**: Work out-of-box for common case
3. **Validation**: Fail fast on invalid config

**Implementation**:

```go
package main

import (
    "fmt"
    "os"
    "strconv"
)

type Config struct {
    ListenAddr string // Address to listen on (e.g., ":8080")
    TargetHost string // Target host to proxy to (e.g., "localhost")
    TargetPort string // Target port to proxy to (e.g., "2222")
    Debug      bool   // Enable debug logging
}

func LoadConfig() *Config {
    config := &Config{
        ListenAddr: getEnv("LISTEN_ADDR", ":8080"),
        TargetHost: getEnv("TARGET_HOST", "localhost"),
        TargetPort: getEnv("TARGET_PORT", "2222"),
        Debug:      getEnvBool("DEBUG", false),
    }
    
    // Validate configuration
    // Why validate: Fail fast rather than runtime errors
    if config.TargetPort == "" {
        panic("TARGET_PORT cannot be empty")
    }
    
    return config
}

func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
    if value := os.Getenv(key); value != "" {
        b, err := strconv.ParseBool(value)
        if err != nil {
            return defaultValue
        }
        return b
    }
    return defaultValue
}
```

**Why This Design**:
- **Environment Variables**: Standard Kubernetes pattern, easy to override
- **Defaults**: localhost:2222 is the remote access server location
- **Validation**: Catch misconfigurations early
- **Type Safety**: Separate functions for string/bool/int parsing

#### handler.go - WebSocket Upgrade and Proxy

**Purpose**: Handle WebSocket upgrade, establish TCP connection, coordinate copying

**Key Concepts**:

1. **WebSocket Upgrade**: HTTP → WebSocket protocol switch
2. **Connection Pairing**: One WebSocket ↔ One TCP connection
3. **Error Propagation**: Error in either direction closes both
4. **Resource Cleanup**: Ensure connections closed on exit

**Implementation**:

```go
package main

import (
    "fmt"
    "net"
    "net/http"
    "time"
    
    "github.com/gorilla/websocket"
    log "github.com/sirupsen/logrus"
)

// WebSocket upgrader configuration
// Why these settings:
// - ReadBufferSize/WriteBufferSize: 4KB is optimal for SSH traffic
// - CheckOrigin: Traefik middleware handles auth, we trust all origins
// - HandshakeTimeout: Prevent slow handshake attacks
var upgrader = websocket.Upgrader{
    ReadBufferSize:  4096,
    WriteBufferSize: 4096,
    CheckOrigin: func(r *http.Request) bool {
        // Origin check handled by Traefik JWT middleware
        // By the time request reaches us, it's already authenticated
        return true
    },
    HandshakeTimeout: 10 * time.Second,
}

func HandleWebSocket(config *Config) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Extract metadata for logging
        // Why: Debugging and audit trail
        remoteAddr := r.RemoteAddr
        userAgent := r.UserAgent()
        
        logger := log.WithFields(log.Fields{
            "remote_addr": remoteAddr,
            "user_agent":  userAgent,
        })
        
        logger.Info("WebSocket connection request received")
        
        // Upgrade HTTP connection to WebSocket
        // Why: WebSocket provides bidirectional channel over HTTP
        wsConn, err := upgrader.Upgrade(w, r, nil)
        if err != nil {
            logger.Errorf("WebSocket upgrade failed: %v", err)
            MetricsConnectionErrors.WithLabelValues("upgrade_failed").Inc()
            return
        }
        defer wsConn.Close()
        
        logger.Info("WebSocket upgrade successful")
        
        // Connect to target TCP server
        // Why dial timeout: Prevent hanging if target unreachable
        targetAddr := fmt.Sprintf("%s:%s", config.TargetHost, config.TargetPort)
        dialer := &net.Dialer{
            Timeout: 5 * time.Second,
        }
        
        tcpConn, err := dialer.Dial("tcp", targetAddr)
        if err != nil {
            logger.Errorf("TCP connection to %s failed: %v", targetAddr, err)
            MetricsConnectionErrors.WithLabelValues("tcp_dial_failed").Inc()
            
            // Send close message to client
            // Why: Inform client of failure reason
            wsConn.WriteMessage(
                websocket.CloseMessage,
                websocket.FormatCloseMessage(
                    websocket.CloseInternalServerErr,
                    "Backend unavailable",
                ),
            )
            return
        }
        defer tcpConn.Close()
        
        logger.Infof("TCP connection to %s established", targetAddr)
        
        // Track active connection
        MetricsActiveConnections.Inc()
        MetricsConnectionsTotal.Inc()
        defer MetricsActiveConnections.Dec()
        
        // Start bidirectional copy
        // Why goroutines: Need to copy in both directions simultaneously
        // Why error channel: Coordinate shutdown when either direction fails
        errChan := make(chan error, 2)
        
        // WebSocket → TCP
        go func() {
            err := copyWebSocketToTCP(wsConn, tcpConn, logger)
            errChan <- err
        }()
        
        // TCP → WebSocket
        go func() {
            err := copyTCPToWebSocket(tcpConn, wsConn, logger)
            errChan <- err
        }()
        
        // Wait for first error (or completion)
        // Why: Either direction failing means connection is done
        err = <-errChan
        if err != nil {
            logger.Warnf("Connection closed with error: %v", err)
        } else {
            logger.Info("Connection closed normally")
        }
        
        // Both goroutines will exit when connections close
        // Defer statements ensure cleanup
    }
}
```

**Why This Design**:
- **Upgrade First**: Validate WebSocket before connecting to TCP (fail fast)
- **Defer Cleanup**: Ensure connections closed even if panic occurs
- **Error Channel**: Coordinate shutdown between goroutines
- **Metrics**: Track connection lifecycle for monitoring
- **Logging**: Structured logs with context for debugging

#### copier.go - Bidirectional Copy

**Purpose**: Copy data between WebSocket and TCP in both directions

**Key Concepts**:

1. **Binary Frames**: SSH protocol is binary, use WebSocket binary frames
2. **Blocking I/O**: Read blocks until data available or error
3. **Error Handling**: Network errors are normal (connection closed), don't panic
4. **Metrics**: Track bytes transferred for capacity planning

**Implementation**:

```go
package main

import (
    "io"
    "net"
    
    "github.com/gorilla/websocket"
    log "github.com/sirupsen/logrus"
)

// copyWebSocketToTCP reads from WebSocket and writes to TCP
// Why separate function: Clear separation of concerns, easier to test
func copyWebSocketToTCP(ws *websocket.Conn, tcp net.Conn, logger *log.Entry) error {
    for {
        // Read WebSocket message
        // Why ReadMessage: Handles frame parsing, returns complete message
        messageType, data, err := ws.ReadMessage()
        if err != nil {
            // Connection closed or error
            // Why check error type: Distinguish normal close from error
            if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
                logger.Debug("WebSocket closed normally")
                return nil
            }
            logger.Debugf("WebSocket read error: %v", err)
            return err
        }
        
        // Only process binary messages
        // Why: SSH protocol is binary, text messages are unexpected
        if messageType != websocket.BinaryMessage {
            logger.Warnf("Unexpected message type: %d", messageType)
            continue
        }
        
        // Write to TCP connection
        // Why Write not WriteAll: Write handles partial writes internally
        n, err := tcp.Write(data)
        if err != nil {
            logger.Debugf("TCP write error: %v", err)
            return err
        }
        
        // Track bytes transferred
        // Why metrics: Capacity planning and debugging
        MetricsBytesTransferred.WithLabelValues("ws_to_tcp").Add(float64(n))
        
        logger.Debugf("Forwarded %d bytes: WebSocket → TCP", n)
    }
}

// copyTCPToWebSocket reads from TCP and writes to WebSocket
// Why separate function: Symmetric with copyWebSocketToTCP
func copyTCPToWebSocket(tcp net.Conn, ws *websocket.Conn, logger *log.Entry) error {
    // Buffer for reading TCP data
    // Why 32KB: Balance between memory usage and syscall overhead
    // SSH typically sends <16KB per packet, 32KB handles bursts
    buf := make([]byte, 32*1024)
    
    for {
        // Read from TCP connection
        // Why Read not ReadFull: Read returns when data available
        n, err := tcp.Read(buf)
        if err != nil {
            // Connection closed or error
            // Why check EOF: Normal connection close
            if err == io.EOF {
                logger.Debug("TCP connection closed")
                return nil
            }
            logger.Debugf("TCP read error: %v", err)
            return err
        }
        
        // Write to WebSocket as binary message
        // Why BinaryMessage: SSH protocol is binary
        // Why buf[:n]: Only send bytes actually read
        err = ws.WriteMessage(websocket.BinaryMessage, buf[:n])
        if err != nil {
            logger.Debugf("WebSocket write error: %v", err)
            return err
        }
        
        // Track bytes transferred
        MetricsBytesTransferred.WithLabelValues("tcp_to_ws").Add(float64(n))
        
        logger.Debugf("Forwarded %d bytes: TCP → WebSocket", n)
    }
}
```

**Why This Design**:
- **Blocking I/O**: Simpler than async, goroutines handle concurrency
- **Binary Frames**: SSH is binary protocol, text frames would corrupt data
- **Buffer Size**: 32KB balances memory and performance
- **Error Handling**: Distinguish normal close from errors
- **Metrics**: Track data flow for monitoring

**Performance Considerations**:

1. **Buffer Size**: 32KB chosen because:
   - SSH packets typically <16KB
   - Larger buffers reduce syscalls
   - Too large wastes memory (many connections)
   - Tested: 32KB optimal for SSH workload

2. **Goroutines**: Two per connection:
   - Lightweight (~2KB stack)
   - 1000 connections = 2000 goroutines = ~4MB
   - Go scheduler handles efficiently

3. **Memory Allocation**:
   - Buffer allocated once per connection
   - WebSocket library reuses frame buffers
   - No per-message allocation

#### metrics.go - Prometheus Metrics

**Purpose**: Export metrics for monitoring and alerting

**Key Concepts**:

1. **Counter**: Monotonically increasing (total connections, bytes)
2. **Gauge**: Current value (active connections)
3. **Histogram**: Distribution (connection duration)
4. **Labels**: Dimensions for filtering (direction, error type)

**Implementation**:

```go
package main

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // Total connections established
    // Why counter: Want to track rate of new connections
    MetricsConnectionsTotal = promauto.NewCounter(prometheus.CounterOpts{
        Name: "websocket_proxy_connections_total",
        Help: "Total number of WebSocket connections established",
    })
    
    // Currently active connections
    // Why gauge: Want to know current load
    MetricsActiveConnections = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "websocket_proxy_active_connections",
        Help: "Number of currently active WebSocket connections",
    })
    
    // Connection errors by type
    // Why labels: Distinguish error types for debugging
    MetricsConnectionErrors = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "websocket_proxy_connection_errors_total",
            Help: "Total number of connection errors by type",
        },
        []string{"error_type"},
    )
    
    // Bytes transferred by direction
    // Why labels: Track upload vs download separately
    MetricsBytesTransferred = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "websocket_proxy_bytes_transferred_total",
            Help: "Total bytes transferred by direction",
        },
        []string{"direction"},
    )
    
    // Connection duration histogram
    // Why histogram: Want percentiles (p50, p95, p99)
    MetricsConnectionDuration = promauto.NewHistogram(prometheus.HistogramOpts{
        Name: "websocket_proxy_connection_duration_seconds",
        Help: "Connection duration in seconds",
        Buckets: []float64{1, 5, 10, 30, 60, 300, 600, 1800, 3600},
    })
)

func InitMetrics() {
    // Metrics are auto-registered by promauto
    // This function exists for future initialization needs
}
```

**Why These Metrics**:
- **connections_total**: Rate of new connections (connections/sec)
- **active_connections**: Current load, capacity planning
- **connection_errors_total**: Error rate, alert on spikes
- **bytes_transferred_total**: Bandwidth usage, cost estimation
- **connection_duration_seconds**: Session length distribution

**Alerting Rules** (Prometheus):
```yaml
# High error rate
- alert: WebSocketProxyHighErrorRate
  expr: rate(websocket_proxy_connection_errors_total[5m]) > 0.05
  annotations:
    summary: "WebSocket proxy error rate >5%"

# High connection count
- alert: WebSocketProxyHighLoad
  expr: websocket_proxy_active_connections > 1000
  annotations:
    summary: "WebSocket proxy has >1000 active connections"
```

---

#### Dockerfile - Container Image

**Purpose**: Build minimal, secure container image

**Key Concepts**:

1. **Multi-Stage Build**: Separate build and runtime environments
2. **Minimal Base**: Alpine Linux for small image size
3. **Non-Root User**: Security best practice
4. **Static Binary**: No runtime dependencies

**Implementation**:

```dockerfile
# Stage 1: Build
# Why golang:1.21-alpine: Latest stable Go, minimal base
FROM golang:1.21-alpine AS builder

# Install build dependencies
# Why git: Go modules may need git for private repos
# Why ca-certificates: HTTPS connections during build
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /build

# Copy go.mod and go.sum first
# Why: Docker layer caching, dependencies change less than code
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY *.go ./

# Build binary
# Why CGO_ENABLED=0: Static binary, no C dependencies
# Why -ldflags="-w -s": Strip debug info, reduce size
# Why -trimpath: Remove build path from binary (security)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -trimpath \
    -o websocket-proxy .

# Stage 2: Runtime
# Why alpine:latest: Minimal base, ~5MB
FROM alpine:latest

# Install runtime dependencies
# Why ca-certificates: HTTPS connections at runtime
RUN apk --no-cache add ca-certificates

# Create non-root user
# Why: Security best practice, limit blast radius
# Why UID 65532: Standard "nobody" UID
RUN adduser -D -u 65532 -g 65532 proxy

# Copy binary from builder
COPY --from=builder /build/websocket-proxy /usr/local/bin/websocket-proxy

# Set ownership
RUN chown proxy:proxy /usr/local/bin/websocket-proxy

# Switch to non-root user
USER proxy

# Expose port
# Why 8080: Non-privileged port (>1024)
EXPOSE 8080

# Health check
# Why: Kubernetes uses this for liveness/readiness
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run binary
ENTRYPOINT ["/usr/local/bin/websocket-proxy"]
```

**Why This Design**:
- **Multi-Stage**: Build image ~800MB, runtime image ~15MB
- **Static Binary**: No libc dependency, portable
- **Non-Root**: Limits damage if container compromised
- **Health Check**: Kubernetes can detect unhealthy containers

**Build and Push**:
```bash
# Build
docker build -t websocket-proxy:v1.0.0 .

# Tag for registry
docker tag websocket-proxy:v1.0.0 \
    public.ecr.aws/jupyter-k8s/websocket-proxy:v1.0.0

# Push
docker push public.ecr.aws/jupyter-k8s/websocket-proxy:v1.0.0
```

### Testing the Proxy

#### Unit Tests

**File**: `handler_test.go`

```go
package main

import (
    "net"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"
    
    "github.com/gorilla/websocket"
)

func TestWebSocketUpgrade(t *testing.T) {
    // Start mock TCP server
    listener, err := net.Listen("tcp", "127.0.0.1:0")
    if err != nil {
        t.Fatal(err)
    }
    defer listener.Close()
    
    go func() {
        conn, _ := listener.Accept()
        defer conn.Close()
        // Echo server
        buf := make([]byte, 1024)
        for {
            n, err := conn.Read(buf)
            if err != nil {
                return
            }
            conn.Write(buf[:n])
        }
    }()
    
    // Create test server
    config := &Config{
        TargetHost: "127.0.0.1",
        TargetPort: listener.Addr().(*net.TCPAddr).Port,
    }
    
    server := httptest.NewServer(HandleWebSocket(config))
    defer server.Close()
    
    // Connect WebSocket client
    wsURL := "ws" + server.URL[4:] // Replace http with ws
    ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
    if err != nil {
        t.Fatal(err)
    }
    defer ws.Close()
    
    // Send test message
    testData := []byte("hello world")
    err = ws.WriteMessage(websocket.BinaryMessage, testData)
    if err != nil {
        t.Fatal(err)
    }
    
    // Read echo response
    _, response, err := ws.ReadMessage()
    if err != nil {
        t.Fatal(err)
    }
    
    // Verify echo
    if string(response) != string(testData) {
        t.Errorf("Expected %s, got %s", testData, response)
    }
}
```

**Why This Test**:
- **Integration**: Tests full WebSocket → TCP → WebSocket flow
- **Echo Server**: Simple verification of bidirectional copy
- **Cleanup**: Defer statements ensure resources freed

#### Load Tests

**Tool**: `k6` (load testing tool)

**Script**: `load_test.js`

```javascript
import ws from 'k6/ws';
import { check } from 'k6';

export let options = {
  stages: [
    { duration: '1m', target: 100 },  // Ramp up to 100 connections
    { duration: '5m', target: 100 },  // Stay at 100 for 5 minutes
    { duration: '1m', target: 0 },    // Ramp down
  ],
};

export default function () {
  const url = 'wss://workspace-test.example.com/ssh-ws?token=...';
  
  const res = ws.connect(url, function (socket) {
    socket.on('open', function () {
      // Send test data
      socket.send('test message');
    });
    
    socket.on('message', function (data) {
      // Verify response
      check(data, { 'received response': (d) => d.length > 0 });
    });
    
    socket.on('close', function () {
      // Connection closed
    });
    
    // Keep connection open for 30 seconds
    socket.setTimeout(function () {
      socket.close();
    }, 30000);
  });
}
```

**Why This Test**:
- **Realistic Load**: 100 concurrent connections typical for small cluster
- **Duration**: 5 minutes tests stability
- **Metrics**: k6 reports latency, throughput, errors

**Expected Results**:
- p95 latency: <100ms
- Error rate: <1%
- Memory usage: <64Mi per connection
- CPU usage: <50m per connection

---

## Connection Handler Architecture

### Overview

The connection handler architecture uses the **Strategy Pattern** to support multiple connection methods (SSM, WebSocket, future methods) without modifying existing code.

### Design Pattern: Strategy

**Why Strategy Pattern**:
- **Open/Closed Principle**: Open for extension (add handlers), closed for modification
- **Single Responsibility**: Each handler focuses on one connection method
- **Testability**: Handlers tested independently
- **Flexibility**: Easy to add/remove handlers

**Pattern Structure**:

```
┌─────────────────────────────────────────────────────────────────┐
│ ConnectionHandler (Interface)                                   │
│  + GenerateConnectionURL(workspace, accessStrategy) → (url, err)│
└─────────────────────────────────────────────────────────────────┘
                          ▲
                          │ implements
         ┌────────────────┴────────────────┐
         │                                  │
┌────────────────────┐          ┌──────────────────────┐
│ SSMHandler         │          │ WebSocketHandler     │
│ (Existing)         │          │ (New)                │
│                    │          │                      │
│ + Generate...()    │          │ + Generate...()      │
│   - Find instance  │          │   - Generate JWT     │
│   - Start session  │          │   - Build WSS URL    │
│   - Return URL     │          │   - Return URL       │
└────────────────────┘          └──────────────────────┘
```

### Implementation

#### connection_handler.go - Interface and Factory

**Purpose**: Define handler interface and factory for creating handlers

**Key Concepts**:

1. **Interface**: Contract all handlers must implement
2. **Factory**: Creates appropriate handler based on strategy type
3. **Error Handling**: Return error if handler type unknown

**Implementation**:

```go
// File: internal/extensionapi/connection_handler.go
package extensionapi

import (
    "context"
    "fmt"
    
    workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// ConnectionHandler defines the interface for generating connection URLs
// Why interface: Allows multiple implementations (SSM, WebSocket, future methods)
type ConnectionHandler interface {
    // GenerateConnectionURL creates a connection URL for the given workspace
    // Returns: (connectionType, connectionURL, error)
    // Why return connectionType: Different handlers may return different types
    GenerateConnectionURL(
        ctx context.Context,
        workspace *workspacev1alpha1.Workspace,
        accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
        namespace string,
    ) (string, string, error)
}

// ConnectionHandlerFactory creates appropriate handler based on strategy
// Why factory: Centralized handler creation, easy to add new types
type ConnectionHandlerFactory struct {
    server *ExtensionServer
}

// NewConnectionHandlerFactory creates a new factory
func NewConnectionHandlerFactory(server *ExtensionServer) *ConnectionHandlerFactory {
    return &ConnectionHandlerFactory{
        server: server,
    }
}

// CreateHandler returns appropriate handler for the access strategy
// Why method on factory: Encapsulates handler creation logic
func (f *ConnectionHandlerFactory) CreateHandler(
    accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
) (ConnectionHandler, error) {
    // Extract handler type from access strategy
    // Why from accessStrategy: Per-workspace configuration
    handlerType := accessStrategy.Spec.CreateConnectionHandler
    
    switch handlerType {
    case "aws":
        // SSM handler (existing implementation)
        return NewSSMConnectionHandler(f.server), nil
        
    case "websocket":
        // WebSocket handler (new implementation)
        return NewWebSocketConnectionHandler(f.server), nil
        
    default:
        // Unknown handler type
        // Why error: Fail fast rather than silent failure
        return nil, fmt.Errorf("unknown connection handler type: %s", handlerType)
    }
}
```

**Why This Design**:
- **Interface**: Defines contract, ensures all handlers compatible
- **Factory**: Centralizes creation, easy to add new handlers
- **Error Handling**: Unknown types caught early
- **Extensibility**: Add new handler = add new case in switch

#### ssm_connection_handler.go - SSM Implementation

**Purpose**: Wrap existing SSM logic in handler interface

**Key Concepts**:

1. **Adapter Pattern**: Adapt existing code to new interface
2. **No Logic Changes**: Just wraps existing `generateVSCodeURL`
3. **Backward Compatibility**: Existing SSM workspaces unchanged

**Implementation**:

```go
// File: internal/extensionapi/ssm_connection_handler.go
package extensionapi

import (
    "context"
    
    connectionv1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/connection/v1alpha1"
    workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// SSMConnectionHandler handles SSM-based connections
// Why struct: Holds reference to server for accessing k8sClient, etc.
type SSMConnectionHandler struct {
    server *ExtensionServer
}

// NewSSMConnectionHandler creates a new SSM handler
func NewSSMConnectionHandler(server *ExtensionServer) *SSMConnectionHandler {
    return &SSMConnectionHandler{
        server: server,
    }
}

// GenerateConnectionURL implements ConnectionHandler interface
// Why: Adapts existing generateVSCodeURL to new interface
func (h *SSMConnectionHandler) GenerateConnectionURL(
    ctx context.Context,
    workspace *workspacev1alpha1.Workspace,
    accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
    namespace string,
) (string, string, error) {
    // Call existing SSM logic
    // Why reuse: No need to rewrite working code
    // Note: generateVSCodeURL is existing function in serverroute_connection.go
    return h.server.generateVSCodeURL(
        nil, // request not needed for SSM
        workspace,
        accessStrategy,
        namespace,
    )
}
```

**Why This Design**:
- **Minimal Changes**: Existing SSM code unchanged
- **Adapter**: Wraps existing function in new interface
- **Backward Compat**: SSM workspaces work exactly as before

#### websocket_connection_handler.go - WebSocket Implementation

**Purpose**: Generate WebSocket connection URLs with JWT tokens

**Key Concepts**:

1. **JWT Generation**: Reuse existing JWT library
2. **URL Construction**: Build WSS URL with token
3. **Domain Extraction**: Get domain from access strategy config
4. **Namespace Encoding**: Base32 encode for DNS-safe subdomain

**Implementation**:

```go
// File: internal/extensionapi/websocket_connection_handler.go
package extensionapi

import (
    "context"
    "fmt"
    
    connectionv1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/connection/v1alpha1"
    workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
    "github.com/jupyter-infra/jupyter-k8s/internal/jwt"
    "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
)

// WebSocketConnectionHandler handles WebSocket-based connections
type WebSocketConnectionHandler struct {
    server *ExtensionServer
}

// NewWebSocketConnectionHandler creates a new WebSocket handler
func NewWebSocketConnectionHandler(server *ExtensionServer) *WebSocketConnectionHandler {
    return &WebSocketConnectionHandler{
        server: server,
    }
}

// GenerateConnectionURL implements ConnectionHandler interface
// Why: Creates WebSocket URL with JWT token
func (h *WebSocketConnectionHandler) GenerateConnectionURL(
    ctx context.Context,
    workspace *workspacev1alpha1.Workspace,
    accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
    namespace string,
) (string, string, error) {
    logger := h.server.logger.WithValues(
        "workspace", workspace.Name,
        "namespace", namespace,
    )
    
    // Extract domain from access strategy
    // Why from config: Allows different domains per strategy
    domain, ok := accessStrategy.Spec.CreateConnectionContext["domain"]
    if !ok || domain == "" {
        return "", "", fmt.Errorf("domain not configured in access strategy")
    }
    
    // Get user from context (set by auth middleware)
    // Why: JWT token needs user claim
    user := GetUser(h.server.request) // Assuming request stored in server
    if user == "" {
        return "", "", fmt.Errorf("user not found in request context")
    }
    
    // Generate JWT token
    // Why JWT: Stateless authentication, already used for web UI
    token, err := h.generateJWTToken(user, workspace, accessStrategy, namespace)
    if err != nil {
        logger.Error(err, "Failed to generate JWT token")
        return "", "", fmt.Errorf("failed to generate JWT token: %w", err)
    }
    
    // Build WebSocket URL
    // Format: wss://{workspace}-{namespace-b32}.{domain}/ssh-ws?token={jwt}
    // Why this format:
    // - wss://: Secure WebSocket (TLS)
    // - {workspace}-{namespace-b32}: Unique subdomain per workspace
    // - {domain}: Configured domain (e.g., workspaces.example.com)
    // - /ssh-ws: Path for SSH WebSocket endpoint
    // - ?token={jwt}: JWT for authentication
    
    // Encode namespace for DNS safety
    // Why base32: DNS-safe, no special characters
    encodedNamespace := workspace.EncodeNamespaceB32(namespace)
    
    url := fmt.Sprintf(
        "wss://%s-%s.%s/ssh-ws?token=%s",
        workspace.Name,
        encodedNamespace,
        domain,
        token,
    )
    
    logger.Info("Generated WebSocket connection URL",
        "domain", domain,
        "encodedNamespace", encodedNamespace,
    )
    
    return connectionv1alpha1.ConnectionTypeVSCodeRemote, url, nil
}

// generateJWTToken creates a JWT token for WebSocket authentication
// Why separate function: Encapsulates token generation logic
func (h *WebSocketConnectionHandler) generateJWTToken(
    user string,
    workspace *workspacev1alpha1.Workspace,
    accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
    namespace string,
) (string, error) {
    // Create signer based on access strategy
    // Why from accessStrategy: May use different signing keys
    signer, err := h.server.signerFactory.CreateSigner(accessStrategy)
    if err != nil {
        return "", fmt.Errorf("failed to create signer: %w", err)
    }
    
    // Generate token with workspace claims
    // Why these parameters:
    // - user: Subject of token
    // - groups: User's groups (for authorization)
    // - subject: User identifier
    // - headers: Additional metadata
    // - path: /ssh-ws (token valid for this path only)
    // - domain: workspace-namespace.domain.com
    // - tokenType: WebSocketConnection (for audit trail)
    
    // Build domain for token
    encodedNamespace := workspace.EncodeNamespaceB32(namespace)
    domain := fmt.Sprintf(
        "%s-%s.%s",
        workspace.Name,
        encodedNamespace,
        accessStrategy.Spec.CreateConnectionContext["domain"],
    )
    
    // Add workspace metadata to headers
    // Why: Traefik middleware can validate workspace claim
    headers := map[string][]string{
        "X-Workspace":  {workspace.Name},
        "X-Namespace":  {namespace},
        "X-Connection": {"vscode-remote"},
    }
    
    token, err := signer.GenerateToken(
        user,                              // user
        []string{},                        // groups (TODO: extract from user)
        user,                              // subject
        headers,                           // headers
        "/ssh-ws",                         // path
        domain,                            // domain
        jwt.TokenTypeWebSocketConnection, // tokenType (NEW constant)
    )
    
    if err != nil {
        return "", fmt.Errorf("failed to generate token: %w", err)
    }
    
    return token, nil
}
```

**Why This Design**:
- **JWT Reuse**: Leverages existing JWT infrastructure
- **Domain Config**: Flexible domain per access strategy
- **Namespace Encoding**: DNS-safe subdomains
- **Token Claims**: Workspace metadata for validation
- **Error Handling**: Detailed errors for debugging

**Token Structure**:
```json
{
  "header": {
    "alg": "HS256",
    "typ": "JWT"
  },
  "payload": {
    "sub": "alice@example.com",
    "exp": 1707598800,
    "iat": 1707595200,
    "path": "/ssh-ws",
    "domain": "my-workspace-default.workspaces.example.com",
    "X-Workspace": "my-workspace",
    "X-Namespace": "default",
    "X-Connection": "vscode-remote"
  },
  "signature": "..."
}
```

---

### Integration with Extension API

**File**: `internal/extensionapi/serverroute_connection.go`

**Modifications**: Update `HandleConnectionCreate` to use factory

**Before** (existing code):
```go
func (s *ExtensionServer) HandleConnectionCreate(w http.ResponseWriter, r *http.Request) {
    // ... validation code ...
    
    // Generate connection URL (hardcoded to SSM)
    responseType, responseURL, err = s.generateVSCodeURL(r, ws, accessStrategy, namespace)
    
    // ... response code ...
}
```

**After** (with factory):
```go
func (s *ExtensionServer) HandleConnectionCreate(w http.ResponseWriter, r *http.Request) {
    logger := GetLoggerFromContext(r.Context())
    
    // ... existing validation code unchanged ...
    
    // Create connection handler based on access strategy
    // Why factory: Routes to appropriate handler (SSM or WebSocket)
    factory := NewConnectionHandlerFactory(s)
    handler, err := factory.CreateHandler(accessStrategy)
    if err != nil {
        logger.Error(err, "Failed to create connection handler",
            "handlerType", accessStrategy.Spec.CreateConnectionHandler)
        WriteKubernetesError(w, http.StatusBadRequest,
            fmt.Sprintf("Unsupported connection handler: %s",
                accessStrategy.Spec.CreateConnectionHandler))
        return
    }
    
    // Generate connection URL using handler
    // Why interface: Same call works for any handler type
    responseType, responseURL, err := handler.GenerateConnectionURL(
        r.Context(),
        ws,
        accessStrategy,
        namespace,
    )
    if err != nil {
        logger.Error(err, "Failed to generate connection URL")
        WriteKubernetesError(w, http.StatusInternalServerError, err.Error())
        return
    }
    
    // ... existing response code unchanged ...
}
```

**Why This Change**:
- **Minimal Impact**: Only connection URL generation changes
- **Backward Compatible**: SSM workspaces use SSM handler
- **Extensible**: Add new handler = add case in factory
- **Testable**: Can mock handler interface

---

## Ingress and Routing

### Traefik IngressRoute Configuration

**Purpose**: Route WebSocket traffic from internet to workspace pods

**Key Concepts**:

1. **IngressRoute**: Traefik CRD for advanced routing
2. **Host-Based Routing**: Each workspace gets unique subdomain
3. **Middleware Chain**: JWT validation before proxying
4. **WebSocket Support**: Traefik handles upgrade automatically

### IngressRoute Template

**Location**: `accessResourceTemplates` in WorkspaceAccessStrategy

**Why Template**: Dynamic values (workspace name, namespace) filled at runtime

**Implementation**:

```yaml
apiVersion: workspace.jupyter.org/v1alpha1
kind: WorkspaceAccessStrategy
metadata:
  name: websocket-access-strategy
spec:
  displayName: "WebSocket Remote Access"
  createConnectionHandler: "websocket"
  
  createConnectionContext:
    domain: "workspaces.example.com"
    proxyPort: "8080"
  
  accessResourceTemplates:
    # IngressRoute for WebSocket connections
    - kind: IngressRoute
      apiVersion: traefik.io/v1alpha1
      namePrefix: workspace-ssh-ws
      template: |
        spec:
          # Entry point for HTTPS traffic
          # Why websecure: TLS termination, wss:// protocol
          entryPoints:
            - websecure
          
          routes:
            # Routing rule
            # Why Host match: Unique subdomain per workspace
            # Why PathPrefix: Specific endpoint for SSH WebSocket
            - match: "Host(`{{ .Workspace.Name }}-{{ b32encode .Workspace.Namespace }}.{{ .AccessStrategy.Spec.CreateConnectionContext.domain }}`) && PathPrefix(`/ssh-ws`)"
              kind: Rule
              priority: 100
              
              # Middleware chain (executed in order)
              middlewares:
                # JWT validation
                # Why first: Reject unauthorized requests early
                - name: jwt-verify
                  namespace: jupyter-k8s-router
                
                # Strip /ssh-ws prefix (optional)
                # Why: Backend expects / not /ssh-ws
                # - name: strip-ssh-ws-prefix
                #   namespace: jupyter-k8s-router
              
              # Backend service
              services:
                - name: "{{ .Service.Name }}"
                  namespace: "{{ .Service.Namespace }}"
                  port: 8080  # websocket-proxy port
```

**Template Variables**:
- `{{ .Workspace.Name }}`: Workspace name (e.g., "my-workspace")
- `{{ .Workspace.Namespace }}`: Namespace (e.g., "default")
- `{{ b32encode .Workspace.Namespace }}`: Base32-encoded namespace (DNS-safe)
- `{{ .AccessStrategy.Spec.CreateConnectionContext.domain }}`: Domain from config
- `{{ .Service.Name }}`: Kubernetes Service name (created by controller)
- `{{ .Service.Namespace }}`: Service namespace

**Example Rendered**:
```yaml
apiVersion: traefik.io/v1alpha1
kind: IngressRoute
metadata:
  name: workspace-ssh-ws-my-workspace
  namespace: default
spec:
  entryPoints:
    - websecure
  routes:
    - match: "Host(`my-workspace-mzxw6.workspaces.example.com`) && PathPrefix(`/ssh-ws`)"
      kind: Rule
      priority: 100
      middlewares:
        - name: jwt-verify
          namespace: jupyter-k8s-router
      services:
        - name: workspace-my-workspace
          namespace: default
          port: 8080
```

### JWT Validation Middleware

**Purpose**: Validate JWT tokens before proxying to workspace

**Type**: Traefik ForwardAuth middleware

**Why ForwardAuth**: Delegates authentication to external service

**Configuration**:

```yaml
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: jwt-verify
  namespace: jupyter-k8s-router
spec:
  forwardAuth:
    # Authentication service endpoint
    # Why: Existing authmiddleware service validates JWT
    address: http://authmiddleware-service:8080/verify
    
    # Trust forwarded headers
    # Why: Preserve original request headers
    trustForwardHeader: true
    
    # Headers to copy from auth response
    # Why: Pass user identity to backend
    authResponseHeaders:
      - X-User
      - X-Workspace
      - X-Namespace
    
    # TLS configuration (if auth service uses HTTPS)
    # tls:
    #   insecureSkipVerify: false
```

**How It Works**:

1. **Request Arrives**: Client connects to `wss://workspace.domain.com/ssh-ws?token=...`
2. **Traefik Intercepts**: Before proxying, calls ForwardAuth middleware
3. **Middleware Calls Auth Service**: 
   ```
   GET http://authmiddleware-service:8080/verify
   Headers:
     X-Forwarded-Uri: /ssh-ws?token=...
     X-Forwarded-Host: workspace.domain.com
   ```
4. **Auth Service Validates**:
   - Extracts token from query param
   - Verifies signature
   - Checks expiry
   - Validates workspace claim matches host
   - Returns 200 OK or 403 Forbidden
5. **Traefik Proxies or Rejects**:
   - If 200: Proxy to websocket-proxy:8080
   - If 403: Return 403 to client

**Auth Service Implementation** (already exists, reuse):

```go
// File: internal/authmiddleware/serverroute_verify.go (existing)
func (s *Server) HandleVerify(w http.ResponseWriter, r *http.Request) {
    // Extract token from X-Forwarded-Uri header
    forwardedURI := r.Header.Get("X-Forwarded-Uri")
    token := extractTokenFromURI(forwardedURI)
    
    // Validate token
    claims, err := s.jwtVerifier.Verify(token)
    if err != nil {
        w.WriteHeader(http.StatusForbidden)
        return
    }
    
    // Validate workspace claim matches host
    forwardedHost := r.Header.Get("X-Forwarded-Host")
    if !validateWorkspaceClaim(claims, forwardedHost) {
        w.WriteHeader(http.StatusForbidden)
        return
    }
    
    // Set response headers for backend
    w.Header().Set("X-User", claims.Subject)
    w.Header().Set("X-Workspace", claims.Workspace)
    w.Header().Set("X-Namespace", claims.Namespace)
    w.WriteHeader(http.StatusOK)
}
```

**Why This Design**:
- **Reuse**: Existing authmiddleware service, no new service needed
- **Centralized**: All JWT validation in one place
- **Flexible**: Can add more validation logic without changing Traefik
- **Observable**: Auth service logs all validation attempts

### TLS Certificate Management

**Requirement**: Wildcard certificate for `*.workspaces.example.com`

**Options**:

#### Option 1: cert-manager (Recommended)

**Why**: Automatic certificate issuance and renewal

**Configuration**:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: workspaces-wildcard-cert
  namespace: jupyter-k8s-router
spec:
  # Certificate details
  secretName: workspaces-tls
  dnsNames:
    - "*.workspaces.example.com"
    - "workspaces.example.com"
  
  # Issuer (Let's Encrypt or internal CA)
  issuerRef:
    name: letsencrypt-prod
    kind: ClusterIssuer
  
  # Renewal before expiry
  renewBefore: 720h  # 30 days
```

**Traefik TLS Configuration**:

```yaml
apiVersion: traefik.io/v1alpha1
kind: TLSStore
metadata:
  name: default
  namespace: jupyter-k8s-router
spec:
  defaultCertificate:
    secretName: workspaces-tls
```

#### Option 2: Manual Certificate

**Why**: For internal CA or existing certificates

**Steps**:
1. Obtain wildcard certificate
2. Create Kubernetes Secret:
   ```bash
   kubectl create secret tls workspaces-tls \
     --cert=wildcard.crt \
     --key=wildcard.key \
     -n jupyter-k8s-router
   ```
3. Configure Traefik to use secret (same as above)

**Why Wildcard**:
- **Scalability**: One cert covers all workspaces
- **Simplicity**: No per-workspace cert management
- **Cost**: One cert vs thousands

---

## Controller Modifications

### Service Port Exposure

**Purpose**: Expose websocket-proxy port 8080 in Kubernetes Service

**File**: `internal/controller/service_builder.go`

**Current Implementation** (simplified):
```go
func (r *WorkspaceReconciler) buildService(workspace *workspacev1alpha1.Workspace) *corev1.Service {
    return &corev1.Service{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("workspace-%s", workspace.Name),
            Namespace: workspace.Namespace,
        },
        Spec: corev1.ServiceSpec{
            Selector: map[string]string{
                "workspace.jupyter.org/workspaceName": workspace.Name,
            },
            Ports: []corev1.ServicePort{
                {
                    Name:       "http",
                    Port:       8888,
                    TargetPort: intstr.FromInt(8888),
                },
            },
        },
    }
}
```

**Modified Implementation**:
```go
func (r *WorkspaceReconciler) buildService(
    workspace *workspacev1alpha1.Workspace,
    accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
) *corev1.Service {
    // Base service with web UI port
    service := &corev1.Service{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("workspace-%s", workspace.Name),
            Namespace: workspace.Namespace,
        },
        Spec: corev1.ServiceSpec{
            Selector: map[string]string{
                "workspace.jupyter.org/workspaceName": workspace.Name,
            },
            Ports: []corev1.ServicePort{
                {
                    Name:       "http",
                    Port:       8888,
                    TargetPort: intstr.FromInt(8888),
                },
            },
        },
    }
    
    // Add websocket-proxy port if WebSocket handler configured
    // Why conditional: Only add port if websocket-proxy sidecar present
    if accessStrategy != nil && accessStrategy.Spec.CreateConnectionHandler == "websocket" {
        // Extract proxy port from config (default 8080)
        proxyPort := 8080
        if portStr, ok := accessStrategy.Spec.CreateConnectionContext["proxyPort"]; ok {
            if p, err := strconv.Atoi(portStr); err == nil {
                proxyPort = p
            }
        }
        
        // Add websocket-proxy port
        service.Spec.Ports = append(service.Spec.Ports, corev1.ServicePort{
            Name:       "websocket-proxy",
            Port:       int32(proxyPort),
            TargetPort: intstr.FromInt(proxyPort),
        })
    }
    
    return service
}
```

**Why This Design**:
- **Conditional**: Only add port if WebSocket configured
- **Configurable**: Port from access strategy config
- **Backward Compatible**: SSM workspaces unchanged (no extra port)
- **Flexible**: Can add more ports in future

### IngressRoute Creation

**Purpose**: Create IngressRoute from template in access strategy

**File**: `internal/controller/resource_manager_access.go`

**New Function**:

```go
// createIngressRoute creates an IngressRoute from template
// Why: Routes external traffic to workspace Service
func (r *ResourceManager) createIngressRoute(
    ctx context.Context,
    workspace *workspacev1alpha1.Workspace,
    accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
    service *corev1.Service,
) error {
    logger := log.FromContext(ctx).WithValues(
        "workspace", workspace.Name,
        "namespace", workspace.Namespace,
    )
    
    // Find IngressRoute template in access strategy
    // Why: Access strategy defines routing configuration
    var ingressRouteTemplate *workspacev1alpha1.AccessResourceTemplate
    for _, template := range accessStrategy.Spec.AccessResourceTemplates {
        if template.Kind == "IngressRoute" {
            ingressRouteTemplate = &template
            break
        }
    }
    
    if ingressRouteTemplate == nil {
        // No IngressRoute template, skip
        // Why: Not all access strategies need ingress (SSM doesn't)
        logger.V(1).Info("No IngressRoute template found, skipping")
        return nil
    }
    
    // Render template with workspace/service data
    // Why: Template has placeholders like {{ .Workspace.Name }}
    rendered, err := r.renderTemplate(
        ingressRouteTemplate.Template,
        workspace,
        accessStrategy,
        service,
    )
    if err != nil {
        return fmt.Errorf("failed to render IngressRoute template: %w", err)
    }
    
    // Parse rendered YAML into IngressRoute object
    // Why: Need typed object for Kubernetes API
    ingressRoute := &traefikv1alpha1.IngressRoute{}
    if err := yaml.Unmarshal([]byte(rendered), ingressRoute); err != nil {
        return fmt.Errorf("failed to parse IngressRoute YAML: %w", err)
    }
    
    // Set metadata
    ingressRoute.Name = fmt.Sprintf("%s-%s",
        ingressRouteTemplate.NamePrefix,
        workspace.Name,
    )
    ingressRoute.Namespace = workspace.Namespace
    
    // Set owner reference for garbage collection
    // Why: IngressRoute deleted when workspace deleted
    if err := controllerutil.SetControllerReference(
        workspace,
        ingressRoute,
        r.scheme,
    ); err != nil {
        return fmt.Errorf("failed to set owner reference: %w", err)
    }
    
    // Create or update IngressRoute
    // Why: Idempotent, handles both create and update
    if err := r.client.Patch(ctx, ingressRoute, client.Apply,
        client.ForceOwnership,
        client.FieldOwner("workspace-controller"),
    ); err != nil {
        return fmt.Errorf("failed to create/update IngressRoute: %w", err)
    }
    
    logger.Info("IngressRoute created/updated", "name", ingressRoute.Name)
    return nil
}

// renderTemplate renders a Go template with workspace data
// Why: Templates allow dynamic configuration
func (r *ResourceManager) renderTemplate(
    templateStr string,
    workspace *workspacev1alpha1.Workspace,
    accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
    service *corev1.Service,
) (string, error) {
    // Create template with custom functions
    // Why b32encode: DNS-safe namespace encoding
    tmpl, err := template.New("resource").Funcs(template.FuncMap{
        "b32encode": workspaceutil.EncodeNamespaceB32,
    }).Parse(templateStr)
    if err != nil {
        return "", fmt.Errorf("failed to parse template: %w", err)
    }
    
    // Prepare template data
    data := map[string]interface{}{
        "Workspace":      workspace,
        "AccessStrategy": accessStrategy,
        "Service":        service,
    }
    
    // Execute template
    var buf bytes.Buffer
    if err := tmpl.Execute(&buf, data); err != nil {
        return "", fmt.Errorf("failed to execute template: %w", err)
    }
    
    return buf.String(), nil
}
```

**Why This Design**:
- **Template-Based**: Flexible, no hardcoded values
- **Conditional**: Only create if template exists
- **Owner Reference**: Automatic cleanup when workspace deleted
- **Idempotent**: Patch operation handles create/update
- **Reusable**: Same logic for any Kubernetes resource

### Reconciliation Flow

**Modified Reconcile Function**:

```go
func (r *WorkspaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // ... existing code to fetch workspace ...
    
    // Get access strategy
    accessStrategy, err := r.getAccessStrategy(ctx, workspace)
    if err != nil {
        return ctrl.Result{}, err
    }
    
    // Create/update Service (with conditional websocket-proxy port)
    service, err := r.reconcileService(ctx, workspace, accessStrategy)
    if err != nil {
        return ctrl.Result{}, err
    }
    
    // Create/update access resources (IngressRoute, NetworkPolicy, etc.)
    // Why: Access strategy defines what resources to create
    if err := r.reconcileAccessResources(ctx, workspace, accessStrategy, service); err != nil {
        return ctrl.Result{}, err
    }
    
    // ... existing code for Deployment, PVC, etc. ...
    
    return ctrl.Result{}, nil
}
```

**Why This Flow**:
- **Service First**: Need Service before creating IngressRoute
- **Access Resources**: Generic, works for any resource type
- **Error Handling**: Return error stops reconciliation, retries later

---

