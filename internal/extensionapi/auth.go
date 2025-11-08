// Package extensionapi provides authentication using official Kubernetes patterns.
package extensionapi

import (
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/server/options"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// AuthConfig holds the authentication configuration
type AuthConfig struct {
	authenticator authenticator.Request
	client        kubernetes.Interface
}

// NewAuthConfig creates a new authentication configuration
func NewAuthConfig(client kubernetes.Interface) *AuthConfig {
	return &AuthConfig{
		client: client,
	}
}

// InitializeAuthenticator initializes the authenticator using official Kubernetes patterns
func (a *AuthConfig) InitializeAuthenticator() error {
	setupLog := log.Log.WithName("extension-api-auth")

	// Create delegating authentication options (same as kube-apiserver)
	authOptions := options.NewDelegatingAuthenticationOptions()

	// Configure to use in-cluster lookup (reads extension-apiserver-authentication ConfigMap)
	authOptions.SkipInClusterLookup = false
	authOptions.TolerateInClusterLookupFailure = false
	authOptions.RemoteKubeConfigFile = "" // Use in-cluster config

	// Create authentication info and config
	authInfo := &server.AuthenticationInfo{}

	// Apply the options to create RequestHeaderConfig automatically
	err := authOptions.ApplyTo(authInfo, nil, nil)
	if err != nil {
		return err
	}

	// The authenticator is now set up in authInfo
	a.authenticator = authInfo.Authenticator

	setupLog.Info("Official Kubernetes authentication initialized successfully")
	return nil
}
