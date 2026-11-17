/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package posixidentity

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
)

// Field indexes on the manager cache.
const (
	// IndexUsernameHash indexes PosixIdentity objects by spec.usernameHash
	IndexUsernameHash = "spec.usernameHash"
	// IndexWorkspaceClaimedUID indexes Workspaces by their posix-uid label
	IndexWorkspaceClaimedUID = "posixidentity.claimedUID"
	// IndexWorkspaceClaimOwner indexes Workspaces carrying a posix-uid label by their owner's
	// username hash
	IndexWorkspaceClaimOwner = "posixidentity.claimOwner"
	// IndexNamespaceGID indexes labelled Namespaces by their posix-gid label value
	IndexNamespaceGID = "posixidentity.namespaceGID"
)

// FieldIndexer is the subset of the manager's field indexer the indexes need.
type FieldIndexer interface {
	IndexField(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error
}

// SetupIndexes registers the field indexes the allocator and the namespace gid lookup read.
func SetupIndexes(ctx context.Context, indexer FieldIndexer) error {
	indexes := []struct {
		obj   client.Object
		field string
		fn    client.IndexerFunc
	}{
		{&workspacev1alpha1.PosixIdentity{}, IndexUsernameHash, indexUsernameHash},
		{&workspacev1alpha1.Workspace{}, IndexWorkspaceClaimedUID, indexWorkspaceClaimedUID},
		{&workspacev1alpha1.Workspace{}, IndexWorkspaceClaimOwner, indexWorkspaceClaimOwner},
		{&corev1.Namespace{}, IndexNamespaceGID, indexNamespaceGID},
	}
	for _, idx := range indexes {
		if err := indexer.IndexField(ctx, idx.obj, idx.field, idx.fn); err != nil {
			return fmt.Errorf("failed to index %s: %w", idx.field, err)
		}
	}
	return nil
}

func indexUsernameHash(obj client.Object) []string {
	identity, ok := obj.(*workspacev1alpha1.PosixIdentity)
	if !ok {
		return nil
	}
	return []string{identity.Spec.UsernameHash}
}

func indexWorkspaceClaimedUID(obj client.Object) []string {
	if uid := obj.GetLabels()[workspaceutil.LabelPosixUID]; uid != "" {
		return []string{uid}
	}
	return nil
}

func indexWorkspaceClaimOwner(obj client.Object) []string {
	owner := obj.GetAnnotations()[workspaceutil.AnnotationCreatedBy]
	if owner == "" || obj.GetLabels()[workspaceutil.LabelPosixUID] == "" {
		return nil
	}
	return []string{UsernameHash(owner)}
}

func indexNamespaceGID(obj client.Object) []string {
	if gid := obj.GetLabels()[workspaceutil.LabelPosixGID]; gid != "" {
		return []string{gid}
	}
	return nil
}
