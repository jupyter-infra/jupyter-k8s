/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package authmiddleware

import (
	"fmt"

	"github.com/go-logr/logr"

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
	case JWTSigningTypeStandard, "":
		// Create StandardSigner without initial keys
		// Keys will be loaded when the HTTP server starts
		standardSigner = jwt.NewStandardSigner(cfg.JWTIssuer, cfg.JWTAudience, cfg.JWTExpiration, cfg.JwtNewKeyUseDelay)
		signer = standardSigner

		logger.Info("Created StandardSigner for JWT signing", "secretName", cfg.JwtSecretName)

	default:
		return nil, nil, fmt.Errorf("unsupported JWT signing type %q", cfg.JWTSigningType)
	}

	return jwt.NewManager(signer, cfg.JWTRefreshEnable, cfg.JWTRefreshWindow, cfg.JWTRefreshHorizon), standardSigner, nil
}
