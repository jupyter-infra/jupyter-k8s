package extensionapi

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// loadAuthConfigFromConfigMap loads auth config from kube-system ConfigMap
func loadAuthConfigFromConfigMap(ctx context.Context, k8sClient client.Client) (*AuthConfig, error) {
	// Get the ConfigMap
	configMap := &corev1.ConfigMap{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      AuthConfigMapName,
		Namespace: AuthConfigMapNamespace,
	}, configMap)
	if err != nil {
		return nil, fmt.Errorf("failed to get authentication ConfigMap: %w", err)
	}

	// Extract client CA certificate
	clientCAData, ok := configMap.Data[RequestHeaderClientCAFileKey]
	if !ok {
		return nil, fmt.Errorf("missing %s in ConfigMap", RequestHeaderClientCAFileKey)
	}

	clientCA, err := parseCertificate(clientCAData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse client CA certificate: %w", err)
	}

	// Extract allowed names
	allowedNamesData, ok := configMap.Data[RequestHeaderAllowedNamesKey]
	if !ok {
		return nil, fmt.Errorf("missing %s in ConfigMap", RequestHeaderAllowedNamesKey)
	}

	var allowedNames []string
	if err := json.Unmarshal([]byte(allowedNamesData), &allowedNames); err != nil {
		return nil, fmt.Errorf("failed to parse allowed names: %w", err)
	}

	return &AuthConfig{
		ClientCA:     clientCA,
		AllowedNames: allowedNames,
	}, nil
}

// parseCertificate parses PEM-encoded certificate
func parseCertificate(certData string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(certData))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM certificate")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	return cert, nil
}
