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
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	workspacesv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/controller"
	webhookconst "github.com/jupyter-ai-contrib/jupyter-k8s/internal/webhook"
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
				AccessType:    "Public",
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

		It("should reject OwnerOnly workspace update by non-owner", func() {
			userInfo := &authenticationv1.UserInfo{Username: "different-user"}
			req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{UserInfo: *userInfo}}
			userCtx := admission.NewContextWithRequest(ctx, req)

			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Spec.AccessType = webhookconst.AccessTypeOwnerOnly
			oldWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "original-user",
			}
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Spec.AccessType = webhookconst.AccessTypeOwnerOnly
			newWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "original-user",
			}

			warnings, err := validator.ValidateUpdate(userCtx, oldWorkspace, newWorkspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("access denied"))
			Expect(warnings).To(BeEmpty())
		})

		It("should allow Public workspace update", func() {
			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Spec.AccessType = webhookconst.AccessTypePublic
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Spec.AccessType = webhookconst.AccessTypePublic
			newWorkspace.Spec.Image = "jupyter/scipy-notebook:latest"

			warnings, err := validator.ValidateUpdate(ctx, oldWorkspace, newWorkspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should allow OwnerOnly workspace update by owner", func() {
			userInfo := &authenticationv1.UserInfo{Username: "owner-user"}
			req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{UserInfo: *userInfo}}
			userCtx := admission.NewContextWithRequest(ctx, req)

			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Spec.AccessType = webhookconst.AccessTypeOwnerOnly
			oldWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "owner-user",
			}
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Spec.AccessType = webhookconst.AccessTypeOwnerOnly
			newWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "owner-user",
			}

			warnings, err := validator.ValidateUpdate(userCtx, oldWorkspace, newWorkspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should allow OwnerOnly workspace deletion by owner", func() {
			userInfo := &authenticationv1.UserInfo{Username: "owner-user"}
			req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{UserInfo: *userInfo}}
			userCtx := admission.NewContextWithRequest(ctx, req)

			ownerOnlyWorkspace := workspace.DeepCopy()
			ownerOnlyWorkspace.Spec.AccessType = webhookconst.AccessTypeOwnerOnly
			ownerOnlyWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "owner-user",
			}

			warnings, err := validator.ValidateDelete(userCtx, ownerOnlyWorkspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should reject update that changes created-by annotation", func() {
			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "original-user",
			}
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "malicious-user",
			}

			warnings, err := validator.ValidateUpdate(ctx, oldWorkspace, newWorkspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("created-by annotation is immutable"))
			Expect(warnings).To(BeEmpty())
		})

		It("should reject changing accessType from Public to OwnerOnly", func() {
			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Spec.AccessType = webhookconst.AccessTypePublic
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Spec.AccessType = webhookconst.AccessTypeOwnerOnly

			warnings, err := validator.ValidateUpdate(ctx, oldWorkspace, newWorkspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot change accessType from Public to OwnerOnly"))
			Expect(warnings).To(BeEmpty())
		})

		It("should allow changing accessType from OwnerOnly to Public", func() {
			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Spec.AccessType = webhookconst.AccessTypeOwnerOnly
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Spec.AccessType = webhookconst.AccessTypePublic

			warnings, err := validator.ValidateUpdate(ctx, oldWorkspace, newWorkspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})
	})

	Context("sanitizeUsername", func() {
		It("should handle normal usernames", func() {
			Expect(sanitizeUsername("user123")).To(Equal("user123"))
			Expect(sanitizeUsername("test@example.com")).To(Equal("test@example.com"))
			Expect(sanitizeUsername("arn:aws:iam::123456789012:role/EKSRole")).To(Equal("arn:aws:iam::123456789012:role/EKSRole"))
		})

		It("should escape special characters", func() {
			Expect(sanitizeUsername("user\nname")).To(Equal("user\\nname"))
			Expect(sanitizeUsername("user\tname")).To(Equal("user\\tname"))
			Expect(sanitizeUsername("user\"name")).To(Equal("user\\\"name"))
			Expect(sanitizeUsername("user\\name")).To(Equal("user\\\\name"))
		})

		It("should handle unicode characters", func() {
			Expect(sanitizeUsername("用户")).To(Equal("用户"))
			Expect(sanitizeUsername("user🚀")).To(Equal("user🚀"))
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

	Context("validateEditPermission", func() {
		var ownerOnlyWorkspace *workspacesv1alpha1.Workspace

		BeforeEach(func() {
			ownerOnlyWorkspace = workspace.DeepCopy()
			ownerOnlyWorkspace.Spec.AccessType = webhookconst.AccessTypeOwnerOnly
			ownerOnlyWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "owner-user",
			}
		})

		It("should allow owner access", func() {
			userInfo := &authenticationv1.UserInfo{Username: "owner-user"}
			req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{UserInfo: *userInfo}}
			userCtx := admission.NewContextWithRequest(ctx, req)

			err := validateEditPermission(userCtx, ownerOnlyWorkspace)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should deny non-owner access", func() {
			userInfo := &authenticationv1.UserInfo{Username: "different-user"}
			req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{UserInfo: *userInfo}}
			userCtx := admission.NewContextWithRequest(ctx, req)

			err := validateEditPermission(userCtx, ownerOnlyWorkspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("access denied"))
		})

		It("should allow cluster admin access", func() {
			userInfo := &authenticationv1.UserInfo{
				Username: "admin-user",
				Groups:   []string{"system:masters"},
			}
			req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{UserInfo: *userInfo}}
			userCtx := admission.NewContextWithRequest(ctx, req)
			Expect(os.Setenv("CLUSTER_ADMIN_GROUP", "system:masters")).To(Succeed())
			defer func() { _ = os.Unsetenv("CLUSTER_ADMIN_GROUP") }()

			err := validateEditPermission(userCtx, ownerOnlyWorkspace)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should deny access when no request context", func() {
			err := validateEditPermission(ctx, ownerOnlyWorkspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to extract user information"))
		})
	})
})
