package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	workspacequery "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
)

var _ = Describe("Template Mutability", func() {
	Context("Workspace templateRef Mutability", func() {
		var (
			ctx       context.Context
			template1 *workspacev1alpha1.WorkspaceTemplate
			template2 *workspacev1alpha1.WorkspaceTemplate
			workspace *workspacev1alpha1.Workspace
		)

		BeforeEach(func() {
			ctx = context.Background()

			template1 = &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mutable-template-1",
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Template 1",
					DefaultImage: "quay.io/jupyter/minimal-notebook:latest",
				},
			}
			Expect(k8sClient.Create(ctx, template1)).To(Succeed())

			template2 = &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mutable-template-2",
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Template 2",
					DefaultImage: "quay.io/jupyter/scipy-notebook:latest",
				},
			}
			Expect(k8sClient.Create(ctx, template2)).To(Succeed())

			workspace = &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mutability-test-workspace",
					Namespace: "default",
					Labels: map[string]string{
						"workspace.jupyter.org/template-name": template1.Name,
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Mutability Test",
					TemplateRef: &workspacev1alpha1.TemplateRef{Name: template1.Name},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())
		})

		AfterEach(func() {
			if workspace != nil {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, workspace))).To(Succeed())
			}
			if template1 != nil {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, template1))).To(Succeed())
			}
			if template2 != nil {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, template2))).To(Succeed())
			}
		})

		It("should allow creating workspace with templateRef", func() {
			updatedWorkspace := &workspacev1alpha1.Workspace{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), updatedWorkspace)).To(Succeed())
			Expect(updatedWorkspace.Spec.TemplateRef.Name).To(Equal(template1.Name))
		})

		It("should allow changing templateRef (no longer immutable)", func() {
			updatedWorkspace := &workspacev1alpha1.Workspace{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), updatedWorkspace)).To(Succeed())

			// Change templateRef to template2 (now allowed)
			updatedWorkspace.Spec.TemplateRef = &workspacev1alpha1.TemplateRef{Name: template2.Name}
			err := k8sClient.Update(ctx, updatedWorkspace)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow updating other fields without changing templateRef", func() {
			updatedWorkspace := &workspacev1alpha1.Workspace{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), updatedWorkspace)).To(Succeed())

			// Change displayName but keep same templateRef
			updatedWorkspace.Spec.DisplayName = "Updated Display Name"
			err := k8sClient.Update(ctx, updatedWorkspace)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), updatedWorkspace)).To(Succeed())
			Expect(updatedWorkspace.Spec.DisplayName).To(Equal("Updated Display Name"))
			Expect(updatedWorkspace.Spec.TemplateRef.Name).To(Equal(template1.Name))
		})
	})

	Context("Template Deletion Protection", func() {
		var (
			ctx       context.Context
			template  *workspacev1alpha1.WorkspaceTemplate
			workspace *workspacev1alpha1.Workspace
		)

		BeforeEach(func() {
			ctx = context.Background()

			template = &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "protected-template",
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Protected Template",
					DefaultImage: "quay.io/jupyter/minimal-notebook:latest",
				},
			}
			Expect(k8sClient.Create(ctx, template)).To(Succeed())

			workspace = &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "protection-test-workspace",
					Namespace: "default",
					Labels: map[string]string{
						"workspace.jupyter.org/template-name":      template.Name,
						"workspace.jupyter.org/template-namespace": template.Namespace,
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Protection Test",
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      template.Name,
						Namespace: template.Namespace,
					},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())
		})

		AfterEach(func() {
			if workspace != nil {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, workspace))).To(Succeed())
			}
			if template != nil {
				updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(template), updatedTemplate); err == nil {
					controllerutil.RemoveFinalizer(updatedTemplate, templateFinalizerName)
					Expect(client.IgnoreNotFound(k8sClient.Update(ctx, updatedTemplate))).To(Succeed())
				}
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, template))).To(Succeed())
			}
		})

		It("should list workspaces using a template", func() {
			workspaces, _, err := workspacequery.ListActiveWorkspacesByTemplate(ctx, k8sClient, template.Name, template.Namespace, "", 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(workspaces).To(HaveLen(1))
			Expect(workspaces[0].Name).To(Equal(workspace.Name))
		})

		It("should return empty list when no workspaces use template", func() {
			workspaces, _, err := workspacequery.ListActiveWorkspacesByTemplate(ctx, k8sClient, "nonexistent-template", "default", "", 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(workspaces).To(BeEmpty())
		})

		It("should detect workspaces using template for deletion protection", func() {
			// Add finalizer to simulate what controller would do
			controllerutil.AddFinalizer(template, templateFinalizerName)
			Expect(k8sClient.Update(ctx, template)).To(Succeed())

			// Verify we can detect workspaces using the template
			workspaces, _, err := workspacequery.ListActiveWorkspacesByTemplate(ctx, k8sClient, template.Name, template.Namespace, "", 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(workspaces).To(HaveLen(1))

			// Controller logic would check this before allowing deletion
			Expect(workspaces).ToNot(BeEmpty())
		})

		It("should detect when no workspaces use template for deletion", func() {
			// Create a template that no workspace uses
			unusedTemplate := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "unused-template",
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Unused Template",
					DefaultImage: "quay.io/jupyter/minimal-notebook:latest",
				},
			}
			Expect(k8sClient.Create(ctx, unusedTemplate)).To(Succeed())
			defer func() {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, unusedTemplate))).To(Succeed())
			}()

			// Verify no workspaces use this template
			workspaces, _, err := workspacequery.ListActiveWorkspacesByTemplate(ctx, k8sClient, unusedTemplate.Name, unusedTemplate.Namespace, "", 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(workspaces).To(BeEmpty())

			// Controller would allow deletion since no workspaces reference it
		})
	})

	// NOTE: Webhook Finalizer Management tests are part of E2E tests
	// These tests require webhooks to be running, which is not supported in envtest
	// The webhook adds finalizers during admission (CREATE/UPDATE operations)
	// The controller adds finalizers as a safety net and removes them when no workspaces use the template
	// See test/e2e for webhook behavior tests

	Context("Cross-Namespace Template References", func() {
		It("should handle workspace referencing template with explicit namespace", func() {
			ctx := context.Background()

			// Create template (cluster-scoped, no namespace in metadata)
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cross-ns-template",
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Cross-NS Template",
					DefaultImage: "quay.io/jupyter/minimal-notebook:latest",
				},
			}
			Expect(k8sClient.Create(ctx, template)).To(Succeed())
			defer func() {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, template))).To(Succeed())
			}()

			// Create workspace with explicit namespace in templateRef
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cross-ns-workspace",
					Namespace: "default",
					Labels: map[string]string{
						LabelWorkspaceTemplate:          template.Name,
						LabelWorkspaceTemplateNamespace: "other-namespace",
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Cross-NS Test",
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      template.Name,
						Namespace: "other-namespace",
					},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())
			defer func() {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, workspace))).To(Succeed())
			}()

			// Query with namespace filter - should find workspace
			workspaces, _, err := workspacequery.ListActiveWorkspacesByTemplate(ctx, k8sClient, template.Name, "other-namespace", "", 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(workspaces).To(HaveLen(1))
			Expect(workspaces[0].Name).To(Equal("cross-ns-workspace"))

			// Query with different namespace - should not find workspace
			workspaces, _, err = workspacequery.ListActiveWorkspacesByTemplate(ctx, k8sClient, template.Name, "default", "", 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(workspaces).To(BeEmpty())
		})

		It("should use workspace namespace as default when templateRef.namespace is empty", func() {
			ctx := context.Background()

			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-ns-template",
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Default NS Template",
					DefaultImage: "quay.io/jupyter/minimal-notebook:latest",
				},
			}
			Expect(k8sClient.Create(ctx, template)).To(Succeed())
			defer func() {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, template))).To(Succeed())
			}()

			// Create workspace without namespace in templateRef
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-ns-workspace",
					Namespace: "default",
					Labels: map[string]string{
						LabelWorkspaceTemplate:          template.Name,
						LabelWorkspaceTemplateNamespace: "default", // Should be set to workspace namespace
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Default NS Test",
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name: template.Name,
						// Namespace omitted - should default to workspace namespace
					},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())
			defer func() {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, workspace))).To(Succeed())
			}()

			// Query with workspace namespace - should find it
			workspaces, _, err := workspacequery.ListActiveWorkspacesByTemplate(ctx, k8sClient, template.Name, "default", "", 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(workspaces).To(HaveLen(1))
			Expect(workspaces[0].Labels[LabelWorkspaceTemplateNamespace]).To(Equal("default"))
		})
	})

	Context("Template Spec Mutability", func() {
		It("should allow template spec modification (mutability)", func() {
			ctx := context.Background()
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mutable-template",
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Original Display Name",
					DefaultImage: "quay.io/jupyter/minimal-notebook:latest",
					ResourceBounds: &workspacev1alpha1.ResourceBounds{
						Resources: map[corev1.ResourceName]workspacev1alpha1.ResourceRange{
							corev1.ResourceCPU: {
								Min: resource.MustParse("100m"),
								Max: resource.MustParse("2"),
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, template)).To(Succeed())
			defer func() {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, template))).To(Succeed())
			}()

			// Modify the template spec (should succeed - templates are now mutable)
			updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(template), updatedTemplate)).To(Succeed())
			updatedTemplate.Spec.DisplayName = "Modified Display Name"
			err := k8sClient.Update(ctx, updatedTemplate)
			Expect(err).NotTo(HaveOccurred(), "template spec should be mutable")

			// Verify the change was persisted
			verifiedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(template), verifiedTemplate)).To(Succeed())
			Expect(verifiedTemplate.Spec.DisplayName).To(Equal("Modified Display Name"))
		})

		It("should allow metadata updates independently of spec", func() {
			ctx := context.Background()
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "metadata-mutable-template",
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Metadata Test",
					DefaultImage: "quay.io/jupyter/minimal-notebook:latest",
				},
			}
			Expect(k8sClient.Create(ctx, template)).To(Succeed())
			defer func() {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, template))).To(Succeed())
			}()

			// Metadata changes (labels, annotations) should be allowed
			updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(template), updatedTemplate)).To(Succeed())
			if updatedTemplate.Labels == nil {
				updatedTemplate.Labels = make(map[string]string)
			}
			updatedTemplate.Labels["new-label"] = "test"
			Expect(k8sClient.Update(ctx, updatedTemplate)).To(Succeed())

			// Verify label was added
			verifyTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(template), verifyTemplate)).To(Succeed())
			Expect(verifyTemplate.Labels["new-label"]).To(Equal("test"))
		})
	})
})
