package controller

import (
	"testing"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResourceManager_IsWorkspaceAvailable(t *testing.T) {
	tests := []struct {
		name      string
		workspace *workspacev1alpha1.Workspace
		expected  bool
	}{
		{
			name: "workspace with Available=True condition",
			workspace: &workspacev1alpha1.Workspace{
				Status: workspacev1alpha1.WorkspaceStatus{
					Conditions: []metav1.Condition{
						{
							Type:   ConditionTypeAvailable,
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "workspace with Available=False condition",
			workspace: &workspacev1alpha1.Workspace{
				Status: workspacev1alpha1.WorkspaceStatus{
					Conditions: []metav1.Condition{
						{
							Type:   ConditionTypeAvailable,
							Status: metav1.ConditionFalse,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "workspace with no Available condition",
			workspace: &workspacev1alpha1.Workspace{
				Status: workspacev1alpha1.WorkspaceStatus{
					Conditions: []metav1.Condition{
						{
							Type:   ConditionTypeProgressing,
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "workspace with multiple conditions including Available=True",
			workspace: &workspacev1alpha1.Workspace{
				Status: workspacev1alpha1.WorkspaceStatus{
					Conditions: []metav1.Condition{
						{
							Type:   ConditionTypeProgressing,
							Status: metav1.ConditionFalse,
						},
						{
							Type:   ConditionTypeAvailable,
							Status: metav1.ConditionTrue,
						},
						{
							Type:   ConditionTypeDegraded,
							Status: metav1.ConditionFalse,
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "workspace with empty conditions",
			workspace: &workspacev1alpha1.Workspace{
				Status: workspacev1alpha1.WorkspaceStatus{
					Conditions: []metav1.Condition{},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, workspacev1alpha1.AddToScheme(scheme))
			require.NoError(t, appsv1.AddToScheme(scheme))
			require.NoError(t, corev1.AddToScheme(scheme))

			client := fake.NewClientBuilder().WithScheme(scheme).Build()
			rm := NewResourceManager(client, scheme, nil, nil, nil, nil, nil)

			result := rm.IsWorkspaceAvailable(tt.workspace)
			assert.Equal(t, tt.expected, result)
		})
	}
}
