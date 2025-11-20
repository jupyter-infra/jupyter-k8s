/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package authmiddleware

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/aws"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/jwt"
)

// NewJWTHandler creates a jwt.Handler based on the configured signing type
func NewJWTHandler(cfg *Config) (jwt.Handler, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	var signer jwt.Signer

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

		// Parse encryption context from config if provided
		var encryptionContext map[string]string
		if cfg.KMSEncryptionContext != "" {
			if err := json.Unmarshal([]byte(cfg.KMSEncryptionContext), &encryptionContext); err != nil {
				return nil, fmt.Errorf("failed to parse KMS encryption context: %w", err)
			}
		}

		kmsConfig := aws.KMSJWTConfig{
			KMSClient:         kmsClient,
			KeyId:             cfg.KMSKeyId,
			Issuer:            cfg.JWTIssuer,
			Audience:          cfg.JWTAudience,
			Expiration:        cfg.JWTExpiration,
			EncryptionContext: encryptionContext,
		}
		signer = aws.NewKMSJWTManager(kmsConfig)

	default:
		return nil, fmt.Errorf("unknown JWT signing type: %s", cfg.JWTSigningType)
	}

	return jwt.NewManager(signer, cfg.JWTRefreshEnable, cfg.JWTRefreshWindow, cfg.JWTRefreshHorizon), nil
}
