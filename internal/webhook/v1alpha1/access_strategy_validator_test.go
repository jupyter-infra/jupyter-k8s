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
			validator := NewAccessStrategyValidator("jupyter-k8s-shared")

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name:      "some-strategy",
						Namespace: "team-b",
					},
				},
			}

			err := validator.ValidateCreateWorkspace(workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("team-b"))
			Expect(err.Error()).To(ContainSubstring("team-a"))
			Expect(err.Error()).To(ContainSubstring("jupyter-k8s-shared"))
		})

		It("should allow accessStrategy targeting the workspace's own namespace", func() {
			validator := NewAccessStrategyValidator("jupyter-k8s-shared")

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name:      "local-strategy",
						Namespace: "team-a",
					},
				},
			}

			err := validator.ValidateCreateWorkspace(workspace)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow accessStrategy targeting the shared namespace", func() {
			validator := NewAccessStrategyValidator("jupyter-k8s-shared")

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name:      "shared-strategy",
						Namespace: "jupyter-k8s-shared",
					},
				},
			}

			err := validator.ValidateCreateWorkspace(workspace)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow accessStrategy with empty namespace", func() {
			validator := NewAccessStrategyValidator("jupyter-k8s-shared")

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name: "some-strategy",
					},
				},
			}

			err := validator.ValidateCreateWorkspace(workspace)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject cross-namespace when no shared namespace is configured", func() {
			validator := NewAccessStrategyValidator("")

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name:      "some-strategy",
						Namespace: "jupyter-k8s-shared",
					},
				},
			}

			err := validator.ValidateCreateWorkspace(workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("jupyter-k8s-shared"))
			Expect(err.Error()).To(ContainSubstring("team-a"))
			Expect(err.Error()).NotTo(ContainSubstring("shared namespace"))
		})

		It("should skip validation when workspace has no accessStrategy", func() {
			validator := NewAccessStrategyValidator("jupyter-k8s-shared")

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
				Spec:       workspacev1alpha1.WorkspaceSpec{},
			}

			err := validator.ValidateCreateWorkspace(workspace)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Update validation", func() {
		It("should validate when accessStrategyRef is added", func() {
			validator := NewAccessStrategyValidator("jupyter-k8s-shared")

			oldWorkspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
				Spec:       workspacev1alpha1.WorkspaceSpec{},
			}
			newWorkspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name:      "some-strategy",
						Namespace: "team-b",
					},
				},
			}

			err := validator.ValidateUpdateWorkspace(oldWorkspace, newWorkspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("team-b"))
		})

		It("should skip validation when accessStrategyRef is removed", func() {
			validator := NewAccessStrategyValidator("jupyter-k8s-shared")

			oldWorkspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name:      "some-strategy",
						Namespace: "team-b",
					},
				},
			}
			newWorkspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
				Spec:       workspacev1alpha1.WorkspaceSpec{},
			}

			err := validator.ValidateUpdateWorkspace(oldWorkspace, newWorkspace)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should validate when accessStrategyRef namespace changes", func() {
			validator := NewAccessStrategyValidator("jupyter-k8s-shared")

			oldWorkspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name:      "some-strategy",
						Namespace: "team-a",
					},
				},
			}
			newWorkspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name:      "some-strategy",
						Namespace: "team-b",
					},
				},
			}

			err := validator.ValidateUpdateWorkspace(oldWorkspace, newWorkspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("team-b"))
		})
	})
})
