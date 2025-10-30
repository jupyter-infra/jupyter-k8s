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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("Compliance Status Conditions", func() {
	var (
		ctx        context.Context
		template   *workspacev1alpha1.WorkspaceTemplate
		workspace  *workspacev1alpha1.Workspace
		reconciler *WorkspaceTemplateReconciler
	)

	BeforeEach(func() {
		ctx = context.Background()
		templateResolver := NewTemplateResolver(k8sClient)
		reconciler = &WorkspaceTemplateReconciler{
			Client:           k8sClient,
			Scheme:           k8sClient.Scheme(),
			templateResolver: templateResolver,
		}
	})

	AfterEach(func() {
		if workspace != nil {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, workspace))).To(Succeed())
		}
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, template))).To(Succeed())
	})

	It("should set TemplateCompliant=True for initially compliant workspace", func() {
		template = &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name: "compliant-status-template",
			},
			Spec: workspacev1alpha1.WorkspaceTemplateSpec{
				DisplayName:  "Compliance Status Template",
				DefaultImage: "test-image:v1",
				AllowedImages: []string{
					"test-image:v1",
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
			},
		}
		Expect(k8sClient.Create(ctx, template)).To(Succeed())

		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "compliant-workspace",
				Namespace: "default",
				Labels: map[string]string{
					"workspace.jupyter.org/template": template.Name,
				},
			},
			Spec: workspacev1alpha1.WorkspaceSpec{
				DisplayName: "Compliant Workspace",
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

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: client.ObjectKeyFromObject(template),
		})
		Expect(err).NotTo(HaveOccurred())

		updatedWorkspace := &workspacev1alpha1.Workspace{}
		Eventually(func() bool {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), updatedWorkspace)
			if err != nil {
				return false
			}
			for _, condition := range updatedWorkspace.Status.Conditions {
				if condition.Type == workspacev1alpha1.ConditionTemplateCompliant {
					return condition.Status == metav1.ConditionTrue
				}
			}
			return false
		}, 5*time.Second, 500*time.Millisecond).Should(BeTrue(), "TemplateCompliant condition should be True")

		var complianceCondition *metav1.Condition
		for i := range updatedWorkspace.Status.Conditions {
			if updatedWorkspace.Status.Conditions[i].Type == workspacev1alpha1.ConditionTemplateCompliant {
				complianceCondition = &updatedWorkspace.Status.Conditions[i]
				break
			}
		}
		Expect(complianceCondition).NotTo(BeNil())
		Expect(complianceCondition.Reason).To(Equal(workspacev1alpha1.ReasonTemplateCompliant))
		Expect(complianceCondition.Message).To(ContainSubstring("complies"))
	})

	It("should update TemplateCompliant=False when template becomes stricter", func() {
		template = &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name: "noncompliant-status-template",
			},
			Spec: workspacev1alpha1.WorkspaceTemplateSpec{
				DisplayName:  "Non-Compliant Status Template",
				DefaultImage: "test-image:v1",
				AllowedImages: []string{
					"test-image:v1",
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
			},
		}
		Expect(k8sClient.Create(ctx, template)).To(Succeed())

		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "becomes-noncompliant-workspace",
				Namespace: "default",
				Labels: map[string]string{
					"workspace.jupyter.org/template": template.Name,
				},
			},
			Spec: workspacev1alpha1.WorkspaceSpec{
				DisplayName: "Becomes Non-Compliant",
				TemplateRef: &workspacev1alpha1.TemplateRef{Name: template.Name},
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1500m"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: client.ObjectKeyFromObject(template),
		})
		Expect(err).NotTo(HaveOccurred())

		updatedWorkspace := &workspacev1alpha1.Workspace{}
		Eventually(func() bool {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), updatedWorkspace)
			if err != nil {
				return false
			}
			for _, condition := range updatedWorkspace.Status.Conditions {
				if condition.Type == workspacev1alpha1.ConditionTemplateCompliant {
					return condition.Status == metav1.ConditionTrue
				}
			}
			return false
		}, 5*time.Second, 500*time.Millisecond).Should(BeTrue(), "should be compliant initially")

		updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(template), updatedTemplate)).To(Succeed())
		updatedTemplate.Spec.ResourceBounds.CPU.Max = resource.MustParse("1")
		Expect(k8sClient.Update(ctx, updatedTemplate)).To(Succeed())

		_, err = reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: client.ObjectKeyFromObject(template),
		})
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() bool {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), updatedWorkspace)
			if err != nil {
				return false
			}
			for _, condition := range updatedWorkspace.Status.Conditions {
				if condition.Type == workspacev1alpha1.ConditionTemplateCompliant {
					return condition.Status == metav1.ConditionFalse
				}
			}
			return false
		}, 5*time.Second, 500*time.Millisecond).Should(BeTrue(), "TemplateCompliant should become False")

		var complianceCondition *metav1.Condition
		for i := range updatedWorkspace.Status.Conditions {
			if updatedWorkspace.Status.Conditions[i].Type == workspacev1alpha1.ConditionTemplateCompliant {
				complianceCondition = &updatedWorkspace.Status.Conditions[i]
				break
			}
		}
		Expect(complianceCondition).NotTo(BeNil())
		Expect(complianceCondition.Reason).To(Equal(workspacev1alpha1.ReasonTemplateNonCompliant))
		Expect(complianceCondition.Message).To(ContainSubstring("violates"))
		Expect(complianceCondition.Message).To(ContainSubstring("CPU"))
	})
})
