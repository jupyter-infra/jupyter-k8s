/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("TemplateValidator", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	buildValidator := func(defaultTemplateNamespace string, objects ...runtime.Object) *TemplateValidator {
		scheme := runtime.NewScheme()
		_ = workspacev1alpha1.AddToScheme(scheme)
		_ = corev1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithRuntimeObjects(objects...).
			Build()
		return NewTemplateValidator(fakeClient, defaultTemplateNamespace)
	}

	Context("Namespace scope validation", func() {
		It("should reject templateRef targeting another team's namespace", func() {
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-template",
					Namespace: "team-b",
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DefaultImage: "jupyter/base-notebook:latest",
					DisplayName:  "Some Template",
				},
			}
			validator := buildValidator("jupyter-k8s-shared", template)

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
				Spec: workspacev1alpha1.WorkspaceSpec{
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      "some-template",
						Namespace: "team-b",
					},
				},
			}

			err := validator.ValidateCreateWorkspace(ctx, workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("team-b"))
			Expect(err.Error()).To(ContainSubstring("team-a"))
			Expect(err.Error()).To(ContainSubstring("jupyter-k8s-shared"))
		})

		It("should allow templateRef targeting the workspace's own namespace", func() {
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "local-template",
					Namespace: "team-a",
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DefaultImage: "jupyter/base-notebook:latest",
					DisplayName:  "Local Template",
				},
			}
			validator := buildValidator("jupyter-k8s-shared", template)

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
				Spec: workspacev1alpha1.WorkspaceSpec{
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      "local-template",
						Namespace: "team-a",
					},
				},
			}

			err := validator.ValidateCreateWorkspace(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow templateRef targeting the shared namespace", func() {
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
			validator := buildValidator("jupyter-k8s-shared", template)

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
				Spec: workspacev1alpha1.WorkspaceSpec{
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      "shared-template",
						Namespace: "jupyter-k8s-shared",
					},
				},
			}

			err := validator.ValidateCreateWorkspace(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow templateRef with empty namespace", func() {
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-template",
					Namespace: "team-a",
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DefaultImage: "jupyter/base-notebook:latest",
					DisplayName:  "Some Template",
				},
			}
			validator := buildValidator("jupyter-k8s-shared", template)

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
				Spec: workspacev1alpha1.WorkspaceSpec{
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name: "some-template",
					},
				},
			}

			err := validator.ValidateCreateWorkspace(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject cross-namespace templateRef when no shared namespace is configured", func() {
			validator := buildValidator("")

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
				Spec: workspacev1alpha1.WorkspaceSpec{
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      "some-template",
						Namespace: "team-b",
					},
				},
			}

			err := validator.ValidateCreateWorkspace(ctx, workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("team-b"))
			Expect(err.Error()).To(ContainSubstring("team-a"))
			Expect(err.Error()).NotTo(ContainSubstring("shared"))
		})

		It("should skip validation when workspace has no templateRef", func() {
			validator := buildValidator("jupyter-k8s-shared")

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
				Spec:       workspacev1alpha1.WorkspaceSpec{},
			}

			err := validator.ValidateCreateWorkspace(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
