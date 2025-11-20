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

package authmiddleware

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jupyter-infra/jupyter-k8s/internal/aws"
	"github.com/jupyter-infra/jupyter-k8s/internal/jwt"
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
