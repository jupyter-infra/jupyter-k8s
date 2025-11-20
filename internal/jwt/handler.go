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

// Package jwt provides JWT token management with pluggable signing strategies.
// It supports both standard HMAC signing and AWS KMS-based signing through
// a common Handler interface.
package jwt

import (
	"errors"
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
	return m.signer.GenerateToken(user, groups, uid, extra, path, domain, tokenType)
}

// ValidateToken delegates to the signer
func (m *Manager) ValidateToken(tokenString string) (*Claims, error) {
	return m.signer.ValidateToken(tokenString)
}

// RefreshToken creates a new token with the same claims
func (m *Manager) RefreshToken(claims *Claims) (string, error) {
	if claims == nil {
		return "", errors.New("claims cannot be nil")
	}

	return m.signer.GenerateToken(
		claims.User,
		claims.Groups,
		claims.UID,
		claims.Extra,
		claims.Path,
		claims.Domain,
		claims.TokenType,
	)
}

// UpdateSkipRefreshToken creates a new token with skipRefresh=true
func (m *Manager) UpdateSkipRefreshToken(claims *Claims) (string, error) {
	if claims == nil {
		return "", errors.New("claims cannot be nil")
	}

	claims.SkipRefresh = true
	return m.signer.GenerateToken(
		claims.User,
		claims.Groups,
		claims.UID,
		claims.Extra,
		claims.Path,
		claims.Domain,
		claims.TokenType,
	)
}

// ShouldRefreshToken determines if a token should be refreshed
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

	if remainingTime <= 0 {
		return false
	}

	if remainingTime > m.refreshWindow {
		return false
	}

	originalIssueTime := claims.IssuedAt.Time
	timeSinceOriginalIssuance := now.Sub(originalIssueTime)

	return timeSinceOriginalIssuance < m.refreshHorizon
}
