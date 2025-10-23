package extensionapi

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
)

func getPodUIDFromSpaceName(spaceName string) (string, error) {
	// Create Kubernetes client
	config := ctrl.GetConfigOrDie()
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "", err
	}

	// Get pods with the workspace label
	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
		LabelSelector: "workspace.workspaces.jupyter.org/name=" + spaceName,
	})
	if err != nil {
		return "", err
	}

	if len(pods.Items) == 0 {
		return "", fmt.Errorf("no pod found for space: %s", spaceName)
	}

	// Return the UID of the first pod
	return string(pods.Items[0].UID), nil
}
