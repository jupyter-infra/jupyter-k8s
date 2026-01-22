/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package authmiddleware

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"

	"github.com/jupyter-infra/jupyter-k8s/internal/aws"
	"github.com/jupyter-infra/jupyter-k8s/internal/jwt"
)

// NewJWTHandler creates a jwt.Handler based on the configured signing type
// For standard signing, returns a StandardSigner that will be populated with keys on server start
// Returns the handler and the StandardSigner (nil for KMS)
func NewJWTHandler(cfg *Config, logger logr.Logger) (jwt.Handler, *jwt.StandardSigner, error) {
	if cfg == nil {
		return nil, nil, fmt.Errorf("config cannot be nil")
	}

	var signer jwt.Signer
	var standardSigner *jwt.StandardSigner

	switch cfg.JWTSigningType {
	case JWTSigningTypeStandard:
		// Create StandardSigner without initial keys
		// Keys will be loaded when the HTTP server starts
		standardSigner = jwt.NewStandardSigner(cfg.JWTIssuer, cfg.JWTAudience, cfg.JWTExpiration, cfg.JwtNewKeyUseDelay)
		signer = standardSigner

		logger.Info("Created StandardSigner for JWT signing", "secretName", cfg.JwtSecretName)

	case JWTSigningTypeKMS:
		// Validate KMS key ID is provided
		if cfg.KMSKeyId == "" {
			return nil, nil, fmt.Errorf("KMS_KEY_ID required when JWT_SIGNING_TYPE is kms")
		}

		// Create KMS client
		kmsClient, err := aws.NewKMSClient(context.Background())
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create KMS client: %w", err)
		}

		// Parse encryption context from config if provided
		var encryptionContext map[string]string
		if cfg.KMSEncryptionContext != "" {
			if err := json.Unmarshal([]byte(cfg.KMSEncryptionContext), &encryptionContext); err != nil {
				return nil, nil, fmt.Errorf("failed to parse KMS encryption context: %w", err)
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
		// No StandardSigner for KMS

	default:
		return nil, nil, fmt.Errorf("unknown JWT signing type: %s", cfg.JWTSigningType)
	}

	return jwt.NewManager(signer, cfg.JWTRefreshEnable, cfg.JWTRefreshWindow, cfg.JWTRefreshHorizon), standardSigner, nil
}
