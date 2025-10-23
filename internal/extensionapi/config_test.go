package extensionapi

import (
	"os"
	"testing"

	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/aws"
)

func TestNewConfig(t *testing.T) {
	tests := []struct {
		name           string
		clusterARN     string
		expectedConfig *Config
	}{
		{
			name:       "with cluster ARN",
			clusterARN: "arn:aws:eks:us-west-2:123456789012:cluster/test",
			expectedConfig: &Config{
				ClusterARN: "arn:aws:eks:us-west-2:123456789012:cluster/test",
			},
		},
		{
			name:       "without cluster ARN",
			clusterARN: "",
			expectedConfig: &Config{
				ClusterARN: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.Setenv(aws.ClusterARNEnv, tt.clusterARN)
			defer func() { _ = os.Unsetenv(aws.ClusterARNEnv) }()

			config, err := NewConfig()
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if config.ClusterARN != tt.expectedConfig.ClusterARN {
				t.Errorf("expected ClusterARN %s, got %s", tt.expectedConfig.ClusterARN, config.ClusterARN)
			}
		})
	}
}
