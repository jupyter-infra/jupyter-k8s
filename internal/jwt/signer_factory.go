/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package jwt

import (
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// SignerFactory creates JWT signers based on access strategy configuration
type SignerFactory interface {
	CreateSigner(accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) (Signer, error)
}

// TokenValidator validates JWT tokens across all configured signers.
// This is separate from SignerFactory because signing requires AccessStrategy context
// while validation must try all signers (the caller doesn't know which signer was used).
type TokenValidator interface {
	ValidateToken(tokenString string) (*Claims, error)
}
