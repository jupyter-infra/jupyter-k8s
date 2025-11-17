/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/controller"
	workspaceutil "github.com/jupyter-ai-contrib/jupyter-k8s/internal/workspace"
)

// Tests are integrated into the main Webhook Suite via webhook_suite_test.go

var _ = Describe("Lazy Finalizer Logic", func() {
	var (
		ctx       context.Context
		scheme    *runtime.Scheme
		k8sClient client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(workspacev1alpha1.AddToScheme(scheme)).To(Succeed())
	})

	Context("ensureTemplateFinalizer", func() {
		It("should not add finalizer when no active workspaces exist", func() {
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-template",
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Test Template",
					DefaultImage: "test:latest",
				},
			}

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(template).
				Build()

			err := ensureTemplateFinalizer(ctx, k8sClient, "test-template", "default")
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer was NOT added
			updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: "test-template", Namespace: "default"}, updatedTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(controllerutil.ContainsFinalizer(updatedTemplate, workspaceutil.TemplateFinalizerName)).To(BeFalse())
		})

		It("should add finalizer when active workspace exists", func() {
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-template",
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Test Template",
					DefaultImage: "test:latest",
				},
			}

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace",
					Namespace: "default",
					Labels: map[string]string{
						controller.LabelWorkspaceTemplate:          "test-template",
						controller.LabelWorkspaceTemplateNamespace: "default",
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      "test-template",
						Namespace: "default",
					},
				},
			}

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(template, workspace).
				Build()

			err := ensureTemplateFinalizer(ctx, k8sClient, "test-template", "default")
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer WAS added
			updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: "test-template", Namespace: "default"}, updatedTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(controllerutil.ContainsFinalizer(updatedTemplate, workspaceutil.TemplateFinalizerName)).To(BeTrue())
		})

		It("should not add finalizer when all workspaces have DeletionTimestamp", func() {
			now := metav1.Now()
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-template",
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Test Template",
					DefaultImage: "test:latest",
				},
			}

			// Per K8s semantics, objects with DeletionTimestamp must have finalizers
			// This represents a workspace that is being deleted but blocked by finalizers
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-workspace",
					Namespace:         "default",
					DeletionTimestamp: &now,
					Finalizers:        []string{"test-finalizer"}, // Required to set DeletionTimestamp
					Labels: map[string]string{
						controller.LabelWorkspaceTemplate:          "test-template",
						controller.LabelWorkspaceTemplateNamespace: "default",
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      "test-template",
						Namespace: "default",
					},
				},
			}

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(template, workspace).
				Build()

			err := ensureTemplateFinalizer(ctx, k8sClient, "test-template", "default")
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer was NOT added (workspace is being deleted)
			// ListActiveWorkspacesByTemplate filters out workspaces with DeletionTimestamp
			updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: "test-template", Namespace: "default"}, updatedTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(controllerutil.ContainsFinalizer(updatedTemplate, workspaceutil.TemplateFinalizerName)).To(BeFalse())
		})

		It("should handle namespace filtering correctly", func() {
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-template",
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Test Template",
					DefaultImage: "test:latest",
				},
			}

			// Workspace in different namespace
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace",
					Namespace: "default",
					Labels: map[string]string{
						controller.LabelWorkspaceTemplate:          "test-template",
						controller.LabelWorkspaceTemplateNamespace: "other-namespace",
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      "test-template",
						Namespace: "other-namespace",
					},
				},
			}

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(template, workspace).
				Build()

			// Query for template in "default" namespace - should not find workspace
			err := ensureTemplateFinalizer(ctx, k8sClient, "test-template", "default")
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer was NOT added (workspace references different namespace)
			updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: "test-template", Namespace: "default"}, updatedTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(controllerutil.ContainsFinalizer(updatedTemplate, workspaceutil.TemplateFinalizerName)).To(BeFalse())
		})

		It("should skip finalizer addition when template does not exist", func() {
			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				Build()

			// Should not error when template doesn't exist
			err := ensureTemplateFinalizer(ctx, k8sClient, "nonexistent-template", "default")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should not re-add finalizer if already present", func() {
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-template",
					Namespace:  "default",
					Finalizers: []string{workspaceutil.TemplateFinalizerName},
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Test Template",
					DefaultImage: "test:latest",
				},
			}

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace",
					Namespace: "default",
					Labels: map[string]string{
						controller.LabelWorkspaceTemplate:          "test-template",
						controller.LabelWorkspaceTemplateNamespace: "default",
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      "test-template",
						Namespace: "default",
					},
				},
			}

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(template, workspace).
				Build()

			err := ensureTemplateFinalizer(ctx, k8sClient, "test-template", "default")
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer still present (only one)
			updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: "test-template", Namespace: "default"}, updatedTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(controllerutil.ContainsFinalizer(updatedTemplate, workspaceutil.TemplateFinalizerName)).To(BeTrue())
			Expect(updatedTemplate.Finalizers).To(HaveLen(1))
		})

		It("should use limit=1 optimization by returning early", func() {
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-template",
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Test Template",
					DefaultImage: "test:latest",
				},
			}

			// Create multiple workspaces
			workspace1 := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace-1",
					Namespace: "default",
					Labels: map[string]string{
						controller.LabelWorkspaceTemplate:          "test-template",
						controller.LabelWorkspaceTemplateNamespace: "default",
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace 1",
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      "test-template",
						Namespace: "default",
					},
				},
			}

			workspace2 := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace-2",
					Namespace: "default",
					Labels: map[string]string{
						controller.LabelWorkspaceTemplate:          "test-template",
						controller.LabelWorkspaceTemplateNamespace: "default",
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace 2",
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      "test-template",
						Namespace: "default",
					},
				},
			}

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(template, workspace1, workspace2).
				Build()

			// Should add finalizer after finding first workspace (limit=1 optimization)
			err := ensureTemplateFinalizer(ctx, k8sClient, "test-template", "default")
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer was added
			updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: "test-template", Namespace: "default"}, updatedTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(controllerutil.ContainsFinalizer(updatedTemplate, workspaceutil.TemplateFinalizerName)).To(BeTrue())
		})
	})
})
