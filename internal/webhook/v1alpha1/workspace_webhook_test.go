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

// createUserContext creates a context with user information for testing
func createUserContext(baseCtx context.Context, operation, username string, groups ...string) context.Context {
	userInfo := &authenticationv1.UserInfo{Username: username, Groups: groups}
	req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{UserInfo: *userInfo, Operation: admissionv1.Operation(operation)}}
	return admission.NewContextWithRequest(baseCtx, req)
}

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
				OwnershipType: "Public",
			},
		}
		defaulter = WorkspaceCustomDefaulter{}
		validator = WorkspaceCustomValidator{}
		ctx = context.Background()
	})

	Context("Defaulter", func() {
		It("should add created-by annotation when none exists", func() {
			ctx = createUserContext(ctx, "CREATE", "test-user")

			err := defaulter.Default(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(workspace.Annotations).To(HaveKey(controller.AnnotationCreatedBy))
			Expect(workspace.Annotations[controller.AnnotationCreatedBy]).To(Equal("test-user"))
			Expect(workspace.Annotations).To(HaveKey(controller.AnnotationLastUpdatedBy))
			Expect(workspace.Annotations[controller.AnnotationLastUpdatedBy]).To(Equal("test-user"))
		})

		It("should not overwrite existing created-by annotation", func() {
			workspace.Annotations = map[string]string{controller.AnnotationCreatedBy: "original-user"}
			ctx = createUserContext(ctx, "UPDATE", "new-user")

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
			ctx = createUserContext(ctx, "UPDATE", "new-user")

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

		It("should not add created-by annotation for UPDATE operations", func() {
			updateCtx := createUserContext(ctx, "UPDATE", "update-user")

			err := defaulter.Default(updateCtx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(workspace.Annotations).NotTo(HaveKey(controller.AnnotationCreatedBy))
			Expect(workspace.Annotations).To(HaveKey(controller.AnnotationLastUpdatedBy))
			Expect(workspace.Annotations[controller.AnnotationLastUpdatedBy]).To(Equal("update-user"))
		})
	})

	Context("Validator", func() {
		It("should validate workspace creation successfully", func() {
			warnings, err := validator.ValidateCreate(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should validate workspace update successfully", func() {
			userCtx := createUserContext(ctx, "UPDATE", "test-user")

			oldWorkspace := workspace.DeepCopy()
			workspace.Spec.DisplayName = "Updated Workspace"

			warnings, err := validator.ValidateUpdate(userCtx, oldWorkspace, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should reject OwnerOnly workspace update by non-owner", func() {
			userCtx := createUserContext(ctx, "UPDATE", "different-user")

			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Spec.OwnershipType = webhookconst.OwnershipTypeOwnerOnly
			oldWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "original-user",
			}
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Spec.OwnershipType = webhookconst.OwnershipTypeOwnerOnly
			newWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "original-user",
			}

			warnings, err := validator.ValidateUpdate(userCtx, oldWorkspace, newWorkspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("access denied"))
			Expect(warnings).To(BeEmpty())
		})

		It("should allow Public workspace update", func() {
			userCtx := createUserContext(ctx, "UPDATE", "test-user")

			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Spec.OwnershipType = webhookconst.OwnershipTypePublic
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Spec.OwnershipType = webhookconst.OwnershipTypePublic
			newWorkspace.Spec.Image = "jupyter/scipy-notebook:latest"

			warnings, err := validator.ValidateUpdate(userCtx, oldWorkspace, newWorkspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should allow OwnerOnly workspace update by owner", func() {
			userCtx := createUserContext(ctx, "UPDATE", "owner-user")

			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Spec.OwnershipType = webhookconst.OwnershipTypeOwnerOnly
			oldWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "owner-user",
			}
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Spec.OwnershipType = webhookconst.OwnershipTypeOwnerOnly
			newWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "owner-user",
			}

			warnings, err := validator.ValidateUpdate(userCtx, oldWorkspace, newWorkspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should allow OwnerOnly workspace deletion by owner", func() {
			userCtx := createUserContext(ctx, "DELETE", "owner-user")

			ownerOnlyWorkspace := workspace.DeepCopy()
			ownerOnlyWorkspace.Spec.OwnershipType = webhookconst.OwnershipTypeOwnerOnly
			ownerOnlyWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "owner-user",
			}

			warnings, err := validator.ValidateDelete(userCtx, ownerOnlyWorkspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should reject update that changes created-by annotation", func() {
			userCtx := createUserContext(ctx, "UPDATE", "different-user")

			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "original-user",
			}
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "malicious-user",
			}

			warnings, err := validator.ValidateUpdate(userCtx, oldWorkspace, newWorkspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("created-by annotation is immutable"))
			Expect(warnings).To(BeEmpty())
		})

		It("should allow changing ownershipType from OwnerOnly to Public", func() {
			ownerCtx := createUserContext(ctx, "UPDATE", "owner-user")

			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Spec.OwnershipType = webhookconst.OwnershipTypeOwnerOnly
			oldWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "owner-user",
			}
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Spec.OwnershipType = webhookconst.OwnershipTypePublic
			newWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "owner-user",
			}

			warnings, err := validator.ValidateUpdate(ownerCtx, oldWorkspace, newWorkspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should reject changing ownershipType from Public to OwnerOnly by non-owner", func() {
			nonOwnerCtx := createUserContext(ctx, "UPDATE", "different-user")

			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Spec.OwnershipType = webhookconst.OwnershipTypePublic
			oldWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "original-user",
			}
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Spec.OwnershipType = webhookconst.OwnershipTypeOwnerOnly
			newWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "original-user",
			}

			warnings, err := validator.ValidateUpdate(nonOwnerCtx, oldWorkspace, newWorkspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("access denied"))
			Expect(warnings).To(BeEmpty())
		})

		It("should allow changing ownershipType from Public to OwnerOnly by workspace creator", func() {
			creatorCtx := createUserContext(ctx, "UPDATE", "creator-user")

			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Spec.OwnershipType = webhookconst.OwnershipTypePublic
			oldWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "creator-user",
			}
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Spec.OwnershipType = webhookconst.OwnershipTypeOwnerOnly
			newWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "creator-user",
			}

			warnings, err := validator.ValidateUpdate(creatorCtx, oldWorkspace, newWorkspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should allow changing ownershipType from Public to OwnerOnly by admin", func() {
			adminCtx := createUserContext(ctx, "UPDATE", "admin-user", "system:masters")

			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Spec.OwnershipType = webhookconst.OwnershipTypePublic
			oldWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "original-user",
			}
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Spec.OwnershipType = webhookconst.OwnershipTypeOwnerOnly
			newWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "original-user",
			}

			warnings, err := validator.ValidateUpdate(adminCtx, oldWorkspace, newWorkspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should reject update that removes created-by annotation", func() {
			userCtx := createUserContext(ctx, "UPDATE", "different-user")

			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "original-user",
				"other-annotation":             "other-value",
			}
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Annotations = map[string]string{
				"other-annotation": "other-value",
			}

			warnings, err := validator.ValidateUpdate(userCtx, oldWorkspace, newWorkspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("created-by annotation is immutable"))
			Expect(warnings).To(BeEmpty())
		})

		It("should reject update that removes all annotations", func() {
			userCtx := createUserContext(ctx, "UPDATE", "different-user")

			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "original-user",
				"other-annotation":             "other-value",
			}
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Annotations = nil

			warnings, err := validator.ValidateUpdate(userCtx, oldWorkspace, newWorkspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("created-by annotation cannot be removed"))
			Expect(warnings).To(BeEmpty())
		})

		It("should allow admin to modify created-by annotation", func() {
			adminCtx := createUserContext(ctx, "UPDATE", "admin-user", "system:masters")

			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "original-user",
			}
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "new-user",
			}

			warnings, err := validator.ValidateUpdate(adminCtx, oldWorkspace, newWorkspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should reject update that sets created-by annotation to empty string", func() {
			userCtx := createUserContext(ctx, "UPDATE", "different-user")

			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "original-user",
			}
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "",
			}

			warnings, err := validator.ValidateUpdate(userCtx, oldWorkspace, newWorkspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("created-by annotation is immutable"))
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

	Context("validateOwnershipPermission", func() {
		var ownerOnlyWorkspace *workspacesv1alpha1.Workspace

		BeforeEach(func() {
			ownerOnlyWorkspace = workspace.DeepCopy()
			ownerOnlyWorkspace.Spec.OwnershipType = webhookconst.OwnershipTypeOwnerOnly
			ownerOnlyWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "owner-user",
			}
		})

		It("should allow owner access", func() {
			userInfo := &authenticationv1.UserInfo{Username: "owner-user"}
			req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{UserInfo: *userInfo}}
			userCtx := admission.NewContextWithRequest(ctx, req)

			err := validateOwnershipPermission(userCtx, ownerOnlyWorkspace)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should deny non-owner access", func() {
			userInfo := &authenticationv1.UserInfo{Username: "different-user"}
			req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{UserInfo: *userInfo}}
			userCtx := admission.NewContextWithRequest(ctx, req)

			err := validateOwnershipPermission(userCtx, ownerOnlyWorkspace)
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

			err := validateOwnershipPermission(userCtx, ownerOnlyWorkspace)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should deny access when no request context", func() {
			err := validateOwnershipPermission(ctx, ownerOnlyWorkspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to extract user information"))
		})

		It("should allow system:masters access", func() {
			userInfo := &authenticationv1.UserInfo{
				Username: "admin-user",
				Groups:   []string{"system:masters"},
			}
			req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{UserInfo: *userInfo}}
			userCtx := admission.NewContextWithRequest(ctx, req)

			err := validateOwnershipPermission(userCtx, ownerOnlyWorkspace)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
