/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package jwt

// Signer handles core JWT operations - encryption-specific
type Signer interface {
	GenerateToken(user string, groups []string, uid string, extra map[string][]string, path string, domain string, tokenType string, skipRefresh bool) (string, error)
	GenerateRefreshToken(claims *Claims) (string, error)
	ValidateToken(tokenString string) (*Claims, error)
}
