package extensionapi

import (
	"crypto/x509"
	"fmt"
	"net/http"

	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/request/headerrequest"
)

// AuthConfig holds authentication configuration for extension API server
type AuthConfig struct {
	ClientCA      *x509.Certificate     // CA for verifying client certificates
	AllowedNames  []string              // Allowed client certificate names
	authenticator authenticator.Request // Standard Kubernetes authenticator
}

// UserInfo represents authenticated user information
type UserInfo struct {
	Username string   `json:"username"`
	Groups   []string `json:"groups"`
}

// InitializeAuthenticator sets up standard Kubernetes authenticator
func (a *AuthConfig) InitializeAuthenticator() error {
	if a == nil {
		return fmt.Errorf("AuthConfig is nil")
	}

	var err error
	a.authenticator, err = headerrequest.New(
		[]string{HeaderRemoteUser},  // Username headers
		nil,                         // UID headers (not used)
		[]string{HeaderRemoteGroup}, // Group headers
		[]string{ExtraHeaderPrefix}, // Extra header prefixes
	)
	if err != nil {
		return fmt.Errorf("failed to create authenticator: %w", err)
	}

	return nil
}

// AuthenticateRequest validates request using Kubernetes authentication headers
func (a *AuthConfig) AuthenticateRequest(r *http.Request) (*UserInfo, error) {
	if a == nil {
		return nil, fmt.Errorf("AuthConfig is nil")
	}
	if a.authenticator == nil {
		return nil, fmt.Errorf("authenticator not initialized")
	}
	if r == nil {
		return nil, fmt.Errorf("request is nil")
	}

	// Use standard Kubernetes authentication
	response, ok, err := a.authenticator.AuthenticateRequest(r)
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("authentication failed: no valid credentials")
	}
	if response == nil || response.User == nil {
		return nil, fmt.Errorf("authentication failed: invalid response")
	}

	// Convert to our UserInfo format
	return &UserInfo{
		Username: response.User.GetName(),
		Groups:   response.User.GetGroups(),
	}, nil
}

// IsAllowedClientName checks if certificate Common Name is allowed
func (a *AuthConfig) IsAllowedClientName(commonName string) bool {
	if a == nil || commonName == "" {
		return false
	}

	for _, allowed := range a.AllowedNames {
		if commonName == allowed {
			return true
		}
	}
	return false
}
