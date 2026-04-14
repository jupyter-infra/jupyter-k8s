/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

// Package jwt provides JWT token management with pluggable signing strategies.
package jwt

import (
	"errors"
	"fmt"
	"time"
)

// Handler combines signing and token lifecycle management
type Handler interface {
	GenerateToken(user string, groups []string, uid string, extra map[string][]string, path string, domain string, tokenType string) (string, error)
	ValidateToken(tokenString string) (*Claims, error)
	RefreshToken(claims *Claims) (string, error)
	UpdateSkipRefreshToken(claims *Claims) (string, error)
	ShouldRefreshToken(claims *Claims) bool
}

// Manager implements Handler with an embedded signer
type Manager struct {
	signer         Signer
	enableRefresh  bool
	refreshWindow  time.Duration
	refreshHorizon time.Duration
}

// NewManager creates a new Manager
func NewManager(signer Signer, enableRefresh bool, refreshWindow time.Duration, refreshHorizon time.Duration) *Manager {
	return &Manager{
		signer:         signer,
		enableRefresh:  enableRefresh,
		refreshWindow:  refreshWindow,
		refreshHorizon: refreshHorizon,
	}
}

// GenerateToken delegates to the signer
func (m *Manager) GenerateToken(
	user string,
	groups []string,
	uid string,
	extra map[string][]string,
	path string,
	domain string,
	tokenType string,
) (string, error) {
	return m.signer.GenerateToken(user, groups, uid, extra, path, domain, tokenType, false)
}

// ValidateToken delegates to the signer
func (m *Manager) ValidateToken(tokenString string) (*Claims, error) {
	return m.signer.ValidateToken(tokenString)
}

// RefreshToken creates a new token preserving the original IssuedAt for horizon tracking.
// Returns an error if the token is beyond the refresh horizon, forcing re-authentication.
func (m *Manager) RefreshToken(claims *Claims) (string, error) {
	if claims == nil {
		return "", errors.New("claims cannot be nil")
	}

	now := time.Now().UTC()
	timeSinceOriginalIssuance := now.Sub(claims.IssuedAt.Time)

	if timeSinceOriginalIssuance >= m.refreshHorizon {
		return "", fmt.Errorf("token beyond refresh horizon (%v since issuance, limit %v)", timeSinceOriginalIssuance, m.refreshHorizon)
	}

	// Within horizon: refresh preserving the original IssuedAt
	return m.signer.GenerateRefreshToken(claims)
}

// UpdateSkipRefreshToken creates a new token with skipRefresh=true.
// Used when the access review fails during refresh — stops further refresh attempts.
func (m *Manager) UpdateSkipRefreshToken(claims *Claims) (string, error) {
	if claims == nil {
		return "", errors.New("claims cannot be nil")
	}

	return m.signer.GenerateToken(
		claims.User, claims.Groups, claims.UID, claims.Extra,
		claims.Path, claims.Domain, claims.TokenType, true,
	)
}

// ShouldRefreshToken determines if a token should be refreshed.
// Returns true when the token is within the refresh window (approaching expiry)
// and not already marked as skip-refresh.
// The refresh horizon is enforced by RefreshToken, not here — so tokens beyond the
// horizon still get one final refresh with SkipRefresh=true.
func (m *Manager) ShouldRefreshToken(claims *Claims) bool {
	if !m.enableRefresh {
		return false
	}

	if claims == nil || claims.ExpiresAt == nil || claims.IssuedAt == nil || claims.SkipRefresh {
		return false
	}

	now := time.Now().UTC()
	expiryTime := claims.ExpiresAt.Time
	remainingTime := expiryTime.Sub(now)

	return remainingTime > 0 && remainingTime <= m.refreshWindow
}
