package authmiddleware

import (
	jwt5 "github.com/golang-jwt/jwt/v5"
)

// Claims represents the JWT claims for our auth token
type Claims struct {
	jwt5.RegisteredClaims
	User   string   `json:"user,omitempty"`
	Groups []string `json:"groups,omitempty"`
	Path   string   `json:"path,omitempty"`
	Domain string   `json:"domain,omitempty"`
}
