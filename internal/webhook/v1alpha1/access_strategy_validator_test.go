/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("AccessStrategyValidator", func() {
	Context("Namespace scope validation", func() {
		It("should reject accessStrategy targeting another team's namespace", func() {
			validator := NewAccessStrategyValidator(testSharedNamespace)

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: testNamespaceTeamA},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name:      testSomeStrategy,
						Namespace: testNamespaceTeamB,
					},
				},
			}

			err := validator.ValidateCreateWorkspace(workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(testNamespaceTeamB))
			Expect(err.Error()).To(ContainSubstring(testNamespaceTeamA))
			Expect(err.Error()).To(ContainSubstring(testSharedNamespace))
		})

		It("should allow accessStrategy targeting the workspace's own namespace", func() {
			validator := NewAccessStrategyValidator(testSharedNamespace)

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: testNamespaceTeamA},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name:      "local-strategy",
						Namespace: testNamespaceTeamA,
					},
				},
			}

			err := validator.ValidateCreateWorkspace(workspace)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow accessStrategy targeting the shared namespace", func() {
			validator := NewAccessStrategyValidator(testSharedNamespace)

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: testNamespaceTeamA},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name:      "shared-strategy",
						Namespace: testSharedNamespace,
					},
				},
			}

			err := validator.ValidateCreateWorkspace(workspace)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow accessStrategy with empty namespace", func() {
			validator := NewAccessStrategyValidator(testSharedNamespace)

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: testNamespaceTeamA},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name: testSomeStrategy,
					},
				},
			}

			err := validator.ValidateCreateWorkspace(workspace)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject cross-namespace when no shared namespace is configured", func() {
			validator := NewAccessStrategyValidator("")

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: testNamespaceTeamA},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name:      testSomeStrategy,
						Namespace: testSharedNamespace,
					},
				},
			}

			err := validator.ValidateCreateWorkspace(workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(testSharedNamespace))
			Expect(err.Error()).To(ContainSubstring(testNamespaceTeamA))
			Expect(err.Error()).NotTo(ContainSubstring("shared namespace"))
		})

		It("should skip validation when workspace has no accessStrategy", func() {
			validator := NewAccessStrategyValidator(testSharedNamespace)

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: testNamespaceTeamA},
				Spec:       workspacev1alpha1.WorkspaceSpec{},
			}

			err := validator.ValidateCreateWorkspace(workspace)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Update validation", func() {
		It("should validate when accessStrategyRef is added", func() {
			validator := NewAccessStrategyValidator(testSharedNamespace)

			oldWorkspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: testNamespaceTeamA},
				Spec:       workspacev1alpha1.WorkspaceSpec{},
			}
			newWorkspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: testNamespaceTeamA},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name:      testSomeStrategy,
						Namespace: testNamespaceTeamB,
					},
				},
			}

			err := validator.ValidateUpdateWorkspace(oldWorkspace, newWorkspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(testNamespaceTeamB))
		})

		It("should skip validation when accessStrategyRef is removed", func() {
			validator := NewAccessStrategyValidator(testSharedNamespace)

			oldWorkspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: testNamespaceTeamA},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name:      testSomeStrategy,
						Namespace: testNamespaceTeamB,
					},
				},
			}
			newWorkspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: testNamespaceTeamA},
				Spec:       workspacev1alpha1.WorkspaceSpec{},
			}

			err := validator.ValidateUpdateWorkspace(oldWorkspace, newWorkspace)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should validate when accessStrategyRef namespace changes", func() {
			validator := NewAccessStrategyValidator(testSharedNamespace)

			oldWorkspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: testNamespaceTeamA},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name:      testSomeStrategy,
						Namespace: testNamespaceTeamA,
					},
				},
			}
			newWorkspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: testNamespaceTeamA},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name:      testSomeStrategy,
						Namespace: testNamespaceTeamB,
					},
				},
			}

			err := validator.ValidateUpdateWorkspace(oldWorkspace, newWorkspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(testNamespaceTeamB))
		})
	})

	Context("Template namespace scope validation", func() {
		// All template cases use namespace "team-a"; only the access strategy namespace varies.
		templateWithAS := func(asNamespace string) *workspacev1alpha1.WorkspaceTemplate {
			return &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: testTemplateNameTmpl, Namespace: testNamespaceTeamA},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DefaultAccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name:      testSomeStrategy,
						Namespace: asNamespace,
					},
				},
			}
		}

		It("should reject defaultAccessStrategy targeting another team's namespace", func() {
			validator := NewAccessStrategyValidator(testSharedNamespace)

			err := validator.ValidateCreateTemplate(templateWithAS(testNamespaceTeamB))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(testNamespaceTeamB))
			Expect(err.Error()).To(ContainSubstring(testNamespaceTeamA))
			Expect(err.Error()).To(ContainSubstring(testSharedNamespace))
			Expect(err.Error()).To(ContainSubstring("template namespace"))
		})

		It("should allow defaultAccessStrategy targeting the template's own namespace", func() {
			validator := NewAccessStrategyValidator(testSharedNamespace)

			err := validator.ValidateCreateTemplate(templateWithAS(testNamespaceTeamA))
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow defaultAccessStrategy targeting the shared namespace", func() {
			validator := NewAccessStrategyValidator(testSharedNamespace)

			err := validator.ValidateCreateTemplate(templateWithAS(testSharedNamespace))
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow defaultAccessStrategy with empty namespace", func() {
			validator := NewAccessStrategyValidator(testSharedNamespace)

			err := validator.ValidateCreateTemplate(templateWithAS(""))
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject cross-namespace when no shared namespace is configured", func() {
			validator := NewAccessStrategyValidator("")

			err := validator.ValidateCreateTemplate(templateWithAS(testSharedNamespace))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(testSharedNamespace))
			Expect(err.Error()).To(ContainSubstring(testNamespaceTeamA))
			Expect(err.Error()).NotTo(ContainSubstring("shared namespace"))
		})

		It("should skip validation when template has no defaultAccessStrategy", func() {
			validator := NewAccessStrategyValidator(testSharedNamespace)

			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: testTemplateNameTmpl, Namespace: testNamespaceTeamA},
			}
			err := validator.ValidateCreateTemplate(template)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should validate when defaultAccessStrategy namespace changes to a foreign namespace", func() {
			validator := NewAccessStrategyValidator(testSharedNamespace)

			err := validator.ValidateUpdateTemplate(
				templateWithAS(testNamespaceTeamA),
				templateWithAS(testNamespaceTeamB))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(testNamespaceTeamB))
		})

		It("should skip validation when defaultAccessStrategy is removed", func() {
			validator := NewAccessStrategyValidator(testSharedNamespace)

			newTemplate := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: testTemplateNameTmpl, Namespace: testNamespaceTeamA},
			}
			err := validator.ValidateUpdateTemplate(templateWithAS(testNamespaceTeamB), newTemplate)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
