package authmiddleware

import (
	"testing"
	"time"
)

func TestNewJWTHandler_StandardSigning(t *testing.T) {
	cfg := &Config{
		JWTSigningType:    JWTSigningTypeStandard,
		JWTSigningKey:     "test-signing-key-32-characters-long",
		JWTIssuer:         "test-issuer",
		JWTAudience:       "test-audience",
		JWTExpiration:     time.Hour,
		JWTRefreshEnable:  false,
		JWTRefreshWindow:  0,
		JWTRefreshHorizon: 0,
	}

	handler, err := NewJWTHandler(cfg)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if handler == nil {
		t.Fatal("Expected handler, got nil")
	}

	// Test that handler can generate tokens
	token, err := handler.GenerateToken("testuser", []string{"group1"}, "uid", nil, "/path", "domain", "session")
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}
	if token == "" {
		t.Fatal("Expected non-empty token")
	}
}

func TestNewJWTHandler_StandardSigning_MissingKey(t *testing.T) {
	cfg := &Config{
		JWTSigningType: JWTSigningTypeStandard,
		JWTSigningKey:  "", // Missing key - but this is validated in config.go, not factory
		JWTIssuer:      "test-issuer",
		JWTAudience:    "test-audience",
		JWTExpiration:  time.Hour,
	}

	// This should succeed because config validation happens elsewhere
	// The factory just uses whatever is in the config
	handler, err := NewJWTHandler(cfg)
	if err != nil {
		t.Fatalf("Expected no error from factory, got %v", err)
	}
	if handler == nil {
		t.Fatal("Expected handler, got nil")
	}
}

func TestNewJWTHandler_KMSSigning_MissingKeyId(t *testing.T) {
	cfg := &Config{
		JWTSigningType: JWTSigningTypeKMS,
		KMSKeyId:       "", // Missing KMS key ID
		JWTIssuer:      "test-issuer",
		JWTAudience:    "test-audience",
		JWTExpiration:  time.Hour,
	}

	_, err := NewJWTHandler(cfg)
	if err == nil {
		t.Fatal("Expected error for missing KMS key ID")
	}
	if err.Error() != "KMS_KEY_ID required when JWT_SIGNING_TYPE is kms" {
		t.Errorf("Expected specific error message, got %v", err)
	}
}

func TestNewJWTHandler_InvalidSigningType(t *testing.T) {
	cfg := &Config{
		JWTSigningType: "invalid-type",
		JWTIssuer:      "test-issuer",
		JWTAudience:    "test-audience",
		JWTExpiration:  time.Hour,
	}

	_, err := NewJWTHandler(cfg)
	if err == nil {
		t.Fatal("Expected error for invalid signing type")
	}
	if err.Error() != "unknown JWT signing type: invalid-type" {
		t.Errorf("Expected specific error message, got %v", err)
	}
}

func TestNewJWTHandler_NilConfig(t *testing.T) {
	_, err := NewJWTHandler(nil)
	if err == nil {
		t.Fatal("Expected error for nil config")
	}
	if err.Error() != "config cannot be nil" {
		t.Errorf("Expected 'config cannot be nil', got %v", err)
	}
}

// Note: KMS signing success test would require mocking AWS KMS client
// This is intentionally omitted as it would require significant test infrastructure
// and the KMS client creation is already tested in the aws package
