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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	serversv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("JupyterServer Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		jupyterserver := &serversv1alpha1.JupyterServer{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind JupyterServer")
			err := k8sClient.Get(ctx, typeNamespacedName, jupyterserver)
			if err != nil && errors.IsNotFound(err) {
				resource := &serversv1alpha1.JupyterServer{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					// TODO(user): Specify other spec details if needed.
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &serversv1alpha1.JupyterServer{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance JupyterServer")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Creating a mock StateMachine")
			statusManager := StatusManager{
				client: k8sClient,
			}
			options := JupyterServerControllerOptions{
				ApplicationImagesPullPolicy: corev1.PullIfNotPresent,
				ApplicationImagesRegistry:   "",
			}
			deploymentBuilder := DeploymentBuilder{
				scheme:        k8sClient.Scheme(),
				options:       options,
				imageResolver: NewImageResolver("docker.io/library"),
			}
			serviceBuilder := ServiceBuilder{
				scheme: k8sClient.Scheme(),
			}
			resourceManager := ResourceManager{
				client:            k8sClient,
				deploymentBuilder: &deploymentBuilder,
				serviceBuilder:    &serviceBuilder,
				statusManager:     &statusManager,
			}
			stateMachine := StateMachine{
				resourceManager: &resourceManager,
				statusManager:   &statusManager,
			}

			By("Reconciling the created resource")
			controllerReconciler := &JupyterServerReconciler{
				Client:       k8sClient,
				Scheme:       k8sClient.Scheme(),
				stateMachine: &stateMachine,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})
