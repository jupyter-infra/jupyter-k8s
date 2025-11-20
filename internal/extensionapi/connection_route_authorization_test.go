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

package extensionapi

import (
	"net/http"
	"testing"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	rlog "sigs.k8s.io/controller-runtime/pkg/log"
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
			AccessType: "Public",
		},
	}

	client := fake.NewClientBuilder().WithScheme(newTestScheme()).WithObjects(workspace).Build()
	logger := rlog.Log.WithName("test")
	server := &ExtensionServer{k8sClient: client, logger: &logger}

	req, _ := http.NewRequest("POST", "/test", nil)
	// Set user in Kubernetes authentication context
	userInfo := &user.DefaultInfo{Name: "test-user"}
	ctx := request.WithUser(req.Context(), userInfo)
	req = req.WithContext(ctx)

	result, err := server.checkWorkspaceAuthorization(req, "public-workspace", "default")

	assert.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.False(t, result.NotFound)
}

func TestCheckWorkspaceAuthorization_PrivateWorkspace_SameUser(t *testing.T) {
	// Create private workspace owned by test-user
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "private-workspace",
			Namespace: "default",
			Annotations: map[string]string{
				"workspace.jupyter.org/created-by": "test-user",
			},
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			AccessType: "OwnerOnly",
		},
	}

	client := fake.NewClientBuilder().WithScheme(newTestScheme()).WithObjects(workspace).Build()
	logger := rlog.Log.WithName("test")
	server := &ExtensionServer{k8sClient: client, logger: &logger}

	req, _ := http.NewRequest("POST", "/test", nil)
	// Set user in Kubernetes authentication context
	userInfo := &user.DefaultInfo{Name: "test-user"}
	ctx := request.WithUser(req.Context(), userInfo)
	req = req.WithContext(ctx)

	result, err := server.checkWorkspaceAuthorization(req, "private-workspace", "default")

	assert.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.False(t, result.NotFound)
}

func TestCheckWorkspaceAuthorization_PrivateWorkspace_DifferentUser(t *testing.T) {
	// Create private workspace owned by owner-user
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "private-workspace",
			Namespace: "default",
			Annotations: map[string]string{
				"workspace.jupyter.org/created-by": "owner-user",
			},
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			AccessType: "OwnerOnly",
		},
	}

	client := fake.NewClientBuilder().WithScheme(newTestScheme()).WithObjects(workspace).Build()
	logger := rlog.Log.WithName("test")
	server := &ExtensionServer{k8sClient: client, logger: &logger}

	req, _ := http.NewRequest("POST", "/test", nil)
	// Set user in Kubernetes authentication context
	userInfo := &user.DefaultInfo{Name: "different-user"}
	ctx := request.WithUser(req.Context(), userInfo)
	req = req.WithContext(ctx)

	result, err := server.checkWorkspaceAuthorization(req, "private-workspace", "default")

	assert.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.False(t, result.NotFound)
	assert.Contains(t, result.Reason, "not the workspace owner")
}

func TestCheckWorkspaceAuthorization_WorkspaceNotFound(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(newTestScheme()).Build()
	logger := rlog.Log.WithName("test")
	server := &ExtensionServer{k8sClient: client, logger: &logger}

	req, _ := http.NewRequest("POST", "/test", nil)
	// Set user in Kubernetes authentication context
	userInfo := &user.DefaultInfo{Name: "test-user"}
	ctx := request.WithUser(req.Context(), userInfo)
	req = req.WithContext(ctx)
	result, err := server.checkWorkspaceAuthorization(req, "non-existent", "default")

	assert.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.True(t, result.NotFound)
	assert.Contains(t, result.Reason, "Workspace not found")
}
