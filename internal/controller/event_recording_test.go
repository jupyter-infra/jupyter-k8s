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
	"k8s.io/client-go/tools/record"

	workspacesv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("Event Recording", func() {
	var (
		fakeRecorder  *record.FakeRecorder
		statusManager *StatusManager
		stateMachine  *StateMachine
		ctx           context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		// Create a fake recorder with buffer size for events
		fakeRecorder = record.NewFakeRecorder(10)
		statusManager = NewStatusManager(k8sClient)
	})

	AfterEach(func() {
		// Drain any remaining events
		for len(fakeRecorder.Events) > 0 {
			<-fakeRecorder.Events
		}
	})

	Context("Template Validation Events", func() {
		It("should record TemplateValidationFailed event when validation fails", func() {
			By("creating a workspace with invalid template overrides")
			workspace := &workspacesv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-validation-failed",
					Namespace: "default",
				},
				Spec: workspacesv1alpha1.WorkspaceSpec{
					DisplayName: "Test Validation Failed",
					TemplateRef: stringPtr("test-template"),
					Image:       "disallowed-image:latest",
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			By("creating a template with image restrictions")
			template := &workspacesv1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-template",
				},
				Spec: workspacesv1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Test Template",
					DefaultImage: "allowed-image:latest",
					AllowedImages: []string{
						"allowed-image:latest",
					},
				},
			}
			Expect(k8sClient.Create(ctx, template)).To(Succeed())

			By("creating state machine with fake recorder")
			templateResolver := NewTemplateResolver(k8sClient)
			options := WorkspaceControllerOptions{
				ApplicationImagesPullPolicy: corev1.PullIfNotPresent,
			}
			deploymentBuilder := NewDeploymentBuilder(k8sClient.Scheme(), options, k8sClient)
			serviceBuilder := NewServiceBuilder(k8sClient.Scheme())
			pvcBuilder := NewPVCBuilder(k8sClient.Scheme())
			resourceManager := NewResourceManager(k8sClient, deploymentBuilder, serviceBuilder, pvcBuilder, statusManager)
			stateMachine = NewStateMachine(resourceManager, statusManager, templateResolver, fakeRecorder)

			By("reconciling the workspace")
			_, err := stateMachine.ReconcileDesiredState(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())

			By("verifying TemplateValidationFailed event was recorded")
			var event string
			Eventually(func() bool {
				select {
				case event = <-fakeRecorder.Events:
					return true
				default:
					return false
				}
			}, "5s").Should(BeTrue(), "Expected to receive an event")
			Expect(event).To(ContainSubstring("Warning"))
			Expect(event).To(ContainSubstring("TemplateValidationFailed"))

			By("cleaning up")
			Expect(k8sClient.Delete(ctx, workspace)).To(Succeed())
			Expect(k8sClient.Delete(ctx, template)).To(Succeed())
		})

		It("should record TemplateValidated event when validation passes", func() {
			By("creating a workspace with valid template overrides")
			workspace := &workspacesv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-validation-passed",
					Namespace: "default",
				},
				Spec: workspacesv1alpha1.WorkspaceSpec{
					DisplayName: "Test Validation Passed",
					TemplateRef: stringPtr("test-template-valid"),
					Image:       "allowed-image:latest",
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			By("creating a template that allows the image")
			template := &workspacesv1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-template-valid",
				},
				Spec: workspacesv1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Test Template Valid",
					DefaultImage: "allowed-image:latest",
					AllowedImages: []string{
						"allowed-image:latest",
					},
				},
			}
			Expect(k8sClient.Create(ctx, template)).To(Succeed())

			By("creating state machine with fake recorder")
			templateResolver := NewTemplateResolver(k8sClient)
			options := WorkspaceControllerOptions{
				ApplicationImagesPullPolicy: corev1.PullIfNotPresent,
			}
			deploymentBuilder := NewDeploymentBuilder(k8sClient.Scheme(), options, k8sClient)
			serviceBuilder := NewServiceBuilder(k8sClient.Scheme())
			pvcBuilder := NewPVCBuilder(k8sClient.Scheme())
			resourceManager := NewResourceManager(k8sClient, deploymentBuilder, serviceBuilder, pvcBuilder, statusManager)
			stateMachine = NewStateMachine(resourceManager, statusManager, templateResolver, fakeRecorder)

			By("reconciling the workspace")
			_, err := stateMachine.ReconcileDesiredState(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())

			By("verifying TemplateValidated event was recorded")
			var event string
			Eventually(func() bool {
				select {
				case event = <-fakeRecorder.Events:
					return true
				default:
					return false
				}
			}, "5s").Should(BeTrue(), "Expected to receive an event")
			Expect(event).To(ContainSubstring("Normal"))
			Expect(event).To(ContainSubstring("TemplateValidated"))

			By("cleaning up")
			Expect(k8sClient.Delete(ctx, workspace)).To(Succeed())
			Expect(k8sClient.Delete(ctx, template)).To(Succeed())
		})
	})

	Context("Resource Bounds Validation Events", func() {
		It("should record event when CPU exceeds bounds", func() {
			By("creating a workspace with CPU exceeding template max")
			workspace := &workspacesv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cpu-exceeded",
					Namespace: "default",
				},
				Spec: workspacesv1alpha1.WorkspaceSpec{
					DisplayName: "Test CPU Exceeded",
					TemplateRef: stringPtr("bounded-template"),
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("10"), // Exceeds max
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			By("creating a template with CPU bounds")
			template := &workspacesv1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bounded-template",
				},
				Spec: workspacesv1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Bounded Template",
					DefaultImage: "test-image:latest",
					ResourceBounds: &workspacesv1alpha1.ResourceBounds{
						CPU: &workspacesv1alpha1.ResourceRange{
							Min: resource.MustParse("250m"),
							Max: resource.MustParse("2"),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, template)).To(Succeed())

			By("creating state machine with fake recorder")
			templateResolver := NewTemplateResolver(k8sClient)
			options := WorkspaceControllerOptions{
				ApplicationImagesPullPolicy: corev1.PullIfNotPresent,
			}
			deploymentBuilder := NewDeploymentBuilder(k8sClient.Scheme(), options, k8sClient)
			serviceBuilder := NewServiceBuilder(k8sClient.Scheme())
			pvcBuilder := NewPVCBuilder(k8sClient.Scheme())
			resourceManager := NewResourceManager(k8sClient, deploymentBuilder, serviceBuilder, pvcBuilder, statusManager)
			stateMachine = NewStateMachine(resourceManager, statusManager, templateResolver, fakeRecorder)

			By("reconciling the workspace")
			_, err := stateMachine.ReconcileDesiredState(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())

			By("verifying validation failed event was recorded")
			Eventually(fakeRecorder.Events).Should(Receive(ContainSubstring("TemplateValidationFailed")))

			By("cleaning up")
			Expect(k8sClient.Delete(ctx, workspace)).To(Succeed())
			Expect(k8sClient.Delete(ctx, template)).To(Succeed())
		})
	})
})

// stringPtr is a helper to get pointer to string
func stringPtr(s string) *string {
	return &s
}
