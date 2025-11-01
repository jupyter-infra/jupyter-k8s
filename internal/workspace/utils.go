// Package workspace provides utilities for workspace operations.
package workspace

import (
	"context"
	"fmt"

	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/controller"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// GetPodUIDFromWorkspaceName gets the pod UID for a given workspace name
func GetPodUIDFromWorkspaceName(clientset kubernetes.Interface, workspaceName string) (string, error) {
	// Get pods with the workspace label
	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
		LabelSelector: controller.LabelWorkspaceName + "=" + workspaceName,
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
