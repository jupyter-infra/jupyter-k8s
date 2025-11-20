/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
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
