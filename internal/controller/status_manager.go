package controller

import (
	"context"
	"fmt"

	workspacesv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"

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
	workspace *workspacesv1alpha1.Workspace,
	conditionsToUpdate *[]metav1.Condition,
	updateResourceNames bool,
) error {
	logger := logf.FromContext(ctx)

	if !updateResourceNames && len(*conditionsToUpdate) == 0 {
		logger.Info("no-op: Workspace.Status is up-to-date")
		return nil
	}
	if len(*conditionsToUpdate) > 0 {
		// requesting to modify condition: overwrite
		workspace.Status.Conditions = *conditionsToUpdate
	}
	if err := sm.client.Status().Update(ctx, workspace); err != nil {
		return fmt.Errorf("failed to update Workspace.Status: %w", err)
	}
	logger.Info("updated Workspace.Status")
	return nil
}

// UpdateStartingStatus sets Available to false and Progressing to true
func (sm *StatusManager) UpdateStartingStatus(
	ctx context.Context,
	workspace *workspacesv1alpha1.Workspace,
	computeReady bool,
	serviceReady bool,
	computeName string,
	serviceName string,
) error {
	// ensure StartingCondition is set to True with the appropriate reason

	startingReason := ReasonResourcesNotReady
	startingMessage := "Workspace is starting"

	if computeReady && serviceReady {
		return fmt.Errorf("invalid call: not all resources should be ready in method UpdateStartingStatus")
	} else if serviceReady {
		startingReason = ReasonComputeNotReady
		startingMessage = "Compute is not ready"
	} else if computeReady {
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

	// Apply all conditions
	conditions := []metav1.Condition{
		availableCondition,
		progressingCondition,
		degradedCondition,
		stoppedCondition,
	}
	conditionsToUpdate := GetNewConditionsOrEmptyIfUnchanged(ctx, workspace, &conditions)
	shouldUpdateResourceNames := workspace.Status.DeploymentName != computeName || workspace.Status.ServiceName != serviceName
	if shouldUpdateResourceNames {
		workspace.Status.DeploymentName = computeName
		workspace.Status.ServiceName = serviceName
	}
	return sm.updateStatus(ctx, workspace, &conditionsToUpdate, shouldUpdateResourceNames)
}

// UpdateErrorStatus sets the Degraded condition to true with the specified error reason and message
func (sm *StatusManager) UpdateErrorStatus(ctx context.Context, workspace *workspacesv1alpha1.Workspace, reason, message string) error {
	// Set DegradedCondition to true with the provided error reason and message
	degradedCondition := NewCondition(
		ConditionTypeDegraded,
		metav1.ConditionTrue,
		reason,
		message,
	)
	conditionsToUpdate := GetNewConditionsOrEmptyIfUnchanged(ctx, workspace, &[]metav1.Condition{degradedCondition})
	return sm.updateStatus(ctx, workspace, &conditionsToUpdate, false)
}

// SetTemplateValidated sets the TemplateValidation condition to true
func (sm *StatusManager) SetTemplateValidated(ctx context.Context, workspace *workspacesv1alpha1.Workspace) error {
	templateCondition := NewCondition(
		ConditionTypeTemplateValidation,
		metav1.ConditionTrue,
		ReasonTemplateValid,
		"Template validation passed",
	)

	conditionsToUpdate := GetNewConditionsOrEmptyIfUnchanged(ctx, workspace, &[]metav1.Condition{templateCondition})
	return sm.updateStatus(ctx, workspace, &conditionsToUpdate, false)
}

// SetTemplateRejected sets the TemplateValidation condition to false with policy violations
func (sm *StatusManager) SetTemplateRejected(ctx context.Context, workspace *workspacesv1alpha1.Workspace, validation *TemplateValidationResult) error {
	// Build violation message
	message := fmt.Sprintf("Template validation failed: %d violation(s)", len(validation.Violations))
	if len(validation.Violations) > 0 {
		// Include first violation in message for visibility
		v := validation.Violations[0]
		message = fmt.Sprintf("%s. First: %s at %s (allowed: %s, actual: %s)",
			message, v.Message, v.Field, v.Allowed, v.Actual)
	}

	templateCondition := NewCondition(
		ConditionTypeTemplateValidation,
		metav1.ConditionFalse,
		ReasonPolicyViolation,
		message,
	)

	// Policy violations are user errors, not system degradation
	// Degraded is for system issues (controller failures, k8s API errors, etc.)
	// Invalid config should only affect Available, not Degraded
	degradedCondition := NewCondition(
		ConditionTypeDegraded,
		metav1.ConditionFalse,
		ReasonPolicyViolation,
		"No system errors detected",
	)

	availableCondition := NewCondition(
		ConditionTypeAvailable,
		metav1.ConditionFalse,
		ReasonPolicyViolation,
		"Template validation failed",
	)

	// Set Progressing to false
	progressingCondition := NewCondition(
		ConditionTypeProgressing,
		metav1.ConditionFalse,
		ReasonPolicyViolation,
		"Template validation failed",
	)

	conditions := []metav1.Condition{
		templateCondition,
		degradedCondition,
		availableCondition,
		progressingCondition,
	}

	conditionsToUpdate := GetNewConditionsOrEmptyIfUnchanged(ctx, workspace, &conditions)
	return sm.updateStatus(ctx, workspace, &conditionsToUpdate, false)
}

// UpdateRunningStatus sets the Available condition to true and Progressing to false
func (sm *StatusManager) UpdateRunningStatus(ctx context.Context, workspace *workspacesv1alpha1.Workspace) error {
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

	// Apply all conditions
	conditions := []metav1.Condition{
		availableCondition,
		progressingCondition,
		degradedCondition,
		stoppedCondition,
	}

	conditionsToUpdate := GetNewConditionsOrEmptyIfUnchanged(ctx, workspace, &conditions)
	return sm.updateStatus(ctx, workspace, &conditionsToUpdate, false)
}

// UpdateStoppingStatus sets Available to false and Progressing to true
func (sm *StatusManager) UpdateStoppingStatus(ctx context.Context, workspace *workspacesv1alpha1.Workspace, computeStopped bool, serviceStopped bool) error {
	stoppingReason := ReasonResourcesNotStopped
	stoppingMessage := "Resources are still running"

	if computeStopped && serviceStopped {
		return fmt.Errorf("invalid call: not all resources should be stopped in method UpdateStoppingStatus")
	} else if serviceStopped {
		stoppingReason = ReasonComputeNotStopped
		stoppingMessage = "Compute is still running"
	} else if computeStopped {
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

	// Apply all conditions
	conditions := []metav1.Condition{
		availableCondition,
		progressingCondition,
		degradedCondition,
		stoppedCondition,
	}

	conditionsToUpdate := GetNewConditionsOrEmptyIfUnchanged(ctx, workspace, &conditions)
	return sm.updateStatus(ctx, workspace, &conditionsToUpdate, false)
}

// UpdateStoppedStatus sets Available and Progressing to false, Stopped to true
func (sm *StatusManager) UpdateStoppedStatus(ctx context.Context, workspace *workspacesv1alpha1.Workspace) error {
	// ensure AvailableCondition is set to false with ReasonDesiredStateStopped
	availableCondition := NewCondition(
		ConditionTypeAvailable,
		metav1.ConditionFalse,
		ReasonDesiredStateStopped,
		"Workspace is stopped",
	)

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

	// Apply all conditions
	conditions := []metav1.Condition{
		availableCondition,
		progressingCondition,
		degradedCondition,
		stoppedCondition,
	}

	conditionsToUpdate := GetNewConditionsOrEmptyIfUnchanged(ctx, workspace, &conditions)
	workspace.Status.DeploymentName = ""
	workspace.Status.ServiceName = ""
	return sm.updateStatus(ctx, workspace, &conditionsToUpdate, true)
}
