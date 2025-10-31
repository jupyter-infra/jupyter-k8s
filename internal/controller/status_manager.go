package controller

import (
	"context"
	"fmt"
	"reflect"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// StatusManager handles Workspace status updates
type StatusManager struct {
	client client.Client
}

// NewStatusManager creates a new StatusManager
func NewStatusManager(k8sClient client.Client) *StatusManager {
	return &StatusManager{
		client: k8sClient,
	}
}

func (sm *StatusManager) updateStatus(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
	conditionsToUpdate *[]metav1.Condition,
	snapshotStatus *workspacev1alpha1.WorkspaceStatus,
) error {
	logger := logf.FromContext(ctx)
	if len(*conditionsToUpdate) > 0 {
		// requesting to modify condition: overwrite
		workspace.Status.Conditions = *conditionsToUpdate
	}

	if reflect.DeepEqual(workspace.Status, snapshotStatus) {
		// no-op: status hasn't changed
		return nil
	}

	if err := sm.client.Status().Update(ctx, workspace); err != nil {
		return fmt.Errorf("failed to update Workspace.Status: %w", err)
	}
	logger.Info("updated Workspace.Status")
	return nil
}

// WorkspaceRunningReadiness wraps the readiness flag of underlying components
type WorkspaceRunningReadiness struct {
	computeReady         bool
	serviceReady         bool
	accessResourcesReady bool
	TemplateValid        bool
}

// UpdateStartingStatus sets Available to false and Progressing to true
func (sm *StatusManager) UpdateStartingStatus(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
	readiness WorkspaceRunningReadiness,
	snapshotStatus *workspacev1alpha1.WorkspaceStatus,
) error {
	// default: nothing is started yet
	startingReason := ReasonResourcesNotReady
	startingMessage := "Workspace is starting"

	if readiness.computeReady && readiness.serviceReady && readiness.accessResourcesReady {
		return fmt.Errorf("invalid call: not all resources should be ready in method UpdateStartingStatus")
	}

	waitingForCompute := !readiness.computeReady && readiness.serviceReady && readiness.accessResourcesReady
	waitingForService := readiness.computeReady && !readiness.serviceReady && readiness.accessResourcesReady
	waitingForAccess := readiness.computeReady && readiness.serviceReady && !readiness.accessResourcesReady

	if waitingForCompute {
		startingReason = ReasonComputeNotReady
		startingMessage = "Compute is not ready"
	} else if waitingForAccess {
		startingReason = ReasonAccessNotReady
		startingMessage = "Access is not ready"
	} else if waitingForService {
		startingReason = ReasonServiceNotReady
		startingMessage = "Service is not ready"
	}
	// Nothing ready corresponds to the default

	availableCondition := NewCondition(
		ConditionTypeAvailable,
		metav1.ConditionFalse,
		startingReason,
		startingMessage,
	)

	progressingCondition := NewCondition(
		ConditionTypeProgressing,
		metav1.ConditionTrue,
		startingReason,
		startingMessage,
	)

	// ensure DegradedCondition is set to False with ReasonNoError
	degradedCondition := NewCondition(
		ConditionTypeDegraded,
		metav1.ConditionFalse,
		ReasonNoError,
		"No errors detected",
	)

	// ensure StoppedCondition is set to False with ReasonDesiredStateRunning
	stoppedCondition := NewCondition(
		ConditionTypeStopped,
		metav1.ConditionFalse,
		ReasonDesiredStateRunning,
		"Workspace is starting",
	)

	// if we got here, validation passed
	validCondition := NewCondition(
		ConditionTypeValid,
		metav1.ConditionTrue,
		ReasonAllChecksPass,
		"All validation checks passed",
	)

	// Apply all conditions
	conditions := []metav1.Condition{
		availableCondition,
		progressingCondition,
		degradedCondition,
		stoppedCondition,
		validCondition,
	}

	conditionsToUpdate := GetNewConditionsOrEmptyIfUnchanged(ctx, workspace, &conditions)
	return sm.updateStatus(ctx, workspace, &conditionsToUpdate, snapshotStatus)
}

// UpdateErrorStatus sets the Degraded condition to true with the specified error reason and message
func (sm *StatusManager) UpdateErrorStatus(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
	reason string,
	message string,
	snapshotStatus *workspacev1alpha1.WorkspaceStatus) error {
	// Set DegradedCondition to true with the provided error reason and message
	degradedCondition := NewCondition(
		ConditionTypeDegraded,
		metav1.ConditionTrue,
		reason,
		message,
	)
	conditionsToUpdate := GetNewConditionsOrEmptyIfUnchanged(ctx, workspace, &[]metav1.Condition{degradedCondition})
	return sm.updateStatus(ctx, workspace, &conditionsToUpdate, snapshotStatus)
}

// SetInvalid sets the Valid condition to false when policy validation fails
func (sm *StatusManager) SetInvalid(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
	validation *TemplateValidationResult,
	snapshotStatus *workspacev1alpha1.WorkspaceStatus) error {
	// Build violation message
	message := fmt.Sprintf("Validation failed: %d violation(s)", len(validation.Violations))
	if len(validation.Violations) > 0 {
		// Include first violation in message for visibility
		v := validation.Violations[0]
		message = fmt.Sprintf("%s. First: %s at %s (allowed: %s, actual: %s)",
			message, v.Message, v.Field, v.Allowed, v.Actual)
	}

	validCondition := NewCondition(
		ConditionTypeValid,
		metav1.ConditionFalse,
		ReasonTemplateViolation,
		message,
	)

	// Policy violations are user errors, not system degradation
	// Degraded is for system issues (controller failures, k8s API errors, etc.)
	// Invalid config should only affect Available, not Degraded
	degradedCondition := NewCondition(
		ConditionTypeDegraded,
		metav1.ConditionFalse,
		ReasonTemplateViolation,
		"No system errors detected",
	)

	availableCondition := NewCondition(
		ConditionTypeAvailable,
		metav1.ConditionFalse,
		ReasonTemplateViolation,
		"Validation failed",
	)

	// Set Progressing to false
	progressingCondition := NewCondition(
		ConditionTypeProgressing,
		metav1.ConditionFalse,
		ReasonTemplateViolation,
		"Validation failed",
	)

	conditions := []metav1.Condition{
		validCondition,
		degradedCondition,
		availableCondition,
		progressingCondition,
	}

	conditionsToUpdate := GetNewConditionsOrEmptyIfUnchanged(ctx, workspace, &conditions)
	return sm.updateStatus(ctx, workspace, &conditionsToUpdate, snapshotStatus)
}

// UpdateRunningStatus sets the Available condition to true and Progressing to false
func (sm *StatusManager) UpdateRunningStatus(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
	snapshotStatus *workspacev1alpha1.WorkspaceStatus) error {
	// ensure AvailableCondition is set to true with ReasonResourcesReady
	availableCondition := NewCondition(
		ConditionTypeAvailable,
		metav1.ConditionTrue,
		ReasonResourcesReady,
		"Workspace is ready",
	)

	// ensure ProgressingCondition is set to false with ReasonResourcesReady
	progressingCondition := NewCondition(
		ConditionTypeProgressing,
		metav1.ConditionFalse,
		ReasonResourcesReady,
		"Workspace is ready",
	)

	// ensure DegradedCondition is set to false with ReasonNoError
	degradedCondition := NewCondition(
		ConditionTypeDegraded,
		metav1.ConditionFalse,
		ReasonNoError,
		"No errors detected",
	)

	// ensure StoppedCondition is set to false with ReasonDesiredStateRunning
	stoppedCondition := NewCondition(
		ConditionTypeStopped,
		metav1.ConditionFalse,
		ReasonDesiredStateRunning,
		"Workspace is running",
	)

	// if we got here, validation passed
	validCondition := NewCondition(
		ConditionTypeValid,
		metav1.ConditionTrue,
		ReasonAllChecksPass,
		"All validation checks passed",
	)

	// apply all conditions
	conditions := []metav1.Condition{
		availableCondition,
		progressingCondition,
		degradedCondition,
		stoppedCondition,
		validCondition,
	}

	conditionsToUpdate := GetNewConditionsOrEmptyIfUnchanged(ctx, workspace, &conditions)
	return sm.updateStatus(ctx, workspace, &conditionsToUpdate, snapshotStatus)
}

// WorkspaceStoppingReadiness wraps the readiness flag of underlying components
type WorkspaceStoppingReadiness struct {
	computeStopped         bool
	serviceStopped         bool
	accessResourcesStopped bool
}

// UpdateStoppingStatus sets Available to false and Progressing to true
func (sm *StatusManager) UpdateStoppingStatus(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
	readiness WorkspaceStoppingReadiness,
	snapshotStatus *workspacev1alpha1.WorkspaceStatus) error {
	// default: nothing is stopped yet
	stoppingReason := ReasonResourcesNotStopped
	stoppingMessage := "Resources are still running"

	if readiness.computeStopped && readiness.serviceStopped && readiness.accessResourcesStopped {
		return fmt.Errorf("invalid call: not all resources should be stopped in method UpdateStoppingStatus")
	}

	waitingForCompute := !readiness.computeStopped && readiness.serviceStopped && readiness.accessResourcesStopped
	waitingForService := readiness.computeStopped && !readiness.serviceStopped && readiness.accessResourcesStopped
	waitingForAccess := readiness.computeStopped && readiness.serviceStopped && !readiness.accessResourcesStopped

	if waitingForCompute {
		stoppingReason = ReasonComputeNotStopped
		stoppingMessage = "Compute is still running"
	} else if waitingForAccess {
		stoppingReason = ReasonAccessNotStopped
		stoppingMessage = "Access is still up"
	} else if waitingForService {
		stoppingReason = ReasonServiceNotStopped
		stoppingMessage = "Service is still up"
	}
	// Nothing stopped corresponds to the default

	// ensure AvailableCondition is set to false with ReasonDesiredStateStopped
	availableCondition := NewCondition(
		ConditionTypeAvailable,
		metav1.ConditionFalse,
		ReasonDesiredStateStopped,
		"Desired status is Stopped",
	)

	// ensure ProgressingCondition is set to false with ReasonDesiredStateStopped
	progressingCondition := NewCondition(
		ConditionTypeProgressing,
		metav1.ConditionTrue,
		ReasonDesiredStateStopped,
		stoppingMessage,
	)

	// ensure DegradedCondition is set to false with ReasonNoError
	degradedCondition := NewCondition(
		ConditionTypeDegraded,
		metav1.ConditionFalse,
		ReasonNoError,
		"No errors detected",
	)

	// ensure StoppedCondition is set to false with appropriate reason
	stoppedCondition := NewCondition(
		ConditionTypeStopped,
		metav1.ConditionFalse,
		stoppingReason,
		stoppingMessage,
	)

	// if we got here, validation passed
	validCondition := NewCondition(
		ConditionTypeValid,
		metav1.ConditionTrue,
		ReasonAllChecksPass,
		"All validation checks passed",
	)

	// Apply all conditions
	conditions := []metav1.Condition{
		availableCondition,
		progressingCondition,
		degradedCondition,
		stoppedCondition,
		validCondition,
	}

	conditionsToUpdate := GetNewConditionsOrEmptyIfUnchanged(ctx, workspace, &conditions)
	return sm.updateStatus(ctx, workspace, &conditionsToUpdate, snapshotStatus)
}

// UpdateStoppedStatus sets Available and Progressing to false, Stopped to true
func (sm *StatusManager) UpdateStoppedStatus(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
	snapshotStatus *workspacev1alpha1.WorkspaceStatus) error {
	// Check if workspace was stopped due to preemption
	var availableCondition metav1.Condition
	if workspace.Annotations != nil && workspace.Annotations[PreemptionReasonAnnotation] == PreemptedReason {
		availableCondition = NewCondition(
			ConditionTypeAvailable,
			metav1.ConditionFalse,
			"Preempted",
			"Workspace preempted due to resource contention",
		)
	} else {
		// ensure AvailableCondition is set to false with ReasonDesiredStateStopped
		availableCondition = NewCondition(
			ConditionTypeAvailable,
			metav1.ConditionFalse,
			ReasonDesiredStateStopped,
			"Workspace is stopped",
		)
	}

	// ensure ProgressingCondition is set to false with ReasonDesiredStateStopped
	progressingCondition := NewCondition(
		ConditionTypeProgressing,
		metav1.ConditionFalse,
		ReasonDesiredStateStopped,
		"Workspace is stopped",
	)

	// ensure DegradedCondition is set to false with ReasonNoError
	degradedCondition := NewCondition(
		ConditionTypeDegraded,
		metav1.ConditionFalse,
		ReasonNoError,
		"No errors detected",
	)

	// ensure StoppedCondition is set to True with ReasonDeploymentAndServiceStopped
	stoppedCondition := NewCondition(
		ConditionTypeStopped,
		metav1.ConditionTrue,
		ReasonResourcesStopped,
		"Workspace is stopped",
	)

	// if we got here, validation passed
	validCondition := NewCondition(
		ConditionTypeValid,
		metav1.ConditionTrue,
		ReasonAllChecksPass,
		"All validation checks passed",
	)

	// Apply all conditions
	conditions := []metav1.Condition{
		availableCondition,
		progressingCondition,
		degradedCondition,
		stoppedCondition,
		validCondition,
	}

	conditionsToUpdate := GetNewConditionsOrEmptyIfUnchanged(ctx, workspace, &conditions)
	workspace.Status.DeploymentName = ""
	workspace.Status.ServiceName = ""
	return sm.updateStatus(ctx, workspace, &conditionsToUpdate, snapshotStatus)
}

// UpdateDeletingStatus sets the workspace status to indicate deletion in progress
func (sm *StatusManager) UpdateDeletingStatus(ctx context.Context, workspace *workspacev1alpha1.Workspace) error {
	condition := metav1.Condition{
		Type:               "Deleting",
		Status:             metav1.ConditionTrue,
		Reason:             "DeletionInProgress",
		Message:            "Workspace resources are being deleted",
		LastTransitionTime: metav1.Now(),
	}

	meta.SetStatusCondition(&workspace.Status.Conditions, condition)
	return sm.client.Status().Update(ctx, workspace)
}
