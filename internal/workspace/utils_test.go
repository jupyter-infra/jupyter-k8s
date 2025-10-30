package workspace

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetPodUIDFromWorkspaceName_Success(t *testing.T) {
	// Create fake clientset with a pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       types.UID("test-pod-uid-123"),
			Labels: map[string]string{
				"workspace.workspaces.jupyter.org/name": "test-workspace",
			},
		},
	}

	clientset := fake.NewSimpleClientset(pod)

	uid, err := GetPodUIDFromWorkspaceName(clientset, "test-workspace")

	assert.NoError(t, err)
	assert.Equal(t, "test-pod-uid-123", uid)
}

func TestGetPodUIDFromWorkspaceName_NoPodFound(t *testing.T) {
	clientset := fake.NewSimpleClientset()

	uid, err := GetPodUIDFromWorkspaceName(clientset, "nonexistent-workspace")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no pod found for workspace")
	assert.Empty(t, uid)
}
