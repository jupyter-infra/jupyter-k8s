package jwt

import (
	"errors"
	"fmt"
	"time"

	jwt5 "github.com/golang-jwt/jwt/v5"
)

// StandardSigner handles JWT token creation and validation using HMAC
type StandardSigner struct {
	signingKey []byte
	issuer     string
	audience   string
	expiration time.Duration
}

// NewStandardSigner creates a new StandardSigner
func NewStandardSigner(signingKey string, issuer string, audience string, expiration time.Duration) *StandardSigner {
	return &StandardSigner{
		signingKey: []byte(signingKey),
		issuer:     issuer,
		audience:   audience,
		expiration: expiration,
	}
}

// GenerateToken creates a new JWT token for the given user and groups
func (s *StandardSigner) GenerateToken(
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
			ExpiresAt: jwt5.NewNumericDate(now.Add(s.expiration)),
			IssuedAt:  jwt5.NewNumericDate(now),
			NotBefore: jwt5.NewNumericDate(now),
			Issuer:    s.issuer,
			Audience:  []string{s.audience},
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
	return token.SignedString(s.signingKey)
}

// ValidateToken validates and parses the token
func (s *StandardSigner) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt5.ParseWithClaims(
		tokenString,
		&Claims{},
		func(t *jwt5.Token) (any, error) {
			if _, ok := t.Method.(*jwt5.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return s.signingKey, nil
		},
		jwt5.WithIssuer(s.issuer),
		jwt5.WithAudience(s.audience),
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
