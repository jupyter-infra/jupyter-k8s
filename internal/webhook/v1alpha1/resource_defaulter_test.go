/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("ResourceDefaulter", func() {
	var (
		template  *workspacev1alpha1.WorkspaceTemplate
		workspace *workspacev1alpha1.Workspace
	)

	BeforeEach(func() {
		template = &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "test-template"},
			Spec: workspacev1alpha1.WorkspaceTemplateSpec{
				DefaultResources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
				},
			},
		}

		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: "test-workspace"},
			Spec:       workspacev1alpha1.WorkspaceSpec{DisplayName: "Test"},
		}
	})

	Context("applyResourceDefaults", func() {
		It("should apply resource defaults when nil", func() {
			applyResourceDefaults(workspace, template)

			Expect(workspace.Spec.Resources).NotTo(BeNil())
			Expect(workspace.Spec.Resources.Requests[corev1.ResourceCPU]).To(Equal(resource.MustParse("200m")))
			Expect(workspace.Spec.Resources.Requests[corev1.ResourceMemory]).To(Equal(resource.MustParse("256Mi")))
			Expect(workspace.Spec.Resources.Limits[corev1.ResourceCPU]).To(Equal(resource.MustParse("500m")))
			Expect(workspace.Spec.Resources.Limits[corev1.ResourceMemory]).To(Equal(resource.MustParse("512Mi")))
		})

		It("should not override existing resources", func() {
			workspace.Spec.Resources = &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("100m"),
				},
			}

			applyResourceDefaults(workspace, template)

			Expect(workspace.Spec.Resources.Requests[corev1.ResourceCPU]).To(Equal(resource.MustParse("100m")))
			Expect(workspace.Spec.Resources.Limits).To(BeNil())
		})

		It("should do nothing when template has no default resources", func() {
			template.Spec.DefaultResources = nil

			applyResourceDefaults(workspace, template)

			Expect(workspace.Spec.Resources).To(BeNil())
		})

		It("should create independent copy (deep copy test)", func() {
			applyResourceDefaults(workspace, template)

			// Modify workspace resources
			workspace.Spec.Resources.Requests[corev1.ResourceCPU] = resource.MustParse("1000m")

			// Template should remain unchanged
			Expect(template.Spec.DefaultResources.Requests[corev1.ResourceCPU]).To(Equal(resource.MustParse("200m")))
		})
	})
})
