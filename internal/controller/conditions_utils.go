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

package controller

import (
	"context"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FindCondition returns a pointer to the condition matching the type in the list, or nil if not found
func FindCondition(conditions *[]metav1.Condition, conditionType string) *metav1.Condition {
	for i := range *conditions {
		condition := &(*conditions)[i]
		if condition.Type == conditionType {
			return condition
		}
	}
	return nil
}

// MergeConditionsIfChanged merges new conditions into the workspace's existing conditions.
// Returns the merged condition list if changes are detected, or an empty list if no updates are needed.
func MergeConditionsIfChanged(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
	conditions *[]metav1.Condition) []metav1.Condition {

	// abort early if nothing is requested
	if len(*conditions) == 0 {
		return []metav1.Condition{}
	}
	logger := logf.FromContext(ctx)

	// Create buffers
	conditionsToUpdate := []metav1.Condition{}
	updated := false
	addedConditionNames := []string{}
	unchangedConditionNames := []string{}
	updatedConditionNames := []string{}

	// Build a map of condition types we're updating
	updateTypes := map[string]bool{}
	for _, condition := range *conditions {
		updateTypes[condition.Type] = true
	}

	// Start with existing conditions that we're not updating
	for _, condition := range workspace.Status.Conditions {
		if !updateTypes[condition.Type] {
			conditionsToUpdate = append(conditionsToUpdate, condition)
		}
	}

	// Then evaluate conditions that we are updating
	for _, condition := range *conditions {
		existingCondition := FindCondition(&workspace.Status.Conditions, condition.Type)

		if existingCondition == nil {
			updated = true
			addedConditionNames = append(addedConditionNames, condition.Type)
			conditionsToUpdate = append(conditionsToUpdate, condition)
		} else if existingCondition.Status == condition.Status &&
			existingCondition.Reason == condition.Reason &&
			existingCondition.Message == condition.Message {
			unchangedConditionNames = append(unchangedConditionNames, condition.Type)
			conditionsToUpdate = append(conditionsToUpdate, condition)
		} else {
			// Update the condition by removing old entry and appending new one
			updated = true
			updatedConditionNames = append(updatedConditionNames, condition.Type)
			conditionsToUpdate = append(conditionsToUpdate, condition)
		}
	}

	if !updated {
		// do NOT log here
		return []metav1.Condition{}
	} else {
		logger.Info(
			"Found conditions to update",
			"Added Conditions", addedConditionNames,
			"Updated Conditions", updatedConditionNames,
			"Unchanged Conditions", unchangedConditionNames,
		)
		return conditionsToUpdate
	}
}
