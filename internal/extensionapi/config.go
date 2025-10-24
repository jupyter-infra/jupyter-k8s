package extensionapi

import (
	"os"

	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/aws"
)

// Config holds configuration for the extension API server
type Config struct {
	ClusterARN string
}

// NewConfig creates a new configuration from environment variables
func NewConfig() (*Config, error) {
	clusterARN := os.Getenv(aws.ClusterARNEnv)

	return &Config{
		ClusterARN: clusterARN,
	}, nil
}
