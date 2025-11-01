package aws

import (
	"context"
	"fmt"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	initOnce  sync.Once
	initError error
)

// EnsureResourcesInitialized ensures KMS key and SSH document are created (only once)
func EnsureResourcesInitialized(ctx context.Context) error {
	initOnce.Do(func() {
		logger := log.FromContext(ctx).WithName("resource-init")
		logger.Info("Initializing resources")

		// Create KMS key
		kmsClient, err := NewKMSClient(ctx)
		if err != nil {
			initError = fmt.Errorf("failed to create KMS client: %w", err)
			return
		}

		_, err = kmsClient.CreateJWTKMSKey(ctx)
		if err != nil {
			initError = fmt.Errorf("failed to create KMS key: %w", err)
			return
		}

		// Create SSH document
		ssmClient, err := NewSSMClient(ctx)
		if err != nil {
			initError = fmt.Errorf("failed to create SSM client: %w", err)
			return
		}

		err = ssmClient.CreateSSHDocument(ctx)
		if err != nil {
			initError = fmt.Errorf("failed to create SSH document: %w", err)
			return
		}

		logger.Info("Resources initialized successfully")
	})
	return initError
}
