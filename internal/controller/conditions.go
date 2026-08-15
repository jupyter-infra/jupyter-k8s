/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

// Package controller defines the jupyter-k8s controller logic
package controller

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Condition types for Workspace resources
const (
	// ConditionTypeAvailable indicates the Workspace is fully available
	ConditionTypeAvailable = "Available"

	// ConditionTypeProgressing indicates the Workspace is in a transitional state (starting or stopping)
	ConditionTypeProgressing = "Progressing"

	// ConditionTypeDegraded indicates the Workspace is in a degraded state
	ConditionTypeDegraded = "Degraded"

	// ConditionTypeStopped indicates if the Workspace is in a stopped state
	ConditionTypeStopped = "Stopped"

	// ConditionTypeDeleting indicates the Workspace is being deleted and resources are being cleaned up
	ConditionTypeDeleting = "Deleting"

	// IntegrationConditionTypeReady is the condition type on status.integrationStatuses[].conditions
	// carrying an integration's probe verdict. Named after the positive state (Kubernetes convention),
	// like ConditionTypeAvailable; its Status (True/False) holds the actual verdict.
	IntegrationConditionTypeReady = "Ready"
)

// Condition reasons for Workspace resources
const (
	// ConditionTypeAvailable and ConditionTypeProgressing reasons
	ReasonResourcesNotReady   = "ResourcesNotReady"
	ReasonComputeNotReady     = "ComputeNotReady"
	ReasonServiceNotReady     = "ServiceNotReady"
	ReasonAccessNotReady      = "AccessNotReady"
	ReasonResourcesReady      = "ResourcesReady"
	ReasonDesiredStateStopped = "DesiredStateStopped"

	// StoppedTypeCondition reasons and ConditionTypeProgressing reasons
	ReasonResourcesNotStopped = "ResourcesNotStopped"
	ReasonComputeNotStopped   = "ComputeNotStopped"
	ReasonServiceNotStopped   = "ServiceNotStopped"
	ReasonAccessNotStopped    = "AccessNotStopped"
	ReasonResourcesStopped    = "AllResourcesStopped"
	ReasonDesiredStateRunning = "DesiredStateRunning"

	// ConditionTypeDegraded reasons
	ReasonDeploymentError              = "ComputeError"
	ReasonServiceError                 = "ServiceError"
	ReasonAccessProbeThresholdExceeded = "AccessProbeThresholdExceeded"
	ReasonNoError                      = "NoError"

	// ConditionTypeAvailable reasons (special cases)
	ReasonPreempted = "Preempted"

	// ConditionTypeDeleting reasons
	ReasonDeletionInProgress = "DeletionInProgress"

	// IntegrationConditionTypeReady reasons on status.integrationStatuses[].conditions (machine-readable, CamelCase).
	IntegrationReasonReady       = "Ready"
	IntegrationReasonProbeFailed = "ProbeFailed"
	IntegrationReasonPodNotFound = "PodNotFound"
	IntegrationReasonProbeError  = "ProbeError"
	// IntegrationReasonNotResolved is reported on status.integrationStatuses[] for an attached
	// integration that has no frozen resolution yet -- e.g. its first-attach capture failed because the
	// referenced resource does not exist or the template is broken. Surfacing it (rather than logging
	// only) lets an admin see an unresolved integration on the Workspace status. The detailed cause is
	// in the operator logs (reconcileIntegrationFreeze); the status message points there.
	IntegrationReasonNotResolved = "NotResolved"

	// Kubernetes Event reasons for integration status transitions (recorded on the Workspace via the
	// EventRecorder, surfaced by `kubectl describe workspace`). These are EDGE-TRIGGERED: an event is
	// emitted only when an integration's probe verdict changes, never on every report-only probe cadence
	// (see getIntegrationStatusEvent), so a persistently-degraded integration does not spam the event stream.
	IntegrationEventDegraded  = "IntegrationDegraded"
	IntegrationEventRecovered = "IntegrationRecovered"
)

// NewCondition creates a new condition with the specified status
func NewCondition(condType string, status metav1.ConditionStatus, reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:               condType,
		Status:             status,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             reason,
		Message:            message,
	}
}
