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

package v1alpha1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/controller"
)

var _ = Describe("TemplateValidator", func() {
	var (
		ctx       context.Context
		validator *TemplateValidator
		template  *workspacev1alpha1.WorkspaceTemplate
	)

	BeforeEach(func() {
		ctx = context.Background()
		validator = NewTemplateValidator(k8sClient)

		template = &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-template",
			},
			Spec: workspacev1alpha1.WorkspaceTemplateSpec{
				DisplayName:  "Test Template",
				DefaultImage: "jupyter/minimal-notebook:latest",
				DefaultResources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, template)).To(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, template)).To(Succeed())
	})

	Describe("Label Management", func() {
		Context("when workspace has templateRef", func() {
			It("should add template label", func() {
				workspace := &workspacev1alpha1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-workspace",
						Namespace: "default",
					},
					Spec: workspacev1alpha1.WorkspaceSpec{
						DisplayName: "Test Workspace",
						TemplateRef: &workspacev1alpha1.TemplateRef{Name: template.Name},
					},
				}

				err := validator.ApplyTemplateDefaults(ctx, workspace)
				Expect(err).NotTo(HaveOccurred())
				Expect(workspace.Labels).To(HaveKey(controller.LabelWorkspaceTemplate))
				Expect(workspace.Labels[controller.LabelWorkspaceTemplate]).To(Equal(template.Name))
			})

			It("should update existing template label", func() {
				workspace := &workspacev1alpha1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-workspace",
						Namespace: "default",
						Labels: map[string]string{
							controller.LabelWorkspaceTemplate: "old-template",
						},
					},
					Spec: workspacev1alpha1.WorkspaceSpec{
						DisplayName: "Test Workspace",
						TemplateRef: &workspacev1alpha1.TemplateRef{Name: template.Name},
					},
				}

				err := validator.ApplyTemplateDefaults(ctx, workspace)
				Expect(err).NotTo(HaveOccurred())
				Expect(workspace.Labels[controller.LabelWorkspaceTemplate]).To(Equal(template.Name))
			})

			It("should preserve other labels while adding template label", func() {
				workspace := &workspacev1alpha1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-workspace",
						Namespace: "default",
						Labels: map[string]string{
							"app":  "myapp",
							"tier": "backend",
						},
					},
					Spec: workspacev1alpha1.WorkspaceSpec{
						DisplayName: "Test Workspace",
						TemplateRef: &workspacev1alpha1.TemplateRef{Name: template.Name},
					},
				}

				err := validator.ApplyTemplateDefaults(ctx, workspace)
				Expect(err).NotTo(HaveOccurred())
				Expect(workspace.Labels).To(HaveKey("app"))
				Expect(workspace.Labels).To(HaveKey("tier"))
				Expect(workspace.Labels).To(HaveKey(controller.LabelWorkspaceTemplate))
				Expect(workspace.Labels["app"]).To(Equal("myapp"))
				Expect(workspace.Labels["tier"]).To(Equal("backend"))
			})
		})

		Context("when workspace has no templateRef", func() {
			It("should remove template label if it exists", func() {
				workspace := &workspacev1alpha1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-workspace",
						Namespace: "default",
						Labels: map[string]string{
							controller.LabelWorkspaceTemplate: "old-template",
							"app":                             "myapp",
						},
					},
					Spec: workspacev1alpha1.WorkspaceSpec{
						DisplayName: "Test Workspace",
						TemplateRef: nil,
					},
				}

				err := validator.ApplyTemplateDefaults(ctx, workspace)
				Expect(err).NotTo(HaveOccurred())
				Expect(workspace.Labels).NotTo(HaveKey(controller.LabelWorkspaceTemplate))
				Expect(workspace.Labels).To(HaveKey("app"))
			})

			It("should not error if label doesn't exist", func() {
				workspace := &workspacev1alpha1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-workspace",
						Namespace: "default",
					},
					Spec: workspacev1alpha1.WorkspaceSpec{
						DisplayName: "Test Workspace",
						TemplateRef: nil,
					},
				}

				err := validator.ApplyTemplateDefaults(ctx, workspace)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should not error if labels map is nil", func() {
				workspace := &workspacev1alpha1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-workspace",
						Namespace: "default",
						Labels:    nil,
					},
					Spec: workspacev1alpha1.WorkspaceSpec{
						DisplayName: "Test Workspace",
						TemplateRef: nil,
					},
				}

				err := validator.ApplyTemplateDefaults(ctx, workspace)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when templateRef.Name is empty", func() {
			It("should remove template label", func() {
				workspace := &workspacev1alpha1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-workspace",
						Namespace: "default",
						Labels: map[string]string{
							controller.LabelWorkspaceTemplate: "old-template",
						},
					},
					Spec: workspacev1alpha1.WorkspaceSpec{
						DisplayName: "Test Workspace",
						TemplateRef: &workspacev1alpha1.TemplateRef{Name: ""},
					},
				}

				err := validator.ApplyTemplateDefaults(ctx, workspace)
				Expect(err).NotTo(HaveOccurred())
				Expect(workspace.Labels).NotTo(HaveKey(controller.LabelWorkspaceTemplate))
			})
		})
	})
})
