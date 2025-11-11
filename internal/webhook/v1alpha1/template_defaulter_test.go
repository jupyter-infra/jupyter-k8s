/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/controller"
)

var _ = Describe("TemplateDefaulter", func() {
	var (
		defaulter *TemplateDefaulter
		template  *workspacev1alpha1.WorkspaceTemplate
		workspace *workspacev1alpha1.Workspace
		ctx       context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()

		scheme := runtime.NewScheme()
		_ = workspacev1alpha1.AddToScheme(scheme)

		template = &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-template",
			},
			Spec: workspacev1alpha1.WorkspaceTemplateSpec{
				DefaultImage:         "jupyter/base-notebook:latest",
				DefaultOwnershipType: "Public",
				DefaultResources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
				PrimaryStorage: &workspacev1alpha1.StorageConfig{
					DefaultSize: resource.MustParse("1Gi"),
				},
				DefaultNodeSelector: map[string]string{
					"node-type": "compute",
				},
			},
		}

		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workspace",
				Namespace: "default",
			},
			Spec: workspacev1alpha1.WorkspaceSpec{
				DisplayName: "Test Workspace",
				TemplateRef: &workspacev1alpha1.TemplateRef{Name: template.Name},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(template).
			Build()

		defaulter = NewTemplateDefaulter(fakeClient)
	})

	Context("ApplyTemplateDefaults", func() {
		It("should apply all defaults when workspace has no values", func() {
			err := defaulter.ApplyTemplateDefaults(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())

			// Check core defaults
			Expect(workspace.Spec.Image).To(Equal("jupyter/base-notebook:latest"))
			Expect(workspace.Spec.OwnershipType).To(Equal("Public"))

			// Check resource defaults
			Expect(workspace.Spec.Resources).NotTo(BeNil())
			Expect(workspace.Spec.Resources.Requests[corev1.ResourceCPU]).To(Equal(resource.MustParse("100m")))

			// Check storage defaults
			Expect(workspace.Spec.Storage).NotTo(BeNil())
			Expect(workspace.Spec.Storage.Size).To(Equal(resource.MustParse("1Gi")))

			// Check scheduling defaults
			Expect(workspace.Spec.NodeSelector).To(HaveKeyWithValue("node-type", "compute"))

			// Check metadata defaults
			Expect(workspace.Labels).To(HaveKeyWithValue(controller.LabelWorkspaceTemplate, "test-template"))
		})

		It("should not override existing values", func() {
			workspace.Spec.Image = "custom/image:latest"
			workspace.Spec.Resources = &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("200m"),
				},
			}

			err := defaulter.ApplyTemplateDefaults(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())

			// Should keep existing values
			Expect(workspace.Spec.Image).To(Equal("custom/image:latest"))
			Expect(workspace.Spec.Resources.Requests[corev1.ResourceCPU]).To(Equal(resource.MustParse("200m")))

			// Should still apply other defaults
			Expect(workspace.Spec.OwnershipType).To(Equal("Public"))
			Expect(workspace.Labels).To(HaveKeyWithValue(controller.LabelWorkspaceTemplate, "test-template"))
		})

		It("should do nothing when no template reference", func() {
			workspace.Spec.TemplateRef = nil

			err := defaulter.ApplyTemplateDefaults(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())

			Expect(workspace.Spec.Image).To(BeEmpty())
			Expect(workspace.Spec.Resources).To(BeNil())
			Expect(workspace.Labels).To(BeNil())
		})

		It("should return error when template not found", func() {
			workspace.Spec.TemplateRef = &workspacev1alpha1.TemplateRef{Name: "non-existent-template"}

			err := defaulter.ApplyTemplateDefaults(ctx, workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get template"))
		})
	})
})
