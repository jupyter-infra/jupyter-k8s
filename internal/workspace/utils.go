// Package workspace provides utilities for workspace operations.
package workspace

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
)

// GetPodUIDFromWorkspaceName gets the pod UID for a given workspace name
func GetPodUIDFromWorkspaceName(workspaceName string) (string, error) {
	// Create Kubernetes client
	config := ctrl.GetConfigOrDie()
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "", err
	}

	// Get pods with the workspace label
	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
		LabelSelector: "workspace.workspaces.jupyter.org/name=" + workspaceName,
	})
	if err != nil {
		return "", err
	}

	if len(pods.Items) == 0 {
		return "", fmt.Errorf("no pod found for workspace: %s", workspaceName)
	}

	// Return the UID of the first pod
	return string(pods.Items[0].UID), nil
}
