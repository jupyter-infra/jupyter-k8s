package authmiddleware

import (
	"errors"
	"fmt"
	"time"

	jwt5 "github.com/golang-jwt/jwt/v5"
)

// Common errors
var (
	ErrInvalidToken     = errors.New("invalid token")
	ErrTokenExpired     = errors.New("token expired")
	ErrInvalidSignature = errors.New("invalid token signature")
	ErrInvalidClaims    = errors.New("invalid token claims")
	ErrDomainMismatch   = errors.New("token domain mismatch")
)

// Claims type is defined in types.go

// JWTHandler exposes JWTManager interface to facilitate unit-testing
type JWTHandler interface {
	GenerateToken(user string, groups []string, uid string, extra map[string][]string, path string, domain string, tokenType string) (string, error)
	ValidateToken(tokenString string) (*Claims, error)
	RefreshToken(claims *Claims) (string, error)
	UpdateSkipRefreshToken(claims *Claims) (string, error)
	ShouldRefreshToken(claims *Claims) bool
}

// JWTManager handles JWT token creation and validation
type JWTManager struct {
	signingKey     []byte
	issuer         string
	audience       string
	expiration     time.Duration
	enableRefresh  bool
	refreshWindow  time.Duration
	refreshHorizon time.Duration
}

// NewJWTManager creates a new JWTManager
func NewJWTManager(cfg *Config) *JWTManager {
	return &JWTManager{
		signingKey:     []byte(cfg.JWTSigningKey),
		issuer:         cfg.JWTIssuer,
		audience:       cfg.JWTAudience,
		expiration:     cfg.JWTExpiration,
		enableRefresh:  cfg.JWTRefreshEnable,
		refreshWindow:  cfg.JWTRefreshWindow,
		refreshHorizon: cfg.JWTRefreshHorizon,
	}
}

// GenerateToken creates a new JWT token for the given user and groups
func (m *JWTManager) GenerateToken(
	username string,
	groups []string,
	uid string,
	extra map[string][]string,
	path string,
	domain string,
	tokenType string) (string, error) {
	now := time.Now().UTC()
	claims := &Claims{
		RegisteredClaims: jwt5.RegisteredClaims{
			ExpiresAt: jwt5.NewNumericDate(now.Add(m.expiration)),
			IssuedAt:  jwt5.NewNumericDate(now),
			NotBefore: jwt5.NewNumericDate(now),
			Issuer:    m.issuer,
			Audience:  []string{m.audience},
			Subject:   username,
		},
		User:        username,
		Groups:      groups,
		UID:         uid,
		Extra:       extra,
		Path:        path,
		Domain:      domain,
		TokenType:   tokenType,
		SkipRefresh: false,
	}

	token := jwt5.NewWithClaims(jwt5.SigningMethodHS256, claims)
	return token.SignedString(m.signingKey)
}

// ValidateToken validates and parses the token
func (m *JWTManager) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt5.ParseWithClaims(
		tokenString,
		&Claims{},
		func(t *jwt5.Token) (any, error) {
			if _, ok := t.Method.(*jwt5.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return m.signingKey, nil
		},
		jwt5.WithIssuer(m.issuer),
		jwt5.WithAudience(m.audience),
		jwt5.WithValidMethods([]string{"HS256"}),
		jwt5.WithLeeway(5*time.Second),
	)

	if err != nil {
		if errors.Is(err, jwt5.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		if errors.Is(err, jwt5.ErrTokenSignatureInvalid) {
			return nil, ErrInvalidSignature
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, ErrInvalidClaims
	}

	return claims, nil
}

// RefreshToken creates a new token with the same claims but a new expiry time
func (m *JWTManager) RefreshToken(claims *Claims) (string, error) {
	if claims == nil {
		return "", errors.New("claims cannot be nil")
	}

	// Create a new token with the same claims but new expiry
	// Preserve user, groups, path and domain from the original claims
	user := claims.User
	groups := claims.Groups
	uid := claims.UID
	extra := claims.Extra
	path := claims.Path
	domain := claims.Domain

	now := time.Now().UTC()
	claims.RegisteredClaims = jwt5.RegisteredClaims{
		ExpiresAt: jwt5.NewNumericDate(now.Add(m.expiration)),
		IssuedAt:  claims.IssuedAt, // keep the original issue date
		NotBefore: jwt5.NewNumericDate(now),
		Issuer:    m.issuer,
		Audience:  []string{m.audience},
		Subject:   user,
	}

	// Restore the original custom claims
	claims.User = user
	claims.Groups = groups
	claims.UID = uid
	claims.Extra = extra
	claims.Path = path
	claims.Domain = domain

	token := jwt5.NewWithClaims(jwt5.SigningMethodHS256, claims)
	return token.SignedString(m.signingKey)
}

// UpdateSkipRefreshToken creates a new token with the same claims but skipRefresh=false
func (m *JWTManager) UpdateSkipRefreshToken(claims *Claims) (string, error) {
	if claims == nil {
		return "", errors.New("claims cannot be nil")
	}
	claims.SkipRefresh = false
	token := jwt5.NewWithClaims(jwt5.SigningMethodHS256, claims)
	return token.SignedString(m.signingKey)
}

// ShouldRefreshToken determines if a token should be refreshed based on its expiration time
// and the manager's refresh window
func (m *JWTManager) ShouldRefreshToken(claims *Claims) bool {
	// abort if config does not allow cookie refresh
	if !m.enableRefresh {
		return false
	}

	// If claims or ExpiresAt is nil, we can't determine if refresh is needed
	if claims == nil || claims.ExpiresAt == nil || claims.SkipRefresh {
		return false
	}

	// Calculate remaining time until token expiry
	now := time.Now().UTC()
	expiryTime := claims.ExpiresAt.Time
	remainingTime := expiryTime.Sub(now)
	// If token is already expired, don't attempt to refresh it
	if remainingTime <= 0 {
		return false
	}

	// Do not refresh if not yet in refresh window (issued or refreshed recently)
	if remainingTime > m.refreshWindow {
		return false
	}

	// Calculate time since original issuance
	originalIssueTime := claims.IssuedAt.Time
	timeSinceOriginalIssuance := now.Sub(originalIssueTime)

	// Refresh only if within the
	return timeSinceOriginalIssuance < m.refreshHorizon
}
