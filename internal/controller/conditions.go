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
	ReasonDeploymentError = "ComputeError"
	ReasonServiceError    = "ServiceError"
	ReasonNoError         = "NoError"

	// ConditionTypeAvailable reasons (special cases)
	ReasonPreempted = "Preempted"
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
