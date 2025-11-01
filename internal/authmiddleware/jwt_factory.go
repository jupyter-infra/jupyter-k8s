package authmiddleware

import (
	"context"
	"fmt"

	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/aws"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/jwt"
)

// NewJWTHandler creates a jwt.Handler based on the configured signing type
func NewJWTHandler(cfg *Config) (jwt.Handler, error) {
	var signer jwt.JWTSigner

	switch cfg.JWTSigningType {
	case JWTSigningTypeStandard:
		signer = jwt.NewStandardSigner(cfg.JWTSigningKey, cfg.JWTIssuer, cfg.JWTAudience, cfg.JWTExpiration)

	case JWTSigningTypeKMS:
		// Validate KMS key ID is provided
		if cfg.KMSKeyId == "" {
			return nil, fmt.Errorf("KMS_KEY_ID required when JWT_SIGNING_TYPE is kms")
		}

		// Create KMS client
		kmsClient, err := aws.NewKMSClient(context.Background())
		if err != nil {
			return nil, fmt.Errorf("failed to create KMS client: %w", err)
		}

		// Create KMS JWT signer
		kmsConfig := aws.KMSJWTConfig{
			KMSClient:  kmsClient,
			KeyId:      cfg.KMSKeyId,
			Issuer:     cfg.JWTIssuer,
			Audience:   cfg.JWTAudience,
			Expiration: cfg.JWTExpiration,
		}
		signer = aws.NewKMSJWTManager(kmsConfig)

	default:
		return nil, fmt.Errorf("unknown JWT signing type: %s", cfg.JWTSigningType)
	}

	return jwt.NewManager(signer, cfg.JWTRefreshEnable, cfg.JWTRefreshWindow, cfg.JWTRefreshHorizon), nil
}
