package controller

import (
	"context"

	serversv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// findCondition finds a condition of a specific type in the conditions slice
func FindCondition(conditions *[]metav1.Condition, conditionType string) *metav1.Condition {
	for i := range *conditions {
		condition := &(*conditions)[i]
		if condition.Type == conditionType {
			return condition
		}
	}
	return nil
}

// GetNewConditionsOrEmptyIfUnchanged returns the new list of conditions for JupyterServer.Status
// or an empty list if there are no update needed.
func GetNewConditionsOrEmptyIfUnchanged(
	ctx context.Context,
	jupyterServer *serversv1alpha1.JupyterServer,
	conditions *[]metav1.Condition) []metav1.Condition {

	// abort early if nothing is requested
	if len(*conditions) == 0 {
		return []metav1.Condition{}
	}
	logger := logf.FromContext(ctx)

	// Create buffers
	conditionsToUpdate := []metav1.Condition{}
	updated := false
	added_condition_names := []string{}
	unchanged_condition_names := []string{}
	updated_condition_names := []string{}

	// Build a map of condition types we're updating
	updateTypes := map[string]bool{}
	for _, condition := range *conditions {
		updateTypes[condition.Type] = true
	}

	// Start with existing conditions that we're not updating
	for _, condition := range jupyterServer.Status.Conditions {
		if !updateTypes[condition.Type] {
			conditionsToUpdate = append(conditionsToUpdate, condition)
		}
	}

	// Then evaluate conditions that we are updating
	for _, condition := range *conditions {
		existingCondition := FindCondition(&jupyterServer.Status.Conditions, condition.Type)

		if existingCondition == nil {
			updated = true
			added_condition_names = append(added_condition_names, condition.Type)
			conditionsToUpdate = append(conditionsToUpdate, condition)
		} else if existingCondition.Status == condition.Status &&
			existingCondition.Reason == condition.Reason &&
			existingCondition.Message == condition.Message {
			unchanged_condition_names = append(unchanged_condition_names, condition.Type)
			conditionsToUpdate = append(conditionsToUpdate, condition)
		} else {
			// Update the condition by removing old entry and appending new one
			updated = true
			updated_condition_names = append(updated_condition_names, condition.Type)
			conditionsToUpdate = append(conditionsToUpdate, condition)
		}
	}

	if !updated {
		logger.Info("Found no condition to update", "Unchanged Conditions", unchanged_condition_names)
		return []metav1.Condition{}
	} else {
		logger.Info(
			"Found conditions to update",
			"Added Conditions", added_condition_names,
			"Updated Conditions", updated_condition_names,
			"Unchanged Conditions", unchanged_condition_names,
		)
		return conditionsToUpdate
	}
}
