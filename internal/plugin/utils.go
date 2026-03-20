/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package plugin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

// originRequestIDKey is the context key for the origin request ID.
type originRequestIDKey struct{}

// ContextWithOriginRequestID returns a context with the given origin request ID.
// The client propagates this to the plugin via the X-Origin-Request-ID header.
func ContextWithOriginRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, originRequestIDKey{}, id)
}

// OriginRequestID returns the origin request ID from the context, or empty string.
func OriginRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(originRequestIDKey{}).(string); ok {
		return id
	}
	return ""
}

// GenerateRequestID returns a random 16-character hex string (8 bytes of entropy).
// Used by both the client (call IDs) and server (request IDs when none is provided).
func GenerateRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
