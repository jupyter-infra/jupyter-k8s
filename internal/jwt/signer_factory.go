package jwt

import (
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// SignerFactory creates JWT signers based on access strategy configuration
type SignerFactory interface {
	CreateSigner(accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) (Signer, error)
}
