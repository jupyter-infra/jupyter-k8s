package aws

import (
	"context"
	"fmt"
	"sync"
)

var (
	initOnce  sync.Once
	initError error
)

// EnsureResourcesInitialized is a variable that holds the function to ensure resources are initialized
var EnsureResourcesInitialized = ensureResourcesInitialized

// ensureResourcesInitialized ensures SSH document is created (only once)
func ensureResourcesInitialized(ctx context.Context) error {
	initOnce.Do(func() {
		// TODO: remove these comments in future change to use customer provided KMS key

		// Create KMS key
		// kmsClient, err := NewKMSClient(ctx)
		// if err != nil {
		// 	initError = fmt.Errorf("failed to create KMS client: %w", err)
		// 	return
		// }

		// _, err = kmsClient.CreateJWTKMSKey(ctx)
		// if err != nil {
		// 	initError = fmt.Errorf("failed to create KMS key: %w", err)
		// 	return
		// }

		// Create SSH document
		ssmClient, err := NewSSMClient(ctx)
		if err != nil {
			initError = fmt.Errorf("failed to create SSM client: %w", err)
			return
		}

		err = ssmClient.createSageMakerSpaceSSMDocument(ctx)
		if err != nil {
			initError = fmt.Errorf("failed to create SSH document: %w", err)
			return
		}
	})
	return initError
}
