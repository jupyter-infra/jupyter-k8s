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
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	workspacesv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/controller"
)

var _ = Describe("Workspace Webhook", func() {
	var (
		workspace *workspacesv1alpha1.Workspace
		defaulter WorkspaceCustomDefaulter
		validator WorkspaceCustomValidator
		ctx       context.Context
	)

	BeforeEach(func() {
		workspace = &workspacesv1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workspace",
				Namespace: "default",
			},
			Spec: workspacesv1alpha1.WorkspaceSpec{
				DisplayName:   "Test Workspace",
				Image:         "jupyter/base-notebook:latest",
				DesiredStatus: "Running",
			},
		}
		defaulter = WorkspaceCustomDefaulter{}
		validator = WorkspaceCustomValidator{}
		ctx = context.Background()
	})

	Context("Defaulter", func() {
		It("should add created-by annotation when none exists", func() {
			userInfo := &authenticationv1.UserInfo{Username: "test-user"}
			req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{UserInfo: *userInfo}}
			ctx = admission.NewContextWithRequest(ctx, req)

			err := defaulter.Default(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(workspace.Annotations).To(HaveKey(controller.AnnotationCreatedBy))
			Expect(workspace.Annotations[controller.AnnotationCreatedBy]).To(Equal("test-user"))
			Expect(workspace.Annotations).To(HaveKey(controller.AnnotationLastUpdatedBy))
			Expect(workspace.Annotations[controller.AnnotationLastUpdatedBy]).To(Equal("test-user"))
		})

		It("should not overwrite existing created-by annotation", func() {
			workspace.Annotations = map[string]string{controller.AnnotationCreatedBy: "original-user"}
			userInfo := &authenticationv1.UserInfo{Username: "new-user"}
			req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{UserInfo: *userInfo}}
			ctx = admission.NewContextWithRequest(ctx, req)

			err := defaulter.Default(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(workspace.Annotations[controller.AnnotationCreatedBy]).To(Equal("original-user"))
			Expect(workspace.Annotations[controller.AnnotationLastUpdatedBy]).To(Equal("new-user"))
		})

		It("should preserve existing annotations", func() {
			workspace.Annotations = map[string]string{
				"custom-annotation":            "custom-value",
				controller.AnnotationCreatedBy: "original-user",
			}
			userInfo := &authenticationv1.UserInfo{Username: "new-user"}
			req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{UserInfo: *userInfo}}
			ctx = admission.NewContextWithRequest(ctx, req)

			err := defaulter.Default(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(workspace.Annotations["custom-annotation"]).To(Equal("custom-value"))
			Expect(workspace.Annotations[controller.AnnotationCreatedBy]).To(Equal("original-user"))
			Expect(workspace.Annotations[controller.AnnotationLastUpdatedBy]).To(Equal("new-user"))
		})

		It("should handle missing user info gracefully", func() {
			err := defaulter.Default(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			// Webhook initializes empty annotations map but doesn't add user annotations
			Expect(workspace.Annotations).To(Equal(map[string]string{}))
		})

		It("should return error for wrong object type", func() {
			wrongObj := &runtime.Unknown{}
			err := defaulter.Default(ctx, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected an Workspace object"))
		})
	})

	Context("Validator", func() {
		It("should validate workspace creation successfully", func() {
			warnings, err := validator.ValidateCreate(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should validate workspace update successfully", func() {
			oldWorkspace := workspace.DeepCopy()
			workspace.Spec.DisplayName = "Updated Workspace"

			warnings, err := validator.ValidateUpdate(ctx, oldWorkspace, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should validate workspace deletion successfully", func() {
			warnings, err := validator.ValidateDelete(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should return error for wrong object type in create", func() {
			wrongObj := &runtime.Unknown{}
			warnings, err := validator.ValidateCreate(ctx, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("expected a Workspace object"))
		})

		It("should return error for wrong object type in update", func() {
			wrongObj := &runtime.Unknown{}
			warnings, err := validator.ValidateUpdate(ctx, workspace, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("expected a Workspace object"))
		})

		It("should return error for wrong object type in delete", func() {
			wrongObj := &runtime.Unknown{}
			warnings, err := validator.ValidateDelete(ctx, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("expected a Workspace object"))
		})
	})
})
