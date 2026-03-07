/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package jwt

import (
	"fmt"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// StandardSignerFactory creates JWT signers using a shared StandardSigner backed by K8s Secrets.
// Unlike AWSSignerFactory which creates per-strategy signers, this factory reuses a single
// StandardSigner instance since all k8s-native strategies share the same Secret-based keys.
type StandardSignerFactory struct {
	signer *StandardSigner
}

// NewStandardSignerFactory creates a new factory wrapping a shared StandardSigner.
func NewStandardSignerFactory(signer *StandardSigner) *StandardSignerFactory {
	return &StandardSignerFactory{
		signer: signer,
	}
}

// Signer returns the underlying StandardSigner for direct access (e.g. key updates).
func (f *StandardSignerFactory) Signer() *StandardSigner {
	return f.signer
}

// CreateSigner returns the shared StandardSigner for compatible access strategies.
// Accepts "" (auto/default) and "k8s-native" handlers. Rejects "aws" since that
// requires AWSSignerFactory.
func (f *StandardSignerFactory) CreateSigner(accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) (Signer, error) {
	if accessStrategy == nil {
		return f.signer, nil
	}

	handler := accessStrategy.Spec.CreateConnectionHandler
	switch handler {
	case "", "k8s-native":
		return f.signer, nil
	case "aws":
		return nil, fmt.Errorf("access strategy requires \"aws\" handler, but only \"k8s-native\" signing is configured")
	default:
		return nil, fmt.Errorf("unsupported connection handler: %s", handler)
	}
}
