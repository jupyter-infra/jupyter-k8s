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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("WorkspaceIntegrationCustomDefaulter", func() {
	var (
		defaulter *WorkspaceIntegrationCustomDefaulter
		wi        *workspacev1alpha1.WorkspaceIntegration
		template  *workspacev1alpha1.WorkspaceIntegrationTemplate
		rayGVK    schema.GroupVersionKind
		ctx       context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		rayGVK = schema.GroupVersionKind{Group: "ray.io", Version: "v1", Kind: "RayCluster"}

		wi = &workspacev1alpha1.WorkspaceIntegration{
			ObjectMeta: metav1.ObjectMeta{Name: "ws-foo-ray-integration", Namespace: "user-ns"},
			Spec: workspacev1alpha1.WorkspaceIntegrationSpec{
				TemplateRef: workspacev1alpha1.IntegrationTemplateRef{
					Name: "ray-integration",
					Parameters: []workspacev1alpha1.IntegrationParameter{
						{Name: "clusterName", Value: "my-ray-cluster"},
					},
				},
				WorkspaceRef: workspacev1alpha1.WorkspaceRef{Name: "ws-foo", Namespace: "user-ns"},
			},
		}

		template = &workspacev1alpha1.WorkspaceIntegrationTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "ray-integration", Namespace: "user-ns"},
			Spec: workspacev1alpha1.WorkspaceIntegrationTemplateSpec{
				DisplayName: "Ray Cluster Integration",
				ResourceRefs: []workspacev1alpha1.ResourceRef{
					{ID: "rayCluster", APIVersion: "ray.io/v1", Kind: "RayCluster", Name: "{{ .Parameters.clusterName }}"},
				},
				StatusProbe: &workspacev1alpha1.IntegrationStatusProbe{
					Exec: &corev1.ExecAction{Command: []string{"ray", "status"}},
				},
				DeploymentModifications: &workspacev1alpha1.DeploymentModifications{
					PodModifications: &workspacev1alpha1.PodModifications{
						AdditionalContainers: []corev1.Container{{Name: "ray-sidecar", Image: "ray:2.9"}},
						PrimaryContainerModifications: &workspacev1alpha1.PrimaryContainerModifications{
							MergeEnv: []workspacev1alpha1.AccessEnvTemplate{
								{Name: "RAY_ADDRESS", ValueTemplate: `{{ resource "rayCluster" "{.status.head.serviceName}" }}`},
							},
						},
					},
				},
			},
		}
	})

	// newScheme builds a scheme with the workspace types, core, and the RayCluster GVK registered
	// as unstructured so the fake client can serve it.
	newScheme := func() *runtime.Scheme {
		scheme := runtime.NewScheme()
		Expect(workspacev1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		scheme.AddKnownTypeWithName(rayGVK, &unstructured.Unstructured{})
		listGVK := rayGVK
		listGVK.Kind = rayGVK.Kind + "List"
		scheme.AddKnownTypeWithName(listGVK, &unstructured.UnstructuredList{})
		return scheme
	}

	rayCluster := func(serviceName string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "ray.io/v1",
			"kind":       "RayCluster",
			"metadata":   map[string]interface{}{"name": "my-ray-cluster", "namespace": "user-ns"},
			"status":     map[string]interface{}{"head": map[string]interface{}{"serviceName": serviceName}},
		}}
	}

	// resolvedEnv returns the resolved primary-container MergeEnv from the wi spec, for assertions.
	// Each guard carries a message so a nil intermediate reports which field was missing rather
	// than panicking with a bare nil dereference.
	resolvedEnv := func(w *workspacev1alpha1.WorkspaceIntegration) []workspacev1alpha1.AccessEnvTemplate {
		Expect(w.Spec.DeploymentModifications).NotTo(BeNil(), "spec.deploymentModifications should be set")
		Expect(w.Spec.DeploymentModifications.PodModifications).NotTo(BeNil(), "spec.deploymentModifications.podModifications should be set")
		Expect(w.Spec.DeploymentModifications.PodModifications.PrimaryContainerModifications).NotTo(BeNil(), "spec.deploymentModifications.podModifications.primaryContainerModifications should be set")
		return w.Spec.DeploymentModifications.PodModifications.PrimaryContainerModifications.MergeEnv
	}

	Context("Default (resolve at admission)", func() {
		It("resolves spec output with literal values when the RayCluster is up", func() {
			c := fake.NewClientBuilder().WithScheme(newScheme()).
				WithObjects(template, rayCluster("my-ray-cluster-head-svc")).Build()
			defaulter = &WorkspaceIntegrationCustomDefaulter{client: c}

			Expect(defaulter.Default(ctx, wi)).To(Succeed())

			Expect(wi.Spec.DeploymentModifications).NotTo(BeNil())
			containers := wi.Spec.DeploymentModifications.PodModifications.AdditionalContainers
			Expect(containers).To(HaveLen(1))
			Expect(containers[0].Name).To(Equal("ray-sidecar"))
			env := resolvedEnv(wi)
			Expect(env).To(HaveLen(1))
			Expect(env[0].ValueTemplate).To(Equal("my-ray-cluster-head-svc"))
		})

		It("freezes the statusProbe onto spec.statusProbe", func() {
			c := fake.NewClientBuilder().WithScheme(newScheme()).
				WithObjects(template, rayCluster("svc")).Build()
			defaulter = &WorkspaceIntegrationCustomDefaulter{client: c}

			Expect(defaulter.Default(ctx, wi)).To(Succeed())
			Expect(wi.Spec.StatusProbe).NotTo(BeNil())
			Expect(wi.Spec.StatusProbe.Exec.Command).To(Equal([]string{"ray", "status"}))
		})

		It("fails closed (rejects admission) when the RayCluster is not found", func() {
			c := fake.NewClientBuilder().WithScheme(newScheme()).
				WithObjects(template).Build() // no RayCluster seeded
			defaulter = &WorkspaceIntegrationCustomDefaulter{client: c}

			err := defaulter.Default(ctx, wi)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("fails closed when the template is not found", func() {
			c := fake.NewClientBuilder().WithScheme(newScheme()).Build() // no template
			defaulter = &WorkspaceIntegrationCustomDefaulter{client: c}

			err := defaulter.Default(ctx, wi)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("integration template"))
		})

		It("re-resolves on update, overwriting a stale resolved value", func() {
			c := fake.NewClientBuilder().WithScheme(newScheme()).
				WithObjects(template, rayCluster("new-head-svc")).Build()
			defaulter = &WorkspaceIntegrationCustomDefaulter{client: c}

			// Simulate a stale prior resolution (e.g. an old cluster's service name).
			wi.Spec.DeploymentModifications = &workspacev1alpha1.DeploymentModifications{
				PodModifications: &workspacev1alpha1.PodModifications{
					PrimaryContainerModifications: &workspacev1alpha1.PrimaryContainerModifications{
						MergeEnv: []workspacev1alpha1.AccessEnvTemplate{
							{Name: "RAY_ADDRESS", ValueTemplate: "old-head-svc"},
						},
					},
				},
			}

			Expect(defaulter.Default(ctx, wi)).To(Succeed())
			env := resolvedEnv(wi)
			Expect(env).To(HaveLen(1))
			Expect(env[0].ValueTemplate).To(Equal("new-head-svc"),
				"update re-resolves from current cluster state, replacing the stale value")
		})

		It("rejects a non-WorkspaceIntegration object", func() {
			c := fake.NewClientBuilder().WithScheme(newScheme()).Build()
			defaulter = &WorkspaceIntegrationCustomDefaulter{client: c}

			err := defaulter.Default(ctx, &workspacev1alpha1.Workspace{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a WorkspaceIntegration"))
		})
	})
})
