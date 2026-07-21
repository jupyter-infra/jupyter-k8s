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
					Name:      testSomeTemplate,
					Namespace: testNamespaceTeamB,
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DefaultImage: testValidBaseNotebook,
					DisplayName:  "Some Template",
				},
			}
			validator := buildValidator(testSharedNamespace, template)

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: testNamespaceTeamA},
				Spec: workspacev1alpha1.WorkspaceSpec{
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      testSomeTemplate,
						Namespace: testNamespaceTeamB,
					},
				},
			}

			err := validator.ValidateCreateWorkspace(ctx, workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(testNamespaceTeamB))
			Expect(err.Error()).To(ContainSubstring(testNamespaceTeamA))
			Expect(err.Error()).To(ContainSubstring(testSharedNamespace))
		})

		It("should allow templateRef targeting the workspace's own namespace", func() {
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "local-template",
					Namespace: testNamespaceTeamA,
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DefaultImage: testValidBaseNotebook,
					DisplayName:  "Local Template",
				},
			}
			validator := buildValidator(testSharedNamespace, template)

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: testNamespaceTeamA},
				Spec: workspacev1alpha1.WorkspaceSpec{
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      "local-template",
						Namespace: testNamespaceTeamA,
					},
				},
			}

			err := validator.ValidateCreateWorkspace(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow templateRef targeting the shared namespace", func() {
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testSharedTemplate,
					Namespace: testSharedNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DefaultImage: testValidBaseNotebook,
					DisplayName:  "Shared Template",
				},
			}
			validator := buildValidator(testSharedNamespace, template)

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: testNamespaceTeamA},
				Spec: workspacev1alpha1.WorkspaceSpec{
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      testSharedTemplate,
						Namespace: testSharedNamespace,
					},
				},
			}

			err := validator.ValidateCreateWorkspace(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow templateRef with empty namespace", func() {
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testSomeTemplate,
					Namespace: testNamespaceTeamA,
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DefaultImage: testValidBaseNotebook,
					DisplayName:  "Some Template",
				},
			}
			validator := buildValidator(testSharedNamespace, template)

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: testNamespaceTeamA},
				Spec: workspacev1alpha1.WorkspaceSpec{
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name: testSomeTemplate,
					},
				},
			}

			err := validator.ValidateCreateWorkspace(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject cross-namespace templateRef when no shared namespace is configured", func() {
			validator := buildValidator("")

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: testNamespaceTeamA},
				Spec: workspacev1alpha1.WorkspaceSpec{
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      testSomeTemplate,
						Namespace: testSharedNamespace,
					},
				},
			}

			err := validator.ValidateCreateWorkspace(ctx, workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(testSharedNamespace))
			Expect(err.Error()).To(ContainSubstring(testNamespaceTeamA))
			Expect(err.Error()).NotTo(ContainSubstring("shared namespace"))
		})

		It("should skip validation when workspace has no templateRef", func() {
			validator := buildValidator(testSharedNamespace)

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: testNamespaceTeamA},
				Spec:       workspacev1alpha1.WorkspaceSpec{},
			}

			err := validator.ValidateCreateWorkspace(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// These exercise the full ValidateCreateWorkspace/ValidateUpdateWorkspace path (template fetch +
	// violation formatting) to confirm the idle shutdown override policy is wired into the webhook,
	// not just the standalone validateIdleShutdownOverrides unit.
	Context("Idle shutdown override enforcement", func() {
		boolPtr := func(b bool) *bool { return &b }
		intPtr := func(i int) *int { return &i }

		templateIdleDefault := func() *workspacev1alpha1.IdleShutdownSpec {
			return &workspacev1alpha1.IdleShutdownSpec{
				Enabled:              true,
				IdleTimeoutInMinutes: 30,
				Detection: workspacev1alpha1.IdleDetectionSpec{
					HTTPGet: &workspacev1alpha1.IdleHTTPGetAction{Transport: "network"},
				},
			}
		}

		// enforcingTemplate locks idle shutdown (allow: false) with a [15, 60] timeout window.
		enforcingTemplate := func() *workspacev1alpha1.WorkspaceTemplate {
			return &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: testSharedTemplate, Namespace: testSharedNamespace},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DefaultImage:        testValidBaseNotebook,
					DisplayName:         "Idle Enforcing Template",
					DefaultIdleShutdown: templateIdleDefault(),
					IdleShutdownOverrides: &workspacev1alpha1.IdleShutdownOverridePolicy{
						Allow:                   boolPtr(false),
						MinIdleTimeoutInMinutes: intPtr(15),
						MaxIdleTimeoutInMinutes: intPtr(60),
					},
				},
			}
		}

		workspaceWithIdle := func(idle *workspacev1alpha1.IdleShutdownSpec) *workspacev1alpha1.Workspace {
			return &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: testNamespaceTeamA},
				Spec: workspacev1alpha1.WorkspaceSpec{
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      testSharedTemplate,
						Namespace: testSharedNamespace,
					},
					IdleShutdown: idle,
				},
			}
		}

		It("should accept a workspace whose timeout is within the enforced bounds", func() {
			validator := buildValidator(testSharedNamespace, enforcingTemplate())

			idle := templateIdleDefault()
			idle.IdleTimeoutInMinutes = 45
			workspace := workspaceWithIdle(idle)

			Expect(validator.ValidateCreateWorkspace(ctx, workspace)).To(Succeed())
		})

		It("should reject a workspace that disables idle shutdown and surface the violation", func() {
			validator := buildValidator(testSharedNamespace, enforcingTemplate())

			idle := templateIdleDefault()
			idle.Enabled = false
			workspace := workspaceWithIdle(idle)

			err := validator.ValidateCreateWorkspace(ctx, workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(testSharedTemplate))
			Expect(err.Error()).To(ContainSubstring("does not allow overriding idle shutdown"))
		})

		It("should reject a workspace whose timeout is outside the enforced bounds", func() {
			validator := buildValidator(testSharedNamespace, enforcingTemplate())

			idle := templateIdleDefault()
			idle.IdleTimeoutInMinutes = 120
			workspace := workspaceWithIdle(idle)

			err := validator.ValidateCreateWorkspace(ctx, workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("outside the range"))
		})

		It("should reject an update that pushes the timeout out of the enforced bounds", func() {
			validator := buildValidator(testSharedNamespace, enforcingTemplate())

			oldIdle := templateIdleDefault()
			oldIdle.IdleTimeoutInMinutes = 45
			oldWorkspace := workspaceWithIdle(oldIdle)

			newIdle := templateIdleDefault()
			newIdle.IdleTimeoutInMinutes = 120
			newWorkspace := workspaceWithIdle(newIdle)

			err := validator.ValidateUpdateWorkspace(ctx, oldWorkspace, newWorkspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("outside the range"))
		})
	})

	// ValidateNamespaceScope is the namespace-only gate the mutating webhook calls before stamping a
	// protection finalizer. Unlike ValidateCreateWorkspace it must NOT fetch the template, so it
	// enforces scope even when the template does not exist and never reports a missing-template error.
	Context("ValidateNamespaceScope", func() {
		It("should reject a disallowed namespace without fetching the template", func() {
			// No template objects are seeded: the gate must reject on scope alone, never on existence.
			validator := buildValidator(testSharedNamespace)

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: testNamespaceTeamA},
				Spec: workspacev1alpha1.WorkspaceSpec{
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      "missing-template",
						Namespace: testNamespaceTeamB,
					},
				},
			}

			err := validator.ValidateNamespaceScope(workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("templateRef.namespace"))
			Expect(err.Error()).To(ContainSubstring(testNamespaceTeamB))
			// The error is about scope, not a missing template.
			Expect(err.Error()).NotTo(ContainSubstring("not found"))
		})

		It("should allow an in-scope reference even when the template does not exist", func() {
			// No template objects are seeded: an in-scope reference passes the gate; existence is the
			// validating webhook's concern, not this gate's.
			validator := buildValidator(testSharedNamespace)

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: testNamespaceTeamA},
				Spec: workspacev1alpha1.WorkspaceSpec{
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      "missing-template",
						Namespace: testNamespaceTeamA,
					},
				},
			}

			Expect(validator.ValidateNamespaceScope(workspace)).To(Succeed())
		})

		It("should allow the shared namespace", func() {
			validator := buildValidator(testSharedNamespace)

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: testNamespaceTeamA},
				Spec: workspacev1alpha1.WorkspaceSpec{
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      testSharedTemplate,
						Namespace: testSharedNamespace,
					},
				},
			}

			Expect(validator.ValidateNamespaceScope(workspace)).To(Succeed())
		})

		It("should allow an empty templateRef namespace", func() {
			validator := buildValidator(testSharedNamespace)

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: testNamespaceTeamA},
				Spec: workspacev1alpha1.WorkspaceSpec{
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name: testSomeTemplate,
					},
				},
			}

			Expect(validator.ValidateNamespaceScope(workspace)).To(Succeed())
		})

		It("should skip validation when workspace has no templateRef", func() {
			validator := buildValidator(testSharedNamespace)

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: testNamespaceTeamA},
				Spec:       workspacev1alpha1.WorkspaceSpec{},
			}

			Expect(validator.ValidateNamespaceScope(workspace)).To(Succeed())
		})
	})
})
