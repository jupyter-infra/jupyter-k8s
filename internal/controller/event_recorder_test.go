/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"testing"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/events"
)

func TestEventRecorderPreservesEventDetails(t *testing.T) {
	fakeRecorder := events.NewFakeRecorder(1)
	recorder := newEventRecorder(fakeRecorder)

	recorder.Event(
		&workspacev1alpha1.Workspace{},
		corev1.EventTypeNormal,
		"WorkspaceRunning",
		"Workspace is 100% ready",
	)

	require.Equal(t, "Normal WorkspaceRunning Workspace is 100% ready", <-fakeRecorder.Events)
}
