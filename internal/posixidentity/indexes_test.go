/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package posixidentity

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
)

// recordingIndexer records the indexes registered on it.
type recordingIndexer struct{ fields map[string]client.IndexerFunc }

func (r *recordingIndexer) IndexField(_ context.Context, _ client.Object, field string, fn client.IndexerFunc) error {
	r.fields[field] = fn
	return nil
}

func TestSetupIndexes(t *testing.T) {
	indexer := &recordingIndexer{fields: map[string]client.IndexerFunc{}}
	if err := SetupIndexes(context.Background(), indexer); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{IndexUsernameHash, IndexWorkspaceClaimedUID, IndexWorkspaceClaimOwner, IndexNamespaceGID} {
		if indexer.fields[field] == nil {
			t.Errorf("index %s is not registered", field)
		}
	}

	unclaimed := &workspacev1alpha1.Workspace{ObjectMeta: metav1.ObjectMeta{
		Annotations: map[string]string{workspaceutil.AnnotationCreatedBy: alice},
	}}
	if got := indexer.fields[IndexWorkspaceClaimedUID](unclaimed); got != nil {
		t.Errorf("a workspace without a posix-uid label claims nothing: %v", got)
	}
	if got := indexer.fields[IndexWorkspaceClaimOwner](unclaimed); got != nil {
		t.Errorf("a workspace without a posix-uid label has no claim owner: %v", got)
	}
	if got := indexer.fields[IndexUsernameHash](unclaimed); got != nil {
		t.Errorf("the username hash index ignores other kinds: %v", got)
	}
	if got := indexer.fields[IndexNamespaceGID](&corev1.Namespace{}); got != nil {
		t.Errorf("an unlabelled namespace has no gid: %v", got)
	}
}
