/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package pluginclient

import "time"

// Default retry and timeout configuration for the plugin client.
// These are tuned for the sidecar (localhost) deployment model where
// transient failures are brief (sidecar starting up or restarting).
const (
	// Retry defaults are sized for a sidecar container restart on localhost.
	// A Go binary in a distroless image boots in ~200-500ms (dominated by
	// container sandbox creation, not process startup). 4 attempts at 500ms
	// intervals cover a ~1.5s window — well beyond typical restart latency,
	// while failing fast enough to avoid masking CrashLoopBackOff or config errors.
	defaultRetryCount  = 3                      // 3 retries = 4 total attempts
	defaultRetryDelay  = 500 * time.Millisecond // fixed delay between retries (no backoff — it's localhost)
	defaultCallTimeout = 30 * time.Second       // per-call timeout; generous for cloud SDK operations
)
