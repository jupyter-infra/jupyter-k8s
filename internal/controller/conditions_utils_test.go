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
	"testing"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func init() {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
}

func TestFindCondition(t *testing.T) {
	conditions := []metav1.Condition{
		{Type: "Available", Status: metav1.ConditionTrue},
		{Type: "Progressing", Status: metav1.ConditionFalse},
	}

	// Test finding an existing condition
	result := FindCondition(&conditions, "Available")
	assert.NotNil(t, result)
	assert.Equal(t, "Available", result.Type)
	assert.Equal(t, metav1.ConditionTrue, result.Status)

	// Test finding a non-existent condition
	result = FindCondition(&conditions, "NonExistent")
	assert.Nil(t, result)
}

func TestMergeConditionsIfChanged(t *testing.T) {
	ctx := context.Background()
	workspace := &workspacev1alpha1.Workspace{}
	workspace.Status.Conditions = []metav1.Condition{
		{Type: "Existing", Status: metav1.ConditionTrue, Reason: "InitialReason", Message: "Initial message"},
		{Type: "ToUpdate", Status: metav1.ConditionFalse, Reason: "OldReason", Message: "Old message"},
	}

	// Test with completely new conditions
	newConditions := []metav1.Condition{
		{Type: "New", Status: metav1.ConditionTrue, Reason: "NewReason", Message: "New message"},
	}
	result := MergeConditionsIfChanged(ctx, workspace, &newConditions)
	assert.Len(t, result, 3) // 2 existing + 1 new

	// Check that both existing conditions are preserved
	foundExisting := false
	foundNew := false
	for _, cond := range result {
		if cond.Type == "Existing" {
			foundExisting = true
		}
		if cond.Type == "New" {
			foundNew = true
		}
	}
	assert.True(t, foundExisting, "Should contain existing condition")
	assert.True(t, foundNew, "Should contain new condition")

	// Test with updated condition
	updateConditions := []metav1.Condition{
		{Type: "ToUpdate", Status: metav1.ConditionTrue, Reason: "NewReason", Message: "Updated message"},
	}
	result = MergeConditionsIfChanged(ctx, workspace, &updateConditions)
	assert.Len(t, result, 2) // Both existing conditions, one updated

	// Find the updated condition
	var updatedCond *metav1.Condition
	for i, cond := range result {
		if cond.Type == "ToUpdate" {
			updatedCond = &result[i]
			break
		}
	}
	assert.NotNil(t, updatedCond, "Updated condition should exist")
	assert.Equal(t, metav1.ConditionTrue, updatedCond.Status, "Status should be updated")
	assert.Equal(t, "NewReason", updatedCond.Reason, "Reason should be updated")
	assert.Equal(t, "Updated message", updatedCond.Message, "Message should be updated")

	// Test with unchanged condition
	unchangedConditions := []metav1.Condition{
		{Type: "Existing", Status: metav1.ConditionTrue, Reason: "InitialReason", Message: "Initial message"},
	}
	result = MergeConditionsIfChanged(ctx, workspace, &unchangedConditions)
	assert.Empty(t, result, "Should return empty slice when no changes")
}
