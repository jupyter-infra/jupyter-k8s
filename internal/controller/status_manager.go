//nolint:typecheck // Temporary disable for development
package controller

import (
	"context"
	"fmt"

	serversv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// StatusManager handles JupyterServer status updates
type StatusManager struct {
	client client.Client
}

// NewStatusManager creates a new StatusManager
func NewStatusManager(ctrl_client client.Client) *StatusManager {
	return &StatusManager{
		client: ctrl_client,
	}
}

func (sm *StatusManager) updateStatus(
	ctx context.Context,
	jupyterServer *serversv1alpha1.JupyterServer,
	conditionsToUpdate *[]metav1.Condition,
	updateResourceNames bool,
) error {
	logger := logf.FromContext(ctx)

	if !updateResourceNames && len(*conditionsToUpdate) == 0 {
		logger.Info("no-op: JupyterServer.Status is up-to-date")
		return nil
	}
	if len(*conditionsToUpdate) > 0 {
		// requesting to modify condition: overwrite
		jupyterServer.Status.Conditions = *conditionsToUpdate
	}
	if err := sm.client.Status().Update(ctx, jupyterServer); err != nil {
		return fmt.Errorf("failed to update JupyterServer.Status: %w", err)
	}
	logger.Info("updated JupyterServer.Status")
	return nil
}

// UpdateStartingStatus: set Available to false and Progressing to true
func (sm *StatusManager) UpdateStartingStatus(
	ctx context.Context,
	jupyterServer *serversv1alpha1.JupyterServer,
	computeReady bool,
	serviceReady bool,
	computeName string,
	serviceName string,
) error {
	// ensure StartingCondition is set to True with the appropriate reason

	startingReason := ReasonResourcesNotReady
	startingMessage := "JupyterServer is starting"

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
		"JupyterServer is starting",
	)

	// Apply all conditions
	conditions := []metav1.Condition{
		availableCondition,
		progressingCondition,
		degradedCondition,
		stoppedCondition,
	}
	conditionsToUpdate := GetNewConditionsOrEmptyIfUnchanged(ctx, jupyterServer, &conditions)
	shouldUpdateResourceNames := jupyterServer.Status.DeploymentName != computeName || jupyterServer.Status.ServiceName != serviceName
	if shouldUpdateResourceNames {
		jupyterServer.Status.DeploymentName = computeName
		jupyterServer.Status.ServiceName = serviceName
	}
	if err := sm.updateStatus(ctx, jupyterServer, &conditionsToUpdate, shouldUpdateResourceNames); err != nil {
		return err
	}
	return nil
}

// UpdateErrorStatus: sets the Degraded condition to true with the specified error reason and message
func (sm *StatusManager) UpdateErrorStatus(ctx context.Context, jupyterServer *serversv1alpha1.JupyterServer, reason, message string) error {
	// Set DegradedCondition to true with the provided error reason and message
	degradedCondition := NewCondition(
		ConditionTypeDegraded,
		metav1.ConditionTrue,
		reason,
		message,
	)
	conditionsToUpdate := GetNewConditionsOrEmptyIfUnchanged(ctx, jupyterServer, &[]metav1.Condition{degradedCondition})
	if err := sm.updateStatus(ctx, jupyterServer, &conditionsToUpdate, false); err != nil {
		return err
	}
	return nil
}

// UpdateRunningStatus sets the Available condition to true and Progressing to false
func (sm *StatusManager) UpdateRunningStatus(ctx context.Context, jupyterServer *serversv1alpha1.JupyterServer) error {
	// ensure AvailableCondition is set to true with ReasonResourcesReady
	availableCondition := NewCondition(
		ConditionTypeAvailable,
		metav1.ConditionTrue,
		ReasonResourcesReady,
		"JupyterServer is ready",
	)

	// ensure ProgressingCondition is set to false with ReasonResourcesReady
	progressingCondition := NewCondition(
		ConditionTypeProgressing,
		metav1.ConditionFalse,
		ReasonResourcesReady,
		"JupyterServer is ready",
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
		"JupyterServer is running",
	)

	// Apply all conditions
	conditions := []metav1.Condition{
		availableCondition,
		progressingCondition,
		degradedCondition,
		stoppedCondition,
	}

	conditionsToUpdate := GetNewConditionsOrEmptyIfUnchanged(ctx, jupyterServer, &conditions)
	if err := sm.updateStatus(ctx, jupyterServer, &conditionsToUpdate, false); err != nil {
		return err
	}
	return nil
}

// UpdateStoppingStatus updates status to represent a stopping JupyterServer
func (sm *StatusManager) UpdateStoppingStatus(ctx context.Context, jupyterServer *serversv1alpha1.JupyterServer, computeStopped bool, serviceStopped bool) error {
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

	conditionsToUpdate := GetNewConditionsOrEmptyIfUnchanged(ctx, jupyterServer, &conditions)
	if err := sm.updateStatus(ctx, jupyterServer, &conditionsToUpdate, false); err != nil {
		return err
	}
	return nil
}

// UpdateStoppedStatus sets all conditions to represent a stopped server
func (sm *StatusManager) UpdateStoppedStatus(ctx context.Context, jupyterServer *serversv1alpha1.JupyterServer) error {
	// ensure AvailableCondition is set to false with ReasonDesiredStateStopped
	availableCondition := NewCondition(
		ConditionTypeAvailable,
		metav1.ConditionFalse,
		ReasonDesiredStateStopped,
		"JupyterServer is stopped",
	)

	// ensure ProgressingCondition is set to false with ReasonDesiredStateStopped
	progressingCondition := NewCondition(
		ConditionTypeProgressing,
		metav1.ConditionFalse,
		ReasonDesiredStateStopped,
		"JupyterServer is stopped",
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
		"JupyterServer is stopped",
	)

	// Apply all conditions
	conditions := []metav1.Condition{
		availableCondition,
		progressingCondition,
		degradedCondition,
		stoppedCondition,
	}

	conditionsToUpdate := GetNewConditionsOrEmptyIfUnchanged(ctx, jupyterServer, &conditions)
	jupyterServer.Status.DeploymentName = ""
	jupyterServer.Status.ServiceName = ""
	if err := sm.updateStatus(ctx, jupyterServer, &conditionsToUpdate, true); err != nil {
		return err
	}
	return nil
}
