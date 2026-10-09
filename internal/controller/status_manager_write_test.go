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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// countingStatusWriter counts the status updates a StatusManager issues.
type countingStatusWriter struct {
	client.SubResourceWriter
	updates int
}

func (w *countingStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	w.updates++
	return w.SubResourceWriter.Update(ctx, obj, opts...)
}

type countingStatusClient struct {
	client.Client
	status *countingStatusWriter
}

func (c *countingStatusClient) Status() client.SubResourceWriter { return c.status }

func TestStatusManagerWritesOnlyWhenTheStatusChanged(t *testing.T) {
	ctx := context.Background()
	workspace := &workspacev1alpha1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceName, Namespace: testNamespaceName}}
	base := fake.NewClientBuilder().WithScheme(crudScheme(t)).WithStatusSubresource(workspace).WithObjects(workspace).Build()
	c := &countingStatusClient{Client: base, status: &countingStatusWriter{SubResourceWriter: base.Status()}}
	sm := NewStatusManager(c)
	readiness := WorkspaceRunningReadiness{serviceReady: true, accessResourcesReady: true}

	require.NoError(t, sm.UpdateStartingStatus(ctx, workspace, readiness, workspace.Status.DeepCopy()))
	assert.Equal(t, 1, c.status.updates)

	require.NoError(t, sm.UpdateStartingStatus(ctx, workspace, readiness, workspace.Status.DeepCopy()))
	assert.Equal(t, 1, c.status.updates, "an unchanged status is not written")

	readiness.computeStep = &StartStep{Reason: ReasonWaitingForNode, Message: stepTestSchedMsg}
	require.NoError(t, sm.UpdateStartingStatus(ctx, workspace, readiness, workspace.Status.DeepCopy()))
	assert.Equal(t, 2, c.status.updates, "a new message is written")

	require.NoError(t, sm.UpdateStartingStatus(ctx, workspace, readiness, nil))
	assert.Equal(t, 3, c.status.updates, "without a snapshot the status is written")
}
