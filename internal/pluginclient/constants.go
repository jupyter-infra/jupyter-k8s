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
	defaultRetryCount  = 2                      // 2 retries = 3 total attempts
	defaultRetryDelay  = 100 * time.Millisecond // fixed delay between retries (no backoff — it's localhost)
	defaultCallTimeout = 30 * time.Second       // per-call timeout; generous for cloud SDK operations
)
