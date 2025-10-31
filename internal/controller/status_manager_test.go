package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("StatusManager", func() {
	var (
		ctx       context.Context
		workspace *workspacev1alpha1.Workspace
	)

	BeforeEach(func() {
		ctx = context.Background()

		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workspace",
				Namespace: "default",
			},
			Spec: workspacev1alpha1.WorkspaceSpec{
				Image: "jupyter/base-notebook:latest",
			},
		}
	})

	Context("Update Status Methods", func() {
		BeforeEach(func() {
			// Create the workspace in the test cluster first
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())
		})

		AfterEach(func() {
			// Clean up the workspace
			Expect(k8sClient.Delete(ctx, workspace)).To(Succeed())
		})

	})
})
