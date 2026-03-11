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

func newTestFactory(label string) *StandardSignerFactory {
	signer := NewStandardSigner("issuer-"+label, "audience-"+label, 5*time.Minute, 0)
	_ = signer.UpdateKeys(map[string][]byte{
		"kid-" + label: []byte("test-signing-key-at-least-48-bytes-long-for-hs384!"),
	}, "kid-"+label)
	return NewStandardSignerFactory(signer)
}

func TestCompositeSignerFactory_NilAccessStrategy(t *testing.T) {
	defaultFactory := newTestFactory("default")
	composite := NewCompositeSignerFactory(map[string]SignerFactory{
		"k8s-native": defaultFactory,
	}, defaultFactory)

	signer, err := composite.CreateSigner(nil)

	require.NoError(t, err)
	assert.NotNil(t, signer)
}

func TestCompositeSignerFactory_EmptyHandler(t *testing.T) {
	defaultFactory := newTestFactory("default")
	composite := NewCompositeSignerFactory(map[string]SignerFactory{
		"k8s-native": defaultFactory,
	}, defaultFactory)

	signer, err := composite.CreateSigner(&workspacev1alpha1.WorkspaceAccessStrategy{
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			CreateConnectionHandler: "",
		},
	})

	require.NoError(t, err)
	assert.NotNil(t, signer)
}

func TestCompositeSignerFactory_RoutesToRegisteredFactory(t *testing.T) {
	nativeFactory := newTestFactory("native")
	composite := NewCompositeSignerFactory(map[string]SignerFactory{
		"k8s-native": nativeFactory,
	}, nativeFactory)

	signer, err := composite.CreateSigner(&workspacev1alpha1.WorkspaceAccessStrategy{
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			CreateConnectionHandler: "k8s-native",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, nativeFactory.Signer(), signer)
}

func TestCompositeSignerFactory_DefaultFactoryUsedForEmptyHandler(t *testing.T) {
	defaultFactory := newTestFactory("default")
	otherFactory := newTestFactory("other")
	composite := NewCompositeSignerFactory(map[string]SignerFactory{
		"k8s-native": otherFactory,
	}, defaultFactory)

	// Empty handler uses defaultFactory, not the map
	signer, err := composite.CreateSigner(&workspacev1alpha1.WorkspaceAccessStrategy{
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			CreateConnectionHandler: "",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, defaultFactory.Signer(), signer)
}

func TestCompositeSignerFactory_UnsupportedHandler(t *testing.T) {
	composite := NewCompositeSignerFactory(map[string]SignerFactory{
		"k8s-native": newTestFactory("native"),
	}, newTestFactory("default"))

	signer, err := composite.CreateSigner(&workspacev1alpha1.WorkspaceAccessStrategy{
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			CreateConnectionHandler: "nonexistent",
		},
	})

	assert.Error(t, err)
	assert.Nil(t, signer)
	assert.Contains(t, err.Error(), "unsupported connection handler")
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestCompositeSignerFactory_ValidateToken_Success(t *testing.T) {
	factory := newTestFactory("native")
	composite := NewCompositeSignerFactory(map[string]SignerFactory{
		"k8s-native": factory,
	}, factory)

	// Generate a token using the signer
	token, err := factory.Signer().GenerateToken("alice", []string{"team-a"}, "alice-uid", nil, "/workspaces/default/ws", "example.com", TokenTypeBootstrap)
	require.NoError(t, err)

	// Validate through composite
	claims, err := composite.ValidateToken(token)
	require.NoError(t, err)
	assert.Equal(t, "alice", claims.User)
	assert.Equal(t, "/workspaces/default/ws", claims.Path)
	assert.Equal(t, "example.com", claims.Domain)
	assert.Equal(t, TokenTypeBootstrap, claims.TokenType)
}

func TestCompositeSignerFactory_ValidateToken_InvalidToken(t *testing.T) {
	factory := newTestFactory("native")
	composite := NewCompositeSignerFactory(map[string]SignerFactory{
		"k8s-native": factory,
	}, factory)

	_, err := composite.ValidateToken("invalid.token.string")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "token validation failed")
}

func TestCompositeSignerFactory_ValidateToken_NoFactories(t *testing.T) {
	composite := NewCompositeSignerFactory(map[string]SignerFactory{}, newTestFactory("default"))

	_, err := composite.ValidateToken("some-token")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no signer factories configured")
}

func TestCompositeSignerFactory_ValidateToken_MultipleFactories(t *testing.T) {
	// Token signed by factory2, but factory1 can't validate it
	factory1 := newTestFactory("first")
	factory2 := newTestFactory("second")
	composite := NewCompositeSignerFactory(map[string]SignerFactory{
		"first":  factory1,
		"second": factory2,
	}, factory1)

	token, err := factory2.Signer().GenerateToken("bob", nil, "bob", nil, "/ws", "test.com", TokenTypeBootstrap)
	require.NoError(t, err)

	claims, err := composite.ValidateToken(token)
	require.NoError(t, err)
	assert.Equal(t, "bob", claims.User)
}

// erroringSignerFactory is a mock factory whose CreateSigner always returns an error
type erroringSignerFactory struct{}

func (e *erroringSignerFactory) CreateSigner(_ *workspacev1alpha1.WorkspaceAccessStrategy) (Signer, error) {
	return nil, assert.AnError
}

func TestCompositeSignerFactory_ValidateToken_CreateSignerError(t *testing.T) {
	// One factory errors on CreateSigner, the other succeeds
	goodFactory := newTestFactory("good")
	composite := NewCompositeSignerFactory(map[string]SignerFactory{
		"broken": &erroringSignerFactory{},
		"good":   goodFactory,
	}, goodFactory)

	token, err := goodFactory.Signer().GenerateToken("alice", nil, "uid", nil, "/ws", "d.com", TokenTypeBootstrap)
	require.NoError(t, err)

	claims, err := composite.ValidateToken(token)
	require.NoError(t, err)
	assert.Equal(t, "alice", claims.User)
}

func TestCompositeSignerFactory_ValidateToken_AllCreateSignerError(t *testing.T) {
	// All factories error on CreateSigner → validation fails
	composite := NewCompositeSignerFactory(map[string]SignerFactory{
		"broken1": &erroringSignerFactory{},
		"broken2": &erroringSignerFactory{},
	}, &erroringSignerFactory{})

	_, err := composite.ValidateToken("some-token")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "token validation failed")
}

func TestCompositeSignerFactory_GetFactory(t *testing.T) {
	nativeFactory := newTestFactory("native")
	composite := NewCompositeSignerFactory(map[string]SignerFactory{
		"k8s-native": nativeFactory,
	}, nativeFactory)

	f, ok := composite.GetFactory("k8s-native")
	assert.True(t, ok)
	assert.Equal(t, nativeFactory, f)

	_, ok = composite.GetFactory("nonexistent")
	assert.False(t, ok)
}
