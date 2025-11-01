package jwt

// JWTSigner handles core JWT operations - encryption-specific
type JWTSigner interface {
	GenerateToken(user string, groups []string, uid string, extra map[string][]string, path string, domain string, tokenType string) (string, error)
	ValidateToken(tokenString string) (*Claims, error)
}
