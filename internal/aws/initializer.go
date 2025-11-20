/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package aws

import (
	"context"
	"fmt"
	"sync"
)

// ResourceInitializer handles AWS resource initialization
type ResourceInitializer struct {
	ssmClient *SSMClient
	initOnce  sync.Once
	initError error
}

// NewResourceInitializer creates a new ResourceInitializer with AWS clients
func NewResourceInitializer(ctx context.Context) (*ResourceInitializer, error) {
	ssmClient, err := NewSSMClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSM client: %w", err)
	}

	return &ResourceInitializer{
		ssmClient: ssmClient,
	}, nil
}

// EnsureResourcesInitialized ensures SSH document is created (only once)
func (r *ResourceInitializer) EnsureResourcesInitialized(ctx context.Context) error {
	r.initOnce.Do(func() {
		err := r.ssmClient.createSageMakerSpaceSSMDocument(ctx)
		if err != nil {
			r.initError = fmt.Errorf("failed to create SSH document: %w", err)
			return
		}
	})
	return r.initError
}

var globalInitializer *ResourceInitializer
var initializerOnce sync.Once

// EnsureResourcesInitialized is a global function that uses a singleton initializer
var EnsureResourcesInitialized = func(ctx context.Context) error {
	var err error
	initializerOnce.Do(func() {
		globalInitializer, err = NewResourceInitializer(ctx)
	})
	if err != nil {
		return err
	}
	return globalInitializer.EnsureResourcesInitialized(ctx)
}
