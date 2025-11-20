/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
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
