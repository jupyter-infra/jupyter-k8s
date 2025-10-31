package extensionapi

import (
	"net/http"
	"testing"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)
	return scheme
}

func TestCheckWorkspaceAuthorization_PublicWorkspace(t *testing.T) {
	// Create public workspace
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "public-workspace",
			Namespace: "default",
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			OwnershipType: "Public",
		},
	}

	client := fake.NewClientBuilder().WithScheme(newTestScheme()).WithObjects(workspace).Build()
	server := &ExtensionServer{k8sClient: client}

	req, _ := http.NewRequest("POST", "/test", nil)
	err := server.checkWorkspaceAuthorization(req, "public-workspace", "default")

	assert.NoError(t, err)
}

func TestCheckWorkspaceAuthorization_PrivateWorkspace_SameUser(t *testing.T) {
	// Create private workspace owned by test-user
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "private-workspace",
			Namespace: "default",
			Annotations: map[string]string{
				"workspace.jupyter.org/created-by": "arn:aws:sts::123456789012:assumed-role/Admin/test-user",
			},
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			OwnershipType: "OwnerOnly",
		},
	}

	client := fake.NewClientBuilder().WithScheme(newTestScheme()).WithObjects(workspace).Build()
	server := &ExtensionServer{k8sClient: client}

	req, _ := http.NewRequest("POST", "/test", nil)
	req.Header.Set("X-User", "arn:aws:sts::123456789012:assumed-role/Admin/test-user")

	err := server.checkWorkspaceAuthorization(req, "private-workspace", "default")

	assert.NoError(t, err)
}

func TestCheckWorkspaceAuthorization_PrivateWorkspace_DifferentUser(t *testing.T) {
	// Create private workspace owned by owner-user
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "private-workspace",
			Namespace: "default",
			Annotations: map[string]string{
				"workspace.jupyter.org/created-by": "arn:aws:sts::123456789012:assumed-role/Admin/owner-user",
			},
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			OwnershipType: "OwnerOnly",
		},
	}

	client := fake.NewClientBuilder().WithScheme(newTestScheme()).WithObjects(workspace).Build()
	server := &ExtensionServer{k8sClient: client}

	req, _ := http.NewRequest("POST", "/test", nil)
	req.Header.Set("X-User", "arn:aws:sts::123456789012:assumed-role/Admin/different-user")

	err := server.checkWorkspaceAuthorization(req, "private-workspace", "default")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}

func TestCheckWorkspaceAuthorization_PrivateWorkspace_NoOwnerInfo(t *testing.T) {
	// Create private workspace without owner annotation
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "private-workspace",
			Namespace: "default",
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			OwnershipType: "OwnerOnly",
		},
	}

	client := fake.NewClientBuilder().WithScheme(newTestScheme()).WithObjects(workspace).Build()
	server := &ExtensionServer{k8sClient: client}

	req, _ := http.NewRequest("POST", "/test", nil)
	err := server.checkWorkspaceAuthorization(req, "private-workspace", "default")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "workspace owner information not found")
}
