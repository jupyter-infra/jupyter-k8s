package controller

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Condition types for JupyterServer resources
const (
	// ConditionTypeAvailable indicates the JupyterServer is fully available
	ConditionTypeAvailable = "Available"

	// ConditionTypeStarting indicates the JupyterServer is in the process of starting
	ConditionTypeProgressing = "Progressing"

	// ConditionTypeDegraded indicates the JupyterServer is in a degraded state
	ConditionTypeDegraded = "Degraded"

	// ConditionTypeStopped indicates if the JupyterServer is in a stopped state
	ConditionTypeStopped = "Stopped"
)

// Condition reasons for JupyterServer resources
const (
	// ConditionTypeAvailable and ConditionTypeProgressing reasons
	ReasonResourcesNotReady   = "ResourcesNotReady"
	ReasonComputeNotReady     = "ComputeNotReady"
	ReasonServiceNotReady     = "ServiceNotReady"
	ReasonResourcesReady      = "ResourcesReady"
	ReasonDesiredStateStopped = "DesiredStateStopped"

	// StoppedTypeCondition reasons and ConditionTypeProgressing reasons
	ReasonResourcesNotStopped = "DeploymentAndServiceNotStopped"
	ReasonComputeNotStopped   = "ComputeNotStopped"
	ReasonServiceNotStopped   = "ServiceNotStopped"
	ReasonResourcesStopped    = "DeploymentAndServiceStopped"
	ReasonDesiredStateRunning = "DesiredStateRunning"

	// ConditionTypeDegraded reasons
	ReasonDeploymentError = "ComputeError"
	ReasonServiceError    = "ServiceError"
	ReasonNoError         = "NoError"
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
