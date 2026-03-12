/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

// Package extensionapi provides extension API server functionality.
package extensionapi

import (
	"encoding/json"
	"io"
	"net/http"

	connectionv1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/connection/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/jwt"
)

// handleBearerTokenReview handles POST requests to verify bearer tokens.
// Follows the K8s TokenReview pattern: accepts an opaque token, validates it,
// and returns the authenticated identity and claims.
func (s *ExtensionServer) handleBearerTokenReview(w http.ResponseWriter, r *http.Request) {
	logger := GetLoggerFromContext(r.Context())

	if r.Method != http.MethodPost {
		WriteError(w, http.StatusBadRequest, "BearerTokenReview must use POST method")
		return
	}

	logger.Info("Handling BearerTokenReview", "method", r.Method, "path", r.URL.Path)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error(err, "Failed to read request body")
		WriteError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	var review connectionv1alpha1.BearerTokenReview
	if err := json.Unmarshal(body, &review); err != nil {
		logger.Error(err, "Failed to unmarshal BearerTokenReview")
		WriteError(w, http.StatusBadRequest, "Invalid BearerTokenReview format")
		return
	}

	if review.Spec.Token == "" {
		writeBearerTokenReviewResponse(w, &review, false, nil, "token is required")
		return
	}

	// Validate the token using all configured signers
	claims, err := s.tokenValidator.ValidateToken(review.Spec.Token)
	if err != nil {
		logger.Info("Bearer token validation failed", "error", err)
		writeBearerTokenReviewResponse(w, &review, false, nil, "invalid or expired token")
		return
	}

	// Only accept bootstrap tokens (bearer tokens), not session tokens
	if claims.TokenType != jwt.TokenTypeBootstrap {
		logger.Info("Bearer token has wrong type", "expected", jwt.TokenTypeBootstrap, "actual", claims.TokenType)
		writeBearerTokenReviewResponse(w, &review, false, nil, "invalid token type")
		return
	}

	writeBearerTokenReviewResponse(w, &review, true, claims, "")
}

// writeBearerTokenReviewResponse writes the BearerTokenReview response
func writeBearerTokenReviewResponse(w http.ResponseWriter, review *connectionv1alpha1.BearerTokenReview, authenticated bool, claims *jwt.Claims, errMsg string) {
	review.Status = connectionv1alpha1.BearerTokenReviewStatus{
		Authenticated: authenticated,
		Error:         errMsg,
	}

	if authenticated && claims != nil {
		review.Status.Path = claims.Path
		review.Status.Domain = claims.Domain
		review.Status.User = connectionv1alpha1.BearerTokenReviewUser{
			Username: claims.User,
			UID:      claims.UID,
			Groups:   claims.Groups,
			Extra:    claims.Extra,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(review); err != nil {
		// Can't write error response since we already started writing
		return
	}
}
