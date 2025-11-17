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
	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("Lazy Application", func() {
	var (
		ctx       context.Context
		template  *workspacev1alpha1.WorkspaceTemplate
		workspace *workspacev1alpha1.Workspace
	)

	BeforeEach(func() {
		ctx = context.Background()

		template = &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "lazy-app-template",
				Namespace: "default",
			},
			Spec: workspacev1alpha1.WorkspaceTemplateSpec{
				DisplayName:  "Lazy Application Template",
				DefaultImage: "image:v1",
				AllowedImages: []string{
					"image:v1",
					"image:v2",
				},
				DefaultResources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
				ResourceBounds: &workspacev1alpha1.ResourceBounds{
					Resources: map[corev1.ResourceName]workspacev1alpha1.ResourceRange{
						corev1.ResourceCPU: {
							Min: resource.MustParse("50m"),
							Max: resource.MustParse("2"),
						},
						corev1.ResourceMemory: {
							Min: resource.MustParse("64Mi"),
							Max: resource.MustParse("2Gi"),
						},
					},
				},
				PrimaryStorage: &workspacev1alpha1.StorageConfig{
					DefaultSize: resource.MustParse("1Gi"),
				},
			},
		}
		Expect(k8sClient.Create(ctx, template)).To(Succeed())

		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "lazy-app-workspace",
				Namespace: "default",
				Labels: map[string]string{
					"workspace.jupyter.org/template": template.Name,
				},
			},
			Spec: workspacev1alpha1.WorkspaceSpec{
				DisplayName: "Lazy Application Workspace",
				TemplateRef: &workspacev1alpha1.TemplateRef{Name: template.Name},
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, workspace)).To(Succeed())
	})

	AfterEach(func() {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, workspace))).To(Succeed())
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, template))).To(Succeed())
	})

	It("should keep existing workspace spec unchanged when template becomes stricter", func() {
		updatedWorkspace := &workspacev1alpha1.Workspace{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), updatedWorkspace)).To(Succeed())
		originalCPU := updatedWorkspace.Spec.Resources.Requests.Cpu().String()
		Expect(originalCPU).To(Equal("1"))

		updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(template), updatedTemplate)).To(Succeed())
		updatedTemplate.Spec.ResourceBounds.Resources[corev1.ResourceCPU] = workspacev1alpha1.ResourceRange{
			Min: resource.MustParse("50m"),
			Max: resource.MustParse("500m"),
		}
		Expect(k8sClient.Update(ctx, updatedTemplate)).To(Succeed())

		time.Sleep(100 * time.Millisecond)

		verifiedWorkspace := &workspacev1alpha1.Workspace{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), verifiedWorkspace)).To(Succeed())
		Expect(verifiedWorkspace.Spec.Resources.Requests.Cpu().String()).To(Equal("1"), "workspace spec should not change")
	})

	It("should apply new template defaults to new workspaces after template modification", func() {
		updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(template), updatedTemplate)).To(Succeed())
		updatedTemplate.Spec.DefaultImage = "image:v2"
		updatedTemplate.Spec.DefaultResources.Requests[corev1.ResourceCPU] = resource.MustParse("200m")
		Expect(k8sClient.Update(ctx, updatedTemplate)).To(Succeed())

		// Envtest doesn't run webhooks, so manually apply defaults as webhook would
		newWorkspace := &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "new-workspace-after-change",
				Namespace: "default",
			},
			Spec: workspacev1alpha1.WorkspaceSpec{
				DisplayName: "New Workspace",
				TemplateRef: &workspacev1alpha1.TemplateRef{Name: template.Name},
				Image:       updatedTemplate.Spec.DefaultImage,
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    updatedTemplate.Spec.DefaultResources.Requests[corev1.ResourceCPU],
						corev1.ResourceMemory: updatedTemplate.Spec.DefaultResources.Requests[corev1.ResourceMemory],
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, newWorkspace)).To(Succeed())
		defer func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, newWorkspace))).To(Succeed())
		}()

		createdWorkspace := &workspacev1alpha1.Workspace{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(newWorkspace), createdWorkspace)).To(Succeed())
		Expect(createdWorkspace.Spec.Image).To(Equal("image:v2"))
		Expect(createdWorkspace.Spec.Resources.Requests.Cpu().String()).To(Equal("200m"))
	})

	It("should not retroactively apply template changes to existing workspaces", func() {
		// Update template to be more restrictive
		updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(template), updatedTemplate)).To(Succeed())
		updatedTemplate.Spec.DefaultImage = "image:v2"
		updatedTemplate.Spec.DefaultResources.Requests[corev1.ResourceCPU] = resource.MustParse("200m")
		Expect(k8sClient.Update(ctx, updatedTemplate)).To(Succeed())

		time.Sleep(100 * time.Millisecond)

		// Existing workspace should keep its original configuration
		oldWorkspace := &workspacev1alpha1.Workspace{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), oldWorkspace)).To(Succeed())
		Expect(oldWorkspace.Spec.Resources.Requests.Cpu().String()).To(Equal("1"))
	})
})
