/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package pluginclient

import (
	"context"
	"errors"
	"io"
	"syscall"
)

// isRetryableError determines whether a failed HTTP call to the plugin sidecar
// should be retried. The decision is based on a strict whitelist of errors that
// prove the sidecar was unreachable — i.e., no response was received.
//
// Retry contract:
//   - Transport errors (connection refused, reset, EOF): RETRY.
//     These indicate the sidecar is not ready (starting up, crashed, restarting).
//     Since it's localhost, transient unavailability resolves in seconds.
//   - Context cancellation / deadline exceeded: NEVER RETRY.
//     context.Canceled means the caller explicitly canceled — retrying would
//     ignore their intent. context.DeadlineExceeded means either the caller's
//     deadline or our per-call timeout expired — if the plugin is alive but slow,
//     retrying adds load without helping.
//   - Any HTTP response (including 500/503): NEVER RETRY.
//     If we got a response, the plugin processed the request. The plugin owns
//     retrying its own cloud provider calls (with exponential backoff via the
//     AWS SDK or equivalent). Retrying from the client would layer retries
//     and risk storms.
//   - Unknown errors: NEVER RETRY.
//     We only retry what we can positively identify as transient transport failures.
//     Anything else (DNS errors, TLS errors, etc.) indicates a configuration
//     problem that retrying won't fix.
func isRetryableError(err error) bool {
	// Caller canceled or their deadline expired — respect their intent.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// ECONNREFUSED: sidecar not listening. Most common transient failure —
	// the sidecar container hasn't started its HTTP server yet, or kubelet
	// is restarting it after a crash.
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}

	// ECONNRESET: sidecar dropped the connection before sending a response.
	// Since all plugin operations are idempotent, safe to retry even if the
	// request may have been partially received.
	if errors.Is(err, syscall.ECONNRESET) {
		return true
	}

	// EOF / UnexpectedEOF: connection closed cleanly or mid-stream before
	// any HTTP response was read. Same reasoning as ECONNRESET — the sidecar
	// went away before responding.
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	return false
}
