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
