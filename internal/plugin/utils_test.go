/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package plugin

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContextWithOriginRequestID_RoundTrip(t *testing.T) {
	ctx := ContextWithOriginRequestID(context.Background(), "req-abc-123")
	assert.Equal(t, "req-abc-123", OriginRequestID(ctx))
}

func TestOriginRequestID_EmptyWhenNotSet(t *testing.T) {
	assert.Equal(t, "", OriginRequestID(context.Background()))
}

func TestOriginRequestID_Overwrite(t *testing.T) {
	ctx := ContextWithOriginRequestID(context.Background(), "first")
	ctx = ContextWithOriginRequestID(ctx, "second")
	assert.Equal(t, "second", OriginRequestID(ctx))
}

func TestGenerateRequestID_Format(t *testing.T) {
	id := GenerateRequestID()
	assert.Len(t, id, 16, "8 random bytes = 16 hex chars")
}

func TestGenerateRequestID_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for range 100 {
		id := GenerateRequestID()
		assert.False(t, seen[id], "duplicate request ID: %s", id)
		seen[id] = true
	}
}
