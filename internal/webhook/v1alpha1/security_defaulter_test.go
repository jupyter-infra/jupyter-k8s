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

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
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
