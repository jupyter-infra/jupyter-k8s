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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("SecurityDefaulter", func() {
	var (
		workspace *workspacev1alpha1.Workspace
		template  *workspacev1alpha1.WorkspaceTemplate
	)

	BeforeEach(func() {
		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workspace",
				Namespace: "default",
			},
			Spec: workspacev1alpha1.WorkspaceSpec{
				DisplayName: "Test Workspace",
			},
		}

		template = &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-template",
			},
			Spec: workspacev1alpha1.WorkspaceTemplateSpec{
				DisplayName:  "Test Template",
				DefaultImage: "test-image",
				DefaultPodSecurityContext: &corev1.PodSecurityContext{
					FSGroup: int64Ptr(1000),
				},
			},
		}
	})

	Context("applySecurityDefaults", func() {
		It("should apply pod security context from template when workspace has none", func() {
			applySecurityDefaults(workspace, template)

			Expect(workspace.Spec.PodSecurityContext).NotTo(BeNil())
			Expect(workspace.Spec.PodSecurityContext.FSGroup).NotTo(BeNil())
			Expect(*workspace.Spec.PodSecurityContext.FSGroup).To(Equal(int64(1000)))
		})

		It("should not override existing pod security context", func() {
			workspace.Spec.PodSecurityContext = &corev1.PodSecurityContext{
				FSGroup: int64Ptr(2000),
			}

			applySecurityDefaults(workspace, template)

			Expect(*workspace.Spec.PodSecurityContext.FSGroup).To(Equal(int64(2000)))
		})

		It("should do nothing when template has no security context", func() {
			template.Spec.DefaultPodSecurityContext = nil

			applySecurityDefaults(workspace, template)

			Expect(workspace.Spec.PodSecurityContext).To(BeNil())
		})
	})
})

func int64Ptr(i int64) *int64 {
	return &i
}
