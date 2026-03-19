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
	webhookconst "github.com/jupyter-infra/jupyter-k8s/internal/webhook"
)

var _ = Describe("TemplateValidator Namespace Scope", func() {
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

	Context("ValidateCreateWorkspace with namespace scope", func() {
		It("should reject cross-namespace templateRef when namespace has template-namespace-scope label set to Namespaced", func() {
			scopedNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "team-a",
					Labels: map[string]string{
						webhookconst.TemplateScopeNamespaceLabel: string(webhookconst.TemplateScopeNamespaced),
					},
				},
			}
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-template",
					Namespace: "team-b",
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DefaultImage: "jupyter/base-notebook:latest",
					DisplayName:  "Other Template",
				},
			}
			validator := buildValidator("", scopedNs, template)

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
				Spec: workspacev1alpha1.WorkspaceSpec{
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      "other-template",
						Namespace: "team-b",
					},
				},
			}

			err := validator.ValidateCreateWorkspace(ctx, workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("template-namespace-scope=Namespaced"))
			Expect(err.Error()).To(ContainSubstring("team-a"))
			Expect(err.Error()).To(ContainSubstring("team-b"))
		})

		It("should allow same-namespace templateRef when namespace has template-namespace-scope label set to Namespaced", func() {
			scopedNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "team-a",
					Labels: map[string]string{
						webhookconst.TemplateScopeNamespaceLabel: string(webhookconst.TemplateScopeNamespaced),
					},
				},
			}
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
			validator := buildValidator("", scopedNs, template)

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

		It("should allow cross-namespace templateRef when namespace has template-namespace-scope label set to Cluster", func() {
			unscopedNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "team-open",
					Labels: map[string]string{
						webhookconst.TemplateScopeNamespaceLabel: string(webhookconst.TemplateScopeCluster),
					},
				},
			}
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "shared-template",
					Namespace: "shared",
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DefaultImage: "jupyter/base-notebook:latest",
					DisplayName:  "Shared Template",
				},
			}
			validator := buildValidator("", unscopedNs, template)

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-open"},
				Spec: workspacev1alpha1.WorkspaceSpec{
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      "shared-template",
						Namespace: "shared",
					},
				},
			}

			err := validator.ValidateCreateWorkspace(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow cross-namespace templateRef when namespace has no template-namespace-scope label", func() {
			unscopedNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "team-default",
				},
			}
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "shared-template",
					Namespace: "shared",
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DefaultImage: "jupyter/base-notebook:latest",
					DisplayName:  "Shared Template",
				},
			}
			validator := buildValidator("", unscopedNs, template)

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-default"},
				Spec: workspacev1alpha1.WorkspaceSpec{
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      "shared-template",
						Namespace: "shared",
					},
				},
			}

			err := validator.ValidateCreateWorkspace(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow templateRef without explicit namespace when namespace has template-namespace-scope label set to Namespaced", func() {
			scopedNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "team-a",
					Labels: map[string]string{
						webhookconst.TemplateScopeNamespaceLabel: string(webhookconst.TemplateScopeNamespaced),
					},
				},
			}
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
			validator := buildValidator("", scopedNs, template)

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
				Spec: workspacev1alpha1.WorkspaceSpec{
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name: "local-template",
						// No namespace — resolver defaults to workspace namespace
					},
				},
			}

			err := validator.ValidateCreateWorkspace(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject implicit cross-namespace resolution via defaultTemplateNamespace fallback "+
			"when namespace has template-namespace-scope label set to Namespaced", func() {
			scopedNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "team-a",
					Labels: map[string]string{
						webhookconst.TemplateScopeNamespaceLabel: string(webhookconst.TemplateScopeNamespaced),
					},
				},
			}
			// Template only exists in the shared namespace, not in team-a
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
			validator := buildValidator("jupyter-k8s-shared", scopedNs, template)

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
				Spec: workspacev1alpha1.WorkspaceSpec{
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name: "shared-template",
						// No namespace — resolver falls back to defaultTemplateNamespace
					},
				},
			}

			err := validator.ValidateCreateWorkspace(ctx, workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("template-namespace-scope=Namespaced"))
			Expect(err.Error()).To(ContainSubstring("jupyter-k8s-shared"))
		})

		It("should skip scope check when workspace has no templateRef", func() {
			scopedNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "team-a",
					Labels: map[string]string{
						webhookconst.TemplateScopeNamespaceLabel: string(webhookconst.TemplateScopeNamespaced),
					},
				},
			}
			validator := buildValidator("", scopedNs)

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "team-a"},
				Spec:       workspacev1alpha1.WorkspaceSpec{},
			}

			err := validator.ValidateCreateWorkspace(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
