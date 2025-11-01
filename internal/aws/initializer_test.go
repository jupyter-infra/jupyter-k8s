package aws

import (
	"context"
	"sync"
	"testing"
)

func TestEnsureResourcesInitialized_MissingClusterARN(t *testing.T) {
	// Reset global state for test
	initOnce = sync.Once{}
	initError = nil

	ctx := context.Background()
	// Don't set EKSClusterARNEnv

	err := EnsureResourcesInitialized(ctx)

	if err == nil {
		t.Error("Expected error when cluster ARN is missing, got nil")
	}
}

func TestEnsureResourcesInitialized_SyncOncePattern(t *testing.T) {
	// Reset global state for test
	initOnce = sync.Once{}
	initError = nil

	ctx := context.Background()
	t.Setenv(EKSClusterARNEnv, "arn:aws:eks:us-west-2:123456789012:cluster/test-cluster")

	// This test verifies that sync.Once prevents multiple initializations
	// We can't easily mock the AWS clients, but we can test that the function
	// returns the same error on subsequent calls if initialization fails

	// First call will attempt initialization and likely fail due to no AWS credentials
	err1 := EnsureResourcesInitialized(ctx)

	// Second call should return the same error without attempting initialization again
	err2 := EnsureResourcesInitialized(ctx)

	// Both errors should be identical (same error instance due to sync.Once)
	if err1 != err2 {
		t.Errorf("Expected same error instance from sync.Once, got different errors: %v vs %v", err1, err2)
	}
}
