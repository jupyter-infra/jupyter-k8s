/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package jwt

import (
	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

// SignerFactory creates JWT signers based on access strategy configuration
type SignerFactory interface {
	CreateSigner(accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) (Signer, error)
}
