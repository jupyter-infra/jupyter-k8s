package extensionapi

import (
	"context"
	"os"

	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/aws"
)

// Config holds configuration for the extension API server
type Config struct {
	KMSKeyID   string
	ClusterARN string
}

// NewConfig creates a new configuration from environment variables
func NewConfig() (*Config, error) {
	// Initialize KMS key
	kmsKeyID, err := aws.CreateOrGetKMSKey(context.Background())
	if err != nil {
		return nil, err
	}

	clusterARN := os.Getenv(aws.ClusterARNEnv)

	return &Config{
		KMSKeyID:   kmsKeyID,
		ClusterARN: clusterARN,
	}, nil
}
