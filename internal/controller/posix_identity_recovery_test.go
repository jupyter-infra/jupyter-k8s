/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/posixidentity"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
)

// recoveryIndexer registers the posixidentity indexes on a fake client builder.
type recoveryIndexer struct{ builder *fake.ClientBuilder }

func (i *recoveryIndexer) IndexField(_ context.Context, obj client.Object, field string, fn client.IndexerFunc) error {
	i.builder = i.builder.WithIndex(obj, field, fn)
	return nil
}

func newRecoveryReconciler(t *testing.T, objs ...client.Object) (*WorkspaceReconciler, client.Client, *record.FakeRecorder) {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, workspacev1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	indexer := &recoveryIndexer{builder: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...)}
	require.NoError(t, posixidentity.SetupIndexes(context.Background(), indexer))
	c := indexer.builder.Build()
	recorder := record.NewFakeRecorder(10)
	return &WorkspaceReconciler{
		Client:        c,
		identities:    posixidentity.NewAllocator(c, c),
		eventRecorder: recorder,
	}, c, recorder
}

func claimingWorkspace(owner, uid string) *workspacev1alpha1.Workspace {
	return &workspacev1alpha1.Workspace{ObjectMeta: metav1.ObjectMeta{
		Name: "ws", Namespace: "team-a",
		Annotations: map[string]string{workspaceutil.AnnotationCreatedBy: owner},
		Labels:      map[string]string{workspaceutil.LabelPosixUID: uid},
	}}
}

func TestRecoverPosixIdentityRecreatesTheClaimedUID(t *testing.T) {
	ws := claimingWorkspace("alice", "1234567")
	r, c, recorder := newRecoveryReconciler(t, ws)

	r.recoverPosixIdentity(context.Background(), ws)

	identity := &workspacev1alpha1.PosixIdentity{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: "uid-1234567"}, identity))
	assert.Equal(t, "alice", identity.Spec.Username)
	assert.Empty(t, recorder.Events)
}

func TestRecoverPosixIdentityReportsAConflict(t *testing.T) {
	ws := claimingWorkspace("alice", "1234567")
	bobs := &workspacev1alpha1.PosixIdentity{
		ObjectMeta: metav1.ObjectMeta{Name: "uid-1234567"},
		Spec: workspacev1alpha1.PosixIdentitySpec{
			Username: "bob", UsernameHash: posixidentity.UsernameHash("bob"), UnixName: "bob",
		},
	}
	r, _, recorder := newRecoveryReconciler(t, ws, bobs)

	r.recoverPosixIdentity(context.Background(), ws)

	require.Len(t, recorder.Events, 1)
	assert.Contains(t, <-recorder.Events, ReasonPosixIdentityConflict)
}

func TestRecoverPosixIdentitySkipsUnclaimedWorkspaces(t *testing.T) {
	ws := claimingWorkspace("alice", "")
	r, c, _ := newRecoveryReconciler(t, ws)

	r.recoverPosixIdentity(context.Background(), ws)

	list := &workspacev1alpha1.PosixIdentityList{}
	require.NoError(t, c.List(context.Background(), list))
	assert.Empty(t, list.Items)
}
