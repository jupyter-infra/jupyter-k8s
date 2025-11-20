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

var _ = Describe("SchedulingDefaulter", func() {
	var (
		template  *workspacev1alpha1.WorkspaceTemplate
		workspace *workspacev1alpha1.Workspace
	)

	BeforeEach(func() {
		template = &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "test-template"},
			Spec: workspacev1alpha1.WorkspaceTemplateSpec{
				DefaultNodeSelector: map[string]string{
					"node-type":   "compute",
					"environment": "production",
				},
				DefaultAffinity: &corev1.Affinity{
					NodeAffinity: &corev1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
							NodeSelectorTerms: []corev1.NodeSelectorTerm{
								{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "kubernetes.io/arch",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"amd64"},
										},
									},
								},
							},
						},
					},
				},
				DefaultTolerations: []corev1.Toleration{
					{
						Key:      "node.kubernetes.io/not-ready",
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoExecute,
					},
					{
						Key:      "dedicated",
						Operator: corev1.TolerationOpEqual,
						Value:    "jupyter",
						Effect:   corev1.TaintEffectNoSchedule,
					},
				},
			},
		}

		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: "test-workspace"},
			Spec:       workspacev1alpha1.WorkspaceSpec{DisplayName: "Test"},
		}
	})

	Context("applySchedulingDefaults", func() {
		It("should apply node selector defaults when nil", func() {
			applySchedulingDefaults(workspace, template)

			Expect(workspace.Spec.NodeSelector).NotTo(BeNil())
			Expect(workspace.Spec.NodeSelector).To(HaveKeyWithValue("node-type", "compute"))
			Expect(workspace.Spec.NodeSelector).To(HaveKeyWithValue("environment", "production"))
		})

		It("should not override existing node selector", func() {
			workspace.Spec.NodeSelector = map[string]string{
				"existing": "value",
			}

			applySchedulingDefaults(workspace, template)

			Expect(workspace.Spec.NodeSelector).To(HaveKeyWithValue("existing", "value"))
			Expect(workspace.Spec.NodeSelector).NotTo(HaveKey("node-type"))
		})

		It("should apply affinity defaults when nil", func() {
			applySchedulingDefaults(workspace, template)

			Expect(workspace.Spec.Affinity).NotTo(BeNil())
			Expect(workspace.Spec.Affinity.NodeAffinity).NotTo(BeNil())
			Expect(workspace.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).NotTo(BeNil())
		})

		It("should not override existing affinity", func() {
			workspace.Spec.Affinity = &corev1.Affinity{
				PodAffinity: &corev1.PodAffinity{},
			}

			applySchedulingDefaults(workspace, template)

			Expect(workspace.Spec.Affinity.PodAffinity).NotTo(BeNil())
			Expect(workspace.Spec.Affinity.NodeAffinity).To(BeNil())
		})

		It("should apply tolerations defaults when nil", func() {
			applySchedulingDefaults(workspace, template)

			Expect(workspace.Spec.Tolerations).To(HaveLen(2))
			Expect(workspace.Spec.Tolerations[0].Key).To(Equal("node.kubernetes.io/not-ready"))
			Expect(workspace.Spec.Tolerations[1].Key).To(Equal("dedicated"))
			Expect(workspace.Spec.Tolerations[1].Value).To(Equal("jupyter"))
		})

		It("should not override existing tolerations", func() {
			workspace.Spec.Tolerations = []corev1.Toleration{
				{
					Key:    "existing",
					Effect: corev1.TaintEffectNoSchedule,
				},
			}

			applySchedulingDefaults(workspace, template)

			Expect(workspace.Spec.Tolerations).To(HaveLen(1))
			Expect(workspace.Spec.Tolerations[0].Key).To(Equal("existing"))
		})

		It("should create independent copies (deep copy test)", func() {
			applySchedulingDefaults(workspace, template)

			// Modify workspace node selector
			workspace.Spec.NodeSelector["node-type"] = "modified"

			// Template should remain unchanged
			Expect(template.Spec.DefaultNodeSelector["node-type"]).To(Equal("compute"))

			// Modify workspace tolerations
			workspace.Spec.Tolerations[0].Key = "modified"

			// Template should remain unchanged
			Expect(template.Spec.DefaultTolerations[0].Key).To(Equal("node.kubernetes.io/not-ready"))
		})

		It("should handle template with no scheduling defaults", func() {
			template.Spec.DefaultNodeSelector = nil
			template.Spec.DefaultAffinity = nil
			template.Spec.DefaultTolerations = nil

			applySchedulingDefaults(workspace, template)

			Expect(workspace.Spec.NodeSelector).To(BeNil())
			Expect(workspace.Spec.Affinity).To(BeNil())
			Expect(workspace.Spec.Tolerations).To(BeNil())
		})

		It("should handle empty node selector in template", func() {
			template.Spec.DefaultNodeSelector = map[string]string{}

			applySchedulingDefaults(workspace, template)

			Expect(workspace.Spec.NodeSelector).NotTo(BeNil())
			Expect(workspace.Spec.NodeSelector).To(BeEmpty())
		})

		It("should handle empty tolerations in template", func() {
			template.Spec.DefaultTolerations = []corev1.Toleration{}

			applySchedulingDefaults(workspace, template)

			Expect(workspace.Spec.Tolerations).NotTo(BeNil())
			Expect(workspace.Spec.Tolerations).To(BeEmpty())
		})
	})
})
