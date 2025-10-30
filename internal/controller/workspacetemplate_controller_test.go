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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("WorkspaceTemplate Controller Compliance Checking", func() {
	var (
		ctx              context.Context
		template         *workspacev1alpha1.WorkspaceTemplate
		templateResolver *TemplateResolver
		reconciler       *WorkspaceTemplateReconciler
	)

	BeforeEach(func() {
		ctx = context.Background()
		templateResolver = NewTemplateResolver(k8sClient)
		reconciler = &WorkspaceTemplateReconciler{
			Client:           k8sClient,
			Scheme:           k8sClient.Scheme(),
			templateResolver: templateResolver,
		}

		// Create a base template for testing
		template = &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name: "compliance-test-template",
			},
			Spec: workspacev1alpha1.WorkspaceTemplateSpec{
				DisplayName:  "Compliance Test Template",
				DefaultImage: "jk8s-application-jupyter-uv:latest",
				AllowedImages: []string{
					"jk8s-application-jupyter-uv:latest",
				},
				ResourceBounds: &workspacev1alpha1.ResourceBounds{
					CPU: &workspacev1alpha1.ResourceRange{
						Min: resource.MustParse("100m"),
						Max: resource.MustParse("2"),
					},
					Memory: &workspacev1alpha1.ResourceRange{
						Min: resource.MustParse("128Mi"),
						Max: resource.MustParse("2Gi"),
					},
				},
				PrimaryStorage: &workspacev1alpha1.StorageConfig{
					DefaultSize: resource.MustParse("1Gi"),
					MinSize:     &[]resource.Quantity{resource.MustParse("100Mi")}[0],
					MaxSize:     &[]resource.Quantity{resource.MustParse("10Gi")}[0],
				},
			},
		}
		Expect(k8sClient.Create(ctx, template)).To(Succeed())
	})

	AfterEach(func() {
		// Clean up workspaces first
		workspaceList := &workspacev1alpha1.WorkspaceList{}
		Expect(k8sClient.List(ctx, workspaceList, client.MatchingLabels{
			"workspace.jupyter.org/template": template.Name,
		})).To(Succeed())
		for _, ws := range workspaceList.Items {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &ws))).To(Succeed())
		}

		// Clean up template
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, template))).To(Succeed())
	})

	Describe("checkWorkspaceCompliance", func() {
		It("should list workspaces using template", func() {
			// Create workspace using the template
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "compliance-workspace-1",
					Namespace: "default",
					Labels: map[string]string{
						"workspace.jupyter.org/template": template.Name,
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Compliance Workspace 1",
					TemplateRef: &workspacev1alpha1.TemplateRef{Name: template.Name},
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("512Mi"),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			// Get fresh template
			freshTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(template), freshTemplate)).To(Succeed())

			// Check compliance
			err := reconciler.checkWorkspaceCompliance(ctx, freshTemplate)
			Expect(err).NotTo(HaveOccurred())

			// Verify workspace status was updated (check for TemplateCompliant condition)
			updatedWorkspace := &workspacev1alpha1.Workspace{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), updatedWorkspace)).To(Succeed())

			// Should have TemplateCompliant condition
			var complianceCondition *metav1.Condition
			for i := range updatedWorkspace.Status.Conditions {
				if updatedWorkspace.Status.Conditions[i].Type == workspacev1alpha1.ConditionTemplateCompliant {
					complianceCondition = &updatedWorkspace.Status.Conditions[i]
					break
				}
			}
			Expect(complianceCondition).NotTo(BeNil(), "TemplateCompliant condition should be set")
		})

		It("should handle empty workspace list gracefully", func() {
			// No workspaces created
			freshTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(template), freshTemplate)).To(Succeed())

			err := reconciler.checkWorkspaceCompliance(ctx, freshTemplate)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("updateComplianceStatus", func() {
		var workspace *workspacev1alpha1.Workspace

		BeforeEach(func() {
			workspace = &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "status-test-workspace",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Status Test Workspace",
					TemplateRef: &workspacev1alpha1.TemplateRef{Name: template.Name},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())
		})

		AfterEach(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, workspace))).To(Succeed())
		})

		It("should set TemplateCompliant=True for compliant workspace", func() {
			err := reconciler.updateComplianceStatus(ctx, workspace, true, []TemplateViolation{})
			Expect(err).NotTo(HaveOccurred())

			// Verify status was updated
			updatedWorkspace := &workspacev1alpha1.Workspace{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), updatedWorkspace)).To(Succeed())

			// Find TemplateCompliant condition
			var found bool
			for _, condition := range updatedWorkspace.Status.Conditions {
				if condition.Type == workspacev1alpha1.ConditionTemplateCompliant {
					found = true
					Expect(condition.Status).To(Equal(metav1.ConditionTrue))
					Expect(condition.Reason).To(Equal(workspacev1alpha1.ReasonTemplateCompliant))
					Expect(condition.Message).To(ContainSubstring("complies"))
				}
			}
			Expect(found).To(BeTrue(), "TemplateCompliant condition should exist")
		})

		It("should set TemplateCompliant=False for non-compliant workspace", func() {
			violations := []TemplateViolation{
				{
					Type:    ViolationTypeResourceExceeded,
					Field:   "spec.resources.requests.cpu",
					Message: "CPU request exceeds template maximum",
					Allowed: "max: 2",
					Actual:  "4",
				},
			}

			err := reconciler.updateComplianceStatus(ctx, workspace, false, violations)
			Expect(err).NotTo(HaveOccurred())

			// Verify status was updated
			updatedWorkspace := &workspacev1alpha1.Workspace{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), updatedWorkspace)).To(Succeed())

			// Find TemplateCompliant condition
			var found bool
			for _, condition := range updatedWorkspace.Status.Conditions {
				if condition.Type == workspacev1alpha1.ConditionTemplateCompliant {
					found = true
					Expect(condition.Status).To(Equal(metav1.ConditionFalse))
					Expect(condition.Reason).To(Equal(workspacev1alpha1.ReasonTemplateNonCompliant))
					Expect(condition.Message).To(ContainSubstring("violates"))
					Expect(condition.Message).To(ContainSubstring("CPU request exceeds"))
				}
			}
			Expect(found).To(BeTrue(), "TemplateCompliant condition should exist")
		})
	})

	Describe("formatViolations", func() {
		It("should format single violation correctly", func() {
			violations := []TemplateViolation{
				{
					Type:    ViolationTypeResourceExceeded,
					Field:   "spec.resources.requests.cpu",
					Message: "CPU exceeds max",
					Allowed: "max: 2",
					Actual:  "4",
				},
			}

			result := formatViolations(violations)
			Expect(result).To(ContainSubstring("spec.resources.requests.cpu"))
			Expect(result).To(ContainSubstring("CPU exceeds max"))
			Expect(result).To(ContainSubstring("max: 2"))
			Expect(result).To(ContainSubstring("4"))
		})

		It("should format multiple violations with semicolon separator", func() {
			violations := []TemplateViolation{
				{
					Field:   "spec.resources.requests.cpu",
					Message: "CPU exceeds max",
					Allowed: "max: 2",
					Actual:  "4",
				},
				{
					Field:   "spec.resources.requests.memory",
					Message: "Memory exceeds max",
					Allowed: "max: 2Gi",
					Actual:  "4Gi",
				},
			}

			result := formatViolations(violations)
			Expect(result).To(ContainSubstring(";"))
			Expect(result).To(ContainSubstring("cpu"))
			Expect(result).To(ContainSubstring("memory"))
		})

		It("should handle empty violations list", func() {
			result := formatViolations([]TemplateViolation{})
			Expect(result).To(Equal("no violations"))
		})
	})

	Describe("setCondition", func() {
		It("should update existing condition", func() {
			conditions := []metav1.Condition{
				{
					Type:   workspacev1alpha1.ConditionTemplateCompliant,
					Status: metav1.ConditionTrue,
					Reason: "OldReason",
				},
			}

			newCondition := metav1.Condition{
				Type:   workspacev1alpha1.ConditionTemplateCompliant,
				Status: metav1.ConditionFalse,
				Reason: "NewReason",
			}

			setCondition(&conditions, newCondition)

			Expect(conditions).To(HaveLen(1))
			Expect(conditions[0].Status).To(Equal(metav1.ConditionFalse))
			Expect(conditions[0].Reason).To(Equal("NewReason"))
		})

		It("should append new condition", func() {
			conditions := []metav1.Condition{
				{
					Type:   "OtherCondition",
					Status: metav1.ConditionTrue,
				},
			}

			newCondition := metav1.Condition{
				Type:   workspacev1alpha1.ConditionTemplateCompliant,
				Status: metav1.ConditionTrue,
				Reason: workspacev1alpha1.ReasonTemplateCompliant,
			}

			setCondition(&conditions, newCondition)

			Expect(conditions).To(HaveLen(2))
			Expect(conditions[1].Type).To(Equal(workspacev1alpha1.ConditionTemplateCompliant))
		})

		It("should handle nil conditions slice gracefully", func() {
			var conditions *[]metav1.Condition
			newCondition := metav1.Condition{
				Type:   workspacev1alpha1.ConditionTemplateCompliant,
				Status: metav1.ConditionTrue,
			}

			// Should not panic
			setCondition(conditions, newCondition)
		})
	})
})
