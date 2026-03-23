/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package pluginclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		// Retryable: transport-level errors proving sidecar is unreachable
		{
			name:      "ECONNREFUSED — sidecar not listening",
			err:       &net.OpError{Op: "dial", Err: &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED}},
			retryable: true,
		},
		{
			name:      "ECONNRESET — sidecar dropped connection",
			err:       &net.OpError{Op: "read", Err: &os.SyscallError{Syscall: "read", Err: syscall.ECONNRESET}},
			retryable: true,
		},
		{
			name:      "EOF — connection closed before response",
			err:       io.EOF,
			retryable: true,
		},
		{
			name:      "UnexpectedEOF — partial read",
			err:       io.ErrUnexpectedEOF,
			retryable: true,
		},
		{
			name:      "wrapped ECONNREFUSED",
			err:       fmt.Errorf("Get http://localhost:8080/test: %w", &net.OpError{Op: "dial", Err: &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED}}),
			retryable: true,
		},

		// Not retryable: caller intent
		{
			name:      "context.Canceled — caller canceled",
			err:       context.Canceled,
			retryable: false,
		},
		{
			name:      "context.DeadlineExceeded — timeout",
			err:       context.DeadlineExceeded,
			retryable: false,
		},
		{
			name:      "wrapped context.Canceled",
			err:       fmt.Errorf("something: %w", context.Canceled),
			retryable: false,
		},

		// Not retryable: unknown errors
		{
			name:      "generic error",
			err:       errors.New("something went wrong"),
			retryable: false,
		},
		{
			name:      "nil error",
			err:       nil,
			retryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryableError(tt.err)
			assert.Equal(t, tt.retryable, got)
		})
	}
}
