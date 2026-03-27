/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

func buildTestValidator(defaultTemplateNamespace string, objects ...runtime.Object) *TemplateValidator {
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objects...).
		Build()
	return NewTemplateValidator(fakeClient, defaultTemplateNamespace)
}

func TestValidateTemplateNamespace_RejectsCrossNamespace(t *testing.T) {
	validator := buildTestValidator("jupyter-k8s-shared")
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
		Spec: workspacev1alpha1.WorkspaceSpec{
			TemplateRef: &workspacev1alpha1.TemplateRef{
				Name:      "some-template",
				Namespace: "team-b",
			},
		},
	}

	err := validator.validateTemplateNamespace(workspace)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "team-b")
	assert.Contains(t, err.Error(), "team-a")
	assert.Contains(t, err.Error(), "jupyter-k8s-shared")
}

func TestValidateTemplateNamespace_AllowsSameNamespace(t *testing.T) {
	validator := buildTestValidator("jupyter-k8s-shared")
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
		Spec: workspacev1alpha1.WorkspaceSpec{
			TemplateRef: &workspacev1alpha1.TemplateRef{
				Name:      "local-template",
				Namespace: "team-a",
			},
		},
	}

	err := validator.validateTemplateNamespace(workspace)
	assert.NoError(t, err)
}

func TestValidateTemplateNamespace_AllowsSharedNamespace(t *testing.T) {
	validator := buildTestValidator("jupyter-k8s-shared")
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
		Spec: workspacev1alpha1.WorkspaceSpec{
			TemplateRef: &workspacev1alpha1.TemplateRef{
				Name:      "shared-template",
				Namespace: "jupyter-k8s-shared",
			},
		},
	}

	err := validator.validateTemplateNamespace(workspace)
	assert.NoError(t, err)
}

func TestValidateTemplateNamespace_AllowsEmptyNamespace(t *testing.T) {
	validator := buildTestValidator("jupyter-k8s-shared")
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
		Spec: workspacev1alpha1.WorkspaceSpec{
			TemplateRef: &workspacev1alpha1.TemplateRef{
				Name: "some-template",
			},
		},
	}

	err := validator.validateTemplateNamespace(workspace)
	assert.NoError(t, err)
}

func TestValidateTemplateNamespace_RejectsCrossNamespaceWithNoSharedNamespace(t *testing.T) {
	validator := buildTestValidator("")
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
		Spec: workspacev1alpha1.WorkspaceSpec{
			TemplateRef: &workspacev1alpha1.TemplateRef{
				Name:      "some-template",
				Namespace: "team-b",
			},
		},
	}

	err := validator.validateTemplateNamespace(workspace)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "team-b")
	assert.Contains(t, err.Error(), "team-a")
	assert.NotContains(t, err.Error(), "shared")
}

func TestValidateCreateWorkspace_RejectsCrossNamespaceTemplateRef(t *testing.T) {
	template := &workspacev1alpha1.WorkspaceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-template",
			Namespace: "team-b",
		},
		Spec: workspacev1alpha1.WorkspaceTemplateSpec{
			DefaultImage: "jupyter/base-notebook:latest",
			DisplayName:  "Other Template",
		},
	}
	validator := buildTestValidator("jupyter-k8s-shared", template)

	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
		Spec: workspacev1alpha1.WorkspaceSpec{
			TemplateRef: &workspacev1alpha1.TemplateRef{
				Name:      "other-template",
				Namespace: "team-b",
			},
		},
	}

	err := validator.ValidateCreateWorkspace(context.Background(), workspace)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestValidateCreateWorkspace_AllowsSharedNamespaceTemplateRef(t *testing.T) {
	template := &workspacev1alpha1.WorkspaceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "shared-template",
			Namespace: "jupyter-k8s-shared",
		},
		Spec: workspacev1alpha1.WorkspaceTemplateSpec{
			DefaultImage: "jupyter/base-notebook:latest",
			DisplayName:  "Shared Template",
		},
	}
	validator := buildTestValidator("jupyter-k8s-shared", template)

	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
		Spec: workspacev1alpha1.WorkspaceSpec{
			TemplateRef: &workspacev1alpha1.TemplateRef{
				Name:      "shared-template",
				Namespace: "jupyter-k8s-shared",
			},
		},
	}

	err := validator.ValidateCreateWorkspace(context.Background(), workspace)
	assert.NoError(t, err)
}

func TestValidateCreateWorkspace_SkipsCheckWhenNoTemplateRef(t *testing.T) {
	validator := buildTestValidator("jupyter-k8s-shared")
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
		Spec:       workspacev1alpha1.WorkspaceSpec{},
	}

	err := validator.ValidateCreateWorkspace(context.Background(), workspace)
	assert.NoError(t, err)
}
