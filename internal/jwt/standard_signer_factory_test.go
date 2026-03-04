/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package jwt

import (
	"testing"
	"time"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStandardSignerFactory() (*StandardSignerFactory, *StandardSigner) {
	signer := NewStandardSigner("test-issuer", "test-audience", 5*time.Minute, 0)
	_ = signer.UpdateKeys(map[string][]byte{
		"1234567890": []byte("test-signing-key-at-least-48-bytes-long-for-hs384!"),
	}, "1234567890")
	factory := NewStandardSignerFactory(signer)
	return factory, signer
}

func TestNewStandardSignerFactory(t *testing.T) {
	factory, signer := newTestStandardSignerFactory()

	assert.NotNil(t, factory)
	assert.Equal(t, signer, factory.signer)
}

func TestStandardSignerFactory_CreateSigner_NilAccessStrategy(t *testing.T) {
	factory, signer := newTestStandardSignerFactory()

	result, err := factory.CreateSigner(nil)

	assert.NoError(t, err)
	assert.Equal(t, signer, result)
}

func TestStandardSignerFactory_CreateSigner_EmptyHandler(t *testing.T) {
	factory, signer := newTestStandardSignerFactory()

	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			CreateConnectionHandler: "",
		},
	}

	result, err := factory.CreateSigner(accessStrategy)

	assert.NoError(t, err)
	assert.Equal(t, signer, result)
}

func TestStandardSignerFactory_CreateSigner_K8sNativeHandler(t *testing.T) {
	factory, signer := newTestStandardSignerFactory()

	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			CreateConnectionHandler: "k8s-native",
		},
	}

	result, err := factory.CreateSigner(accessStrategy)

	assert.NoError(t, err)
	assert.Equal(t, signer, result)
}

func TestStandardSignerFactory_CreateSigner_AWSHandler(t *testing.T) {
	factory, _ := newTestStandardSignerFactory()

	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			CreateConnectionHandler: "aws",
		},
	}

	result, err := factory.CreateSigner(accessStrategy)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "aws")
}

func TestStandardSignerFactory_CreateSigner_UnsupportedHandler(t *testing.T) {
	factory, _ := newTestStandardSignerFactory()

	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			CreateConnectionHandler: "invalid",
		},
	}

	result, err := factory.CreateSigner(accessStrategy)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "unsupported connection handler")
}

func TestStandardSignerFactory_CreateSigner_GenerateValidateRoundTrip(t *testing.T) {
	factory, _ := newTestStandardSignerFactory()

	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			CreateConnectionHandler: "k8s-native",
		},
	}

	signer, err := factory.CreateSigner(accessStrategy)
	require.NoError(t, err)

	token, err := signer.GenerateToken("testuser", []string{"group1"}, "uid1", nil, "/path", "example.com", TokenTypeBootstrap)
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	claims, err := signer.ValidateToken(token)
	require.NoError(t, err)
	assert.Equal(t, "testuser", claims.User)
	assert.Equal(t, "/path", claims.Path)
	assert.Equal(t, "example.com", claims.Domain)
	assert.Equal(t, TokenTypeBootstrap, claims.TokenType)
}
