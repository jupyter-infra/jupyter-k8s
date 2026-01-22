/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package jwt

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	jwt5 "github.com/golang-jwt/jwt/v5"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// StandardSigner handles JWT token creation and validation using HMAC
// Supports multiple signing keys for key rotation
type StandardSigner struct {
	signingKeys    map[string][]byte    // map[kid]key
	keyAddedTimes  map[string]time.Time // map[kid]timestamp when key was added
	latestKid      string               // newest key ID for signing
	newKeyUseDelay time.Duration        // cooloff period before using a new key
	issuer         string
	audience       string
	expiration     time.Duration
	mu             sync.RWMutex // protect key map, keyAddedTimes, and latestKid
}

// NewStandardSigner creates a new StandardSigner without initial keys.
// Keys must be loaded by calling RetrieveInitialSecret() before use.
func NewStandardSigner(issuer string, audience string, expiration time.Duration, newKeyUseDelay time.Duration) *StandardSigner {
	return &StandardSigner{
		signingKeys:    make(map[string][]byte),
		keyAddedTimes:  make(map[string]time.Time),
		latestKid:      "",
		newKeyUseDelay: newKeyUseDelay,
		issuer:         issuer,
		audience:       audience,
		expiration:     expiration,
	}
}

// getLatestKidAndKeyWithCoolOff returns the latest key ID and signing key that have passed the cooloff period
// Returns empty kid and nil key if no key is beyond the cooloff period
// This combines kid lookup and key retrieval in a single lock to avoid double locking
func (s *StandardSigner) getLatestKidAndKeyWithCoolOff() (string, []byte) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	var usableKid string

	for kid, addedTime := range s.keyAddedTimes {
		timeSinceAdded := now.Sub(addedTime)
		if timeSinceAdded >= s.newKeyUseDelay {
			// This key is beyond cooloff, check if it's the latest usable one
			if usableKid == "" || kid > usableKid {
				usableKid = kid
			}
		}
	}

	if usableKid == "" {
		return "", nil
	}

	return usableKid, s.signingKeys[usableKid]
}

// GenerateToken creates a new JWT token for the given user and groups
// Uses the latest signing key that has passed the cooloff period (newKeyUseDelay)
// This ensures all pods have time to receive new keys via watch before they're used for signing
func (s *StandardSigner) GenerateToken(
	username string,
	groups []string,
	uid string,
	extra map[string][]string,
	path string,
	domain string,
	tokenType string) (string, error) {
	usableKid, signingKey := s.getLatestKidAndKeyWithCoolOff()
	if usableKid == "" || signingKey == nil {
		return "", fmt.Errorf("no signing key available beyond cooloff period (%v)", s.newKeyUseDelay)
	}

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

	// Use HS384 and add kid to header
	token := jwt5.NewWithClaims(jwt5.SigningMethodHS384, claims)
	token.Header["kid"] = usableKid

	return token.SignedString(signingKey)
}

// ValidateToken validates and parses the token
// Requires kid header and validates using the corresponding key
func (s *StandardSigner) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt5.ParseWithClaims(
		tokenString,
		&Claims{},
		func(t *jwt5.Token) (any, error) {
			// Verify algorithm is HMAC
			if _, ok := t.Method.(*jwt5.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}

			// Enforce HS384 only (AWS security requirement)
			if t.Method.Alg() != "HS384" {
				return nil, fmt.Errorf("unexpected algorithm: %v, expected HS384", t.Method.Alg())
			}

			// Extract and validate kid from header
			kid, ok := t.Header["kid"].(string)
			if !ok || kid == "" {
				return nil, fmt.Errorf("missing or invalid kid in token header")
			}

			// Lookup key by kid
			s.mu.RLock()
			key := s.signingKeys[kid]
			s.mu.RUnlock()

			if key == nil {
				return nil, fmt.Errorf("unknown key ID: %s", kid)
			}

			return key, nil
		},
		jwt5.WithIssuer(s.issuer),
		jwt5.WithAudience(s.audience),
		jwt5.WithValidMethods([]string{"HS384"}),
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

// UpdateKeys atomically updates the signing keys
// This is called when the secret watcher detects changes
func (s *StandardSigner) UpdateKeys(signingKeys map[string][]byte, latestKid string) error {
	if len(signingKeys) == 0 {
		return fmt.Errorf("signingKeys cannot be empty")
	}
	if _, ok := signingKeys[latestKid]; !ok {
		return fmt.Errorf("latestKid %s not found in signingKeys", latestKid)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Track timestamps for new keys
	now := time.Now()
	newKeyAddedTimes := make(map[string]time.Time)

	for kid := range signingKeys {
		if oldTime, exists := s.keyAddedTimes[kid]; exists {
			// Key already existed, preserve its original timestamp
			newKeyAddedTimes[kid] = oldTime
		} else {
			// New key, record current time
			newKeyAddedTimes[kid] = now
		}
	}

	s.signingKeys = signingKeys
	s.keyAddedTimes = newKeyAddedTimes
	s.latestKid = latestKid

	return nil
}

// RetrieveInitialSecret loads the initial JWT signing keys from the Kubernetes secret.
// This is called when the HTTP server starts to ensure keys are loaded before accepting requests.
func (s *StandardSigner) RetrieveInitialSecret(
	ctx context.Context,
	runtimeClient client.Client,
	secretName string,
	namespace string,
) error {
	// Get secret
	secret := &corev1.Secret{}
	err := runtimeClient.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: namespace,
	}, secret)
	if err != nil {
		return fmt.Errorf("failed to get JWT signing secret %s: %w", secretName, err)
	}

	// Parse signing keys from secret
	signingKeys, latestKid, err := ParseSigningKeysFromSecret(secret)
	if err != nil {
		return fmt.Errorf("failed to parse signing keys from secret: %w", err)
	}

	// Update signer with initial keys
	if err := s.UpdateKeys(signingKeys, latestKid); err != nil {
		return fmt.Errorf("failed to update signing keys: %w", err)
	}

	return nil
}
