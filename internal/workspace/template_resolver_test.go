/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package workspace

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

const (
	workspaceNamespaceName = "workspace-ns"
	customNamespaceName    = "custom-ns"
	defaultTemplateNsName  = "default-ns"
)

func TestNewTemplateResolver(t *testing.T) {
	k8sClient := fake.NewClientBuilder().Build()
	resolver := NewTemplateResolver(k8sClient, defaultTemplateNsName)

	assert.NotNil(t, resolver)
	assert.Equal(t, defaultTemplateNsName, resolver.defaultTemplateNamespace)
	assert.Equal(t, k8sClient, resolver.client)
}

func TestResolveTemplate(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, workspacev1alpha1.AddToScheme(scheme))

	tests := []struct {
		name                     string
		templateRef              *workspacev1alpha1.TemplateRef
		workspaceNamespace       string
		defaultTemplateNamespace string
		existingTemplates        []client.Object
		mockError                error
		expectedNamespace        string
		expectError              bool
		errorContains            string
	}{
		{
			name:          "nil templateRef",
			templateRef:   nil,
			expectError:   true,
			errorContains: "templateRef is nil",
		},
		{
			name: "template found in specified namespace",
			templateRef: &workspacev1alpha1.TemplateRef{
				Name:      testTemplateName,
				Namespace: customNamespaceName,
			},
			workspaceNamespace: workspaceNamespaceName,
			existingTemplates: []client.Object{
				&workspacev1alpha1.WorkspaceTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testTemplateName,
						Namespace: customNamespaceName,
					},
				},
			},
			expectedNamespace: customNamespaceName,
		},
		{
			name: "template found in workspace namespace when templateRef namespace empty",
			templateRef: &workspacev1alpha1.TemplateRef{
				Name: testTemplateName,
			},
			workspaceNamespace: workspaceNamespaceName,
			existingTemplates: []client.Object{
				&workspacev1alpha1.WorkspaceTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testTemplateName,
						Namespace: workspaceNamespaceName,
					},
				},
			},
			expectedNamespace: workspaceNamespaceName,
		},
		{
			name: "template found in default namespace via fallback",
			templateRef: &workspacev1alpha1.TemplateRef{
				Name: testTemplateName,
			},
			workspaceNamespace:       workspaceNamespaceName,
			defaultTemplateNamespace: defaultTemplateNsName,
			existingTemplates: []client.Object{
				&workspacev1alpha1.WorkspaceTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testTemplateName,
						Namespace: defaultTemplateNsName,
					},
				},
			},
			expectedNamespace: defaultTemplateNsName,
		},
		{
			name: "template found in primary namespace, fallback not used even when default exists",
			templateRef: &workspacev1alpha1.TemplateRef{
				Name: testTemplateName,
			},
			workspaceNamespace:       workspaceNamespaceName,
			defaultTemplateNamespace: defaultTemplateNsName,
			existingTemplates: []client.Object{
				&workspacev1alpha1.WorkspaceTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testTemplateName,
						Namespace: workspaceNamespaceName,
					},
				},
				&workspacev1alpha1.WorkspaceTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testTemplateName,
						Namespace: defaultTemplateNsName,
					},
				},
			},
			expectedNamespace: workspaceNamespaceName,
		},
		{
			name: "template not found anywhere",
			templateRef: &workspacev1alpha1.TemplateRef{
				Name: "missing-template",
			},
			workspaceNamespace:       workspaceNamespaceName,
			defaultTemplateNamespace: defaultTemplateNsName,
			existingTemplates:        []client.Object{},
			expectError:              true,
			errorContains:            "failed to get template missing-template",
		},
		{
			name: "no fallback when no default namespace configured",
			templateRef: &workspacev1alpha1.TemplateRef{
				Name: testTemplateName,
			},
			workspaceNamespace: workspaceNamespaceName,
			existingTemplates:  []client.Object{},
			expectError:        true,
			errorContains:      "failed to get template test-template",
		},
		{
			name: "non-NotFound error should be lifted up without fallback",
			templateRef: &workspacev1alpha1.TemplateRef{
				Name: testTemplateName,
			},
			workspaceNamespace:       workspaceNamespaceName,
			defaultTemplateNamespace: defaultTemplateNsName,
			mockError:                apierrors.NewForbidden(schema.GroupResource{}, testTemplateName, nil),
			expectError:              true,
			errorContains:            "failed to get template test-template",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var k8sClient client.Client

			if tt.mockError != nil {
				k8sClient = &mockClient{
					getFunc: func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						return tt.mockError
					},
				}
			} else {
				k8sClient = fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(tt.existingTemplates...).
					Build()
			}

			resolver := NewTemplateResolver(k8sClient, tt.defaultTemplateNamespace)

			template, err := resolver.ResolveTemplate(context.Background(), tt.templateRef, tt.workspaceNamespace)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, template)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, template)
				assert.Equal(t, tt.templateRef.Name, template.Name)
				assert.Equal(t, tt.expectedNamespace, template.Namespace)
			}
		})
	}
}

func TestResolveTemplate_NonNotFoundError(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, workspacev1alpha1.AddToScheme(scheme))

	// Create a mock client that returns a non-NotFound error
	k8sClient := &mockClient{
		getFunc: func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			// Return a permission error instead of NotFound
			return apierrors.NewForbidden(schema.GroupResource{}, key.Name, nil)
		},
	}

	resolver := NewTemplateResolver(k8sClient, defaultTemplateNsName)
	templateRef := &workspacev1alpha1.TemplateRef{Name: testTemplateName}

	template, err := resolver.ResolveTemplate(context.Background(), templateRef, workspaceNamespaceName)

	// Should return the original error, not attempt fallback
	assert.Error(t, err)
	assert.Nil(t, template)
	assert.Contains(t, err.Error(), "failed to get template test-template")
}

func TestResolveTemplateForWorkspace(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, workspacev1alpha1.AddToScheme(scheme))

	tests := []struct {
		name          string
		workspace     *workspacev1alpha1.Workspace
		expectError   bool
		errorContains string
	}{
		{
			name: "workspace with valid templateRef",
			workspace: &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkspaceName,
					Namespace: workspaceNamespaceName,
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name: testTemplateName,
					},
				},
			},
		},
		{
			name: "workspace with nil templateRef",
			workspace: &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkspaceName,
					Namespace: workspaceNamespaceName,
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					TemplateRef: nil,
				},
			},
			expectError:   true,
			errorContains: "workspace has no templateRef",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			existingTemplates := []client.Object{
				&workspacev1alpha1.WorkspaceTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testTemplateName,
						Namespace: workspaceNamespaceName,
					},
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(existingTemplates...).
				Build()

			resolver := NewTemplateResolver(k8sClient, "")

			template, err := resolver.ResolveTemplateForWorkspace(context.Background(), tt.workspace)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, template)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, template)
			}
		})
	}
}

// mockClient for testing non-NotFound errors
type mockClient struct {
	client.Client
	getFunc func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error
}

func (m *mockClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if m.getFunc != nil {
		return m.getFunc(ctx, key, obj, opts...)
	}
	return nil
}
