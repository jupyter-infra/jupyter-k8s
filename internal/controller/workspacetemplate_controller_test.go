/*
MIT License

Copyright (c) 2025 jupyter-ai-contrib

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	workspaceutil "github.com/jupyter-ai-contrib/jupyter-k8s/internal/workspace"
)

var _ = Describe("WorkspaceTemplate Controller", func() {
	Context("Finalizer Management", func() {
		var (
			ctx       context.Context
			template  *workspacev1alpha1.WorkspaceTemplate
			workspace *workspacev1alpha1.Workspace
		)

		BeforeEach(func() {
			ctx = context.Background()

			template = &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("test-template-%d", time.Now().UnixNano()),
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Test Template",
					DefaultImage: "quay.io/jupyter/minimal-notebook:latest",
				},
			}
			Expect(k8sClient.Create(ctx, template)).To(Succeed())
		})

		AfterEach(func() {
			if workspace != nil {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, workspace))).To(Succeed())
			}
			if template != nil {
				// Remove finalizer to allow cleanup
				updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(template), updatedTemplate); err == nil {
					controllerutil.RemoveFinalizer(updatedTemplate, templateFinalizerName)
					Expect(client.IgnoreNotFound(k8sClient.Update(ctx, updatedTemplate))).To(Succeed())
				}
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, template))).To(Succeed())
			}
		})

		It("should add finalizer when workspace starts using template", func() {
			// Create workspace using the template
			workspace = &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-workspace-%d", time.Now().UnixNano()),
					Namespace: "default",
					Labels: map[string]string{
						workspaceutil.LabelWorkspaceTemplate: template.Name,
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					TemplateRef: &workspacev1alpha1.TemplateRef{Name: template.Name},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			// Manually trigger reconciliation (no controller manager running in unit tests)
			reconciler := &WorkspaceTemplateReconciler{
				Client: k8sClient,
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: template.Name},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer was added
			updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(template), updatedTemplate)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(updatedTemplate, templateFinalizerName)).To(BeTrue(),
				"template should have finalizer when workspace is using it")
		})

		It("should remove finalizer when no workspaces use template", func() {
			// Create workspace
			workspace = &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-workspace-%d", time.Now().UnixNano()),
					Namespace: "default",
					Labels: map[string]string{
						workspaceutil.LabelWorkspaceTemplate: template.Name,
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					TemplateRef: &workspacev1alpha1.TemplateRef{Name: template.Name},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			// Trigger reconciliation to add finalizer
			reconciler := &WorkspaceTemplateReconciler{
				Client: k8sClient,
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: template.Name},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer was added
			updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(template), updatedTemplate)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(updatedTemplate, templateFinalizerName)).To(BeTrue())

			// Delete workspace
			Expect(k8sClient.Delete(ctx, workspace)).To(Succeed())

			// Trigger reconciliation again to remove finalizer
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: template.Name},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer was removed
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(template), updatedTemplate)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(updatedTemplate, templateFinalizerName)).To(BeFalse(),
				"template finalizer should be removed when no workspaces use it")
		})

		It("should block template deletion when workspace is using it", func() {
			// Create workspace
			workspace = &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-workspace-%d", time.Now().UnixNano()),
					Namespace: "default",
					Labels: map[string]string{
						workspaceutil.LabelWorkspaceTemplate: template.Name,
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					TemplateRef: &workspacev1alpha1.TemplateRef{Name: template.Name},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			// Trigger reconciliation to add finalizer
			reconciler := &WorkspaceTemplateReconciler{
				Client: k8sClient,
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: template.Name},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer was added
			updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(template), updatedTemplate)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(updatedTemplate, templateFinalizerName)).To(BeTrue())

			// Try to delete template
			Expect(k8sClient.Delete(ctx, template)).To(Succeed())

			// Trigger reconciliation during deletion - should NOT remove finalizer
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: template.Name},
			})
			Expect(err).NotTo(HaveOccurred())

			// Template should have deletionTimestamp but still exist (blocked by finalizer)
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(template), updatedTemplate)).To(Succeed(),
				"template should still exist (blocked by finalizer)")
			Expect(updatedTemplate.DeletionTimestamp.IsZero()).To(BeFalse(),
				"template should have deletionTimestamp")
			Expect(controllerutil.ContainsFinalizer(updatedTemplate, templateFinalizerName)).To(BeTrue(),
				"finalizer should still be present while workspace uses template")

			// Delete workspace
			Expect(k8sClient.Delete(ctx, workspace)).To(Succeed())

			// Trigger final reconciliation to remove finalizer and allow deletion
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: template.Name},
			})
			Expect(err).NotTo(HaveOccurred())

			// Template should be deleted now
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(template), updatedTemplate)
			Expect(err).To(HaveOccurred(), "template should be deleted after workspace is removed")
		})
	})

	Context("findTemplatesForWorkspace", func() {
		var (
			ctx        context.Context
			reconciler *WorkspaceTemplateReconciler
		)

		BeforeEach(func() {
			ctx = context.Background()
			reconciler = &WorkspaceTemplateReconciler{
				Client: k8sClient,
			}
		})

		It("should return template name from workspace label", func() {
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace",
					Namespace: "default",
					Labels: map[string]string{
						workspaceutil.LabelWorkspaceTemplate: "my-template",
					},
				},
			}

			requests := reconciler.findTemplatesForWorkspace(ctx, workspace)
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal("my-template"))
		})

		It("should return template name during workspace deletion", func() {
			now := metav1.Now()
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-workspace",
					Namespace:         "default",
					DeletionTimestamp: &now,
					Labels: map[string]string{
						workspaceutil.LabelWorkspaceTemplate: "my-template",
					},
				},
			}

			requests := reconciler.findTemplatesForWorkspace(ctx, workspace)
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal("my-template"),
				"should return template name even during deletion (labels persist)")
		})

		It("should return empty when workspace has no template label", func() {
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace",
					Namespace: "default",
				},
			}

			requests := reconciler.findTemplatesForWorkspace(ctx, workspace)
			Expect(requests).To(BeEmpty())
		})

		It("should return empty for non-workspace objects", func() {
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-template",
				},
			}

			requests := reconciler.findTemplatesForWorkspace(ctx, template)
			Expect(requests).To(BeEmpty())
		})
	})

	Context("handleDeletion", func() {
		var (
			ctx        context.Context
			reconciler *WorkspaceTemplateReconciler
			template   *workspacev1alpha1.WorkspaceTemplate
			recorder   *record.FakeRecorder
		)

		BeforeEach(func() {
			ctx = context.Background()
			recorder = record.NewFakeRecorder(10)
			reconciler = &WorkspaceTemplateReconciler{
				Client:   k8sClient,
				recorder: recorder,
			}

			template = &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("deletion-test-template-%d", time.Now().UnixNano()),
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Deletion Test Template",
					DefaultImage: "quay.io/jupyter/minimal-notebook:latest",
				},
			}
		})

		AfterEach(func() {
			if template != nil {
				updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(template), updatedTemplate); err == nil {
					controllerutil.RemoveFinalizer(updatedTemplate, templateFinalizerName)
					Expect(client.IgnoreNotFound(k8sClient.Update(ctx, updatedTemplate))).To(Succeed())
				}
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, template))).To(Succeed())
			}
		})

		It("should allow deletion when no template finalizer is present", func() {
			Expect(k8sClient.Create(ctx, template)).To(Succeed())

			// Add a different finalizer so the resource doesn't get deleted immediately
			controllerutil.AddFinalizer(template, "example.com/other-finalizer")
			Expect(k8sClient.Update(ctx, template)).To(Succeed())

			// Delete the template (marks it for deletion)
			Expect(k8sClient.Delete(ctx, template)).To(Succeed())

			// Get the updated template with deletionTimestamp
			updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(template), updatedTemplate)).To(Succeed())

			// handleDeletion should return without error since template finalizer is not present
			// It won't remove any finalizers, just checks and returns
			result, err := reconciler.handleDeletion(ctx, updatedTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Template should still exist (other finalizer is still present)
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(template), updatedTemplate)).To(Succeed())
			Expect(updatedTemplate.DeletionTimestamp.IsZero()).To(BeFalse())
			Expect(controllerutil.ContainsFinalizer(updatedTemplate, templateFinalizerName)).To(BeFalse())
		})

		It("should remove finalizer when no workspaces use template", func() {
			Expect(k8sClient.Create(ctx, template)).To(Succeed())

			// Add finalizer
			controllerutil.AddFinalizer(template, templateFinalizerName)
			Expect(k8sClient.Update(ctx, template)).To(Succeed())

			// Delete the template (marks it for deletion)
			Expect(k8sClient.Delete(ctx, template)).To(Succeed())

			// Get the updated template with deletionTimestamp
			updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(template), updatedTemplate)).To(Succeed())

			result, err := reconciler.handleDeletion(ctx, updatedTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify finalizer was removed (template should be deleted)
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(template), updatedTemplate)
			Expect(err).To(HaveOccurred(), "template should be deleted after finalizer is removed")
		})

		It("should keep finalizer when workspaces use template", func() {
			Expect(k8sClient.Create(ctx, template)).To(Succeed())

			// Create workspace using template
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-workspace-%d", time.Now().UnixNano()),
					Namespace: "default",
					Labels: map[string]string{
						workspaceutil.LabelWorkspaceTemplate: template.Name,
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					TemplateRef: &workspacev1alpha1.TemplateRef{Name: template.Name},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())
			defer func() {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, workspace))).To(Succeed())
			}()

			// Add finalizer
			controllerutil.AddFinalizer(template, templateFinalizerName)
			Expect(k8sClient.Update(ctx, template)).To(Succeed())

			// Delete the template (marks it for deletion)
			Expect(k8sClient.Delete(ctx, template)).To(Succeed())

			// Get the updated template with deletionTimestamp
			updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: template.Name}, updatedTemplate)).To(Succeed())

			result, err := reconciler.handleDeletion(ctx, updatedTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify finalizer was NOT removed (template should still exist)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: template.Name}, updatedTemplate)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(updatedTemplate, templateFinalizerName)).To(BeTrue(),
				"finalizer should remain when workspaces are using the template")
		})
	})
})
