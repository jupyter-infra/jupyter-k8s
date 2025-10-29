package authmiddleware

import (
	jwt5 "github.com/golang-jwt/jwt/v5"
)

// TokenType constants define the different types of authentication tokens used in the system.
const (
	// TokenTypeBootstrap represents a bootstrap token used for initial session setup. These are
	// short-lived and exchanged by the service for a session token.
	TokenTypeBootstrap = "bootstrap"
	// TokenTypeSession represents a session token used for ongoing authenticated requests
	TokenTypeSession = "session"
)

// Claims represents the JWT claims for our auth token
type Claims struct {
	jwt5.RegisteredClaims
	User        string              `json:"user,omitempty"`
	Groups      []string            `json:"groups,omitempty"`
	UID         string              `json:"uid,omitempty"`
	Extra       map[string][]string `json:"extra,omitempty"`
	Path        string              `json:"path,omitempty"`
	Domain      string              `json:"domain,omitempty"`
	TokenType   string              `json:"token_type,omitempty"`
	SkipRefresh bool                `json:"skipRefresh,omitempty"`
}
