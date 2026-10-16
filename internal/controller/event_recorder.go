/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
)

const reconcileEventAction = "Reconcile"

// EventRecorder keeps controller event emission independent of the Kubernetes
// event API used by controller-runtime.
type EventRecorder interface {
	Event(object runtime.Object, eventType, reason, message string)
}

type eventRecorder struct {
	recorder events.EventRecorder
}

func newEventRecorder(recorder events.EventRecorder) EventRecorder {
	return &eventRecorder{recorder: recorder}
}

func (r *eventRecorder) Event(object runtime.Object, eventType, reason, message string) {
	r.recorder.Eventf(object, nil, eventType, reason, reconcileEventAction, "%s", message)
}
