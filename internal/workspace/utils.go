// Package workspace provides utilities for workspace operations.
package workspace

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Label keys - defined here to avoid import cycles with controller package
const (
	LabelWorkspaceName         = "workspace.jupyter.org/workspaceName"
	LabelComplianceCheckNeeded = "workspace.jupyter.org/compliance-check-needed"
)

// GetPodUIDFromWorkspaceName gets the pod UID for a given workspace name
func GetPodUIDFromWorkspaceName(k8sClient client.Client, workspaceName string) (string, error) {
	// Get pods with the workspace label
	podList := &corev1.PodList{}
	err := k8sClient.List(context.TODO(), podList, client.MatchingLabels{
		LabelWorkspaceName: workspaceName,
	})
	if err != nil {
		return "", err
	}

	if len(podList.Items) == 0 {
		return "", fmt.Errorf("no pod found for workspace: %s", workspaceName)
	}

	// Return the UID of the first pod
	return string(podList.Items[0].UID), nil
}
