/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package jwt

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
)

func TestNewSecretInitializer(t *testing.T) {
	signer := NewStandardSigner("issuer", "audience", 0, 0)
	logger := logr.Discard()

	init := NewSecretInitializer(signer, "my-secret", "my-ns", logger)

	assert.NotNil(t, init)
	assert.Equal(t, signer, init.signer)
	assert.Equal(t, "my-secret", init.secretName)
	assert.Equal(t, "my-ns", init.namespace)
}

func TestSecretInitializer_NeedLeaderElection(t *testing.T) {
	init := NewSecretInitializer(nil, "", "", logr.Discard())
	assert.False(t, init.NeedLeaderElection())
}
