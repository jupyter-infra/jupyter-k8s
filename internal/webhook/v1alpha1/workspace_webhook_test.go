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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
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
		workspace *workspacev1alpha1.Workspace
		defaulter WorkspaceCustomDefaulter
		validator WorkspaceCustomValidator
		ctx       context.Context
	)

	BeforeEach(func() {
		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workspace",
				Namespace: "default",
			},
			Spec: workspacev1alpha1.WorkspaceSpec{
				DisplayName:   "Test Workspace",
				Image:         "jupyter/base-notebook:latest",
				DesiredStatus: "Running",
				OwnershipType: "Public",
			},
		}

		mockClient := &MockClient{}
		defaulter = WorkspaceCustomDefaulter{
			templateDefaulter:       NewTemplateDefaulter(mockClient),
			serviceAccountDefaulter: NewServiceAccountDefaulter(mockClient),
			templateGetter:          NewTemplateGetter(mockClient),
		}
		validator = WorkspaceCustomValidator{
			templateValidator:       NewTemplateValidator(mockClient),
			serviceAccountValidator: NewServiceAccountValidator(mockClient),
		}
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
		var ownerOnlyWorkspace *workspacev1alpha1.Workspace

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

		It("should deny access when no request context", func() {
			err := validateOwnershipPermission(ctx, ownerOnlyWorkspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to extract user information"))
		})
	})

	Context("Template Validator Functions", func() {
		var template *workspacev1alpha1.WorkspaceTemplate

		BeforeEach(func() {
			template = &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "test-template"},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					AllowedImages: []string{"jupyter/base-notebook:latest", "jupyter/scipy-notebook:latest"},
					DefaultImage:  "jupyter/base-notebook:latest",
				},
			}
		})

		Context("validateImageAllowed", func() {
			It("should allow image in allowed list", func() {
				violation := validateImageAllowed("jupyter/base-notebook:latest", template)
				Expect(violation).To(BeNil())
			})

			It("should reject image not in allowed list", func() {
				violation := validateImageAllowed("malicious/image:latest", template)
				Expect(violation).NotTo(BeNil())
				Expect(violation.Type).To(Equal(controller.ViolationTypeImageNotAllowed))
				Expect(violation.Message).To(ContainSubstring("malicious/image:latest"))
				Expect(violation.Message).To(ContainSubstring("test-template"))
			})

			It("should use default image when allowed list is empty", func() {
				template.Spec.AllowedImages = []string{}
				violation := validateImageAllowed("jupyter/base-notebook:latest", template)
				Expect(violation).To(BeNil())
			})

			It("should reject when allowed list is empty and image doesn't match default", func() {
				template.Spec.AllowedImages = []string{}
				violation := validateImageAllowed("other/image:latest", template)
				Expect(violation).NotTo(BeNil())
				Expect(violation.Type).To(Equal(controller.ViolationTypeImageNotAllowed))
			})

			It("should allow any image when AllowCustomImages is true", func() {
				allowCustomImages := true
				template.Spec.AllowCustomImages = &allowCustomImages
				violation := validateImageAllowed("any/custom:image", template)
				Expect(violation).To(BeNil())
			})

			It("should still enforce restrictions when AllowCustomImages is false", func() {
				allowCustomImages := false
				template.Spec.AllowCustomImages = &allowCustomImages
				violation := validateImageAllowed("malicious/image:latest", template)
				Expect(violation).NotTo(BeNil())
				Expect(violation.Type).To(Equal(controller.ViolationTypeImageNotAllowed))
			})

			It("should enforce restrictions when AllowCustomImages is nil (default)", func() {
				template.Spec.AllowCustomImages = nil
				violation := validateImageAllowed("malicious/image:latest", template)
				Expect(violation).NotTo(BeNil())
				Expect(violation.Type).To(Equal(controller.ViolationTypeImageNotAllowed))
			})
		})

		Context("validateStorageSize", func() {
			BeforeEach(func() {
				template.Spec.PrimaryStorage = &workspacev1alpha1.StorageConfig{
					MinSize: &[]resource.Quantity{resource.MustParse("1Gi")}[0],
					MaxSize: &[]resource.Quantity{resource.MustParse("10Gi")}[0],
				}
			})

			It("should allow storage within bounds", func() {
				violation := validateStorageSize(resource.MustParse("5Gi"), template)
				Expect(violation).To(BeNil())
			})

			It("should reject storage below minimum", func() {
				violation := validateStorageSize(resource.MustParse("500Mi"), template)
				Expect(violation).NotTo(BeNil())
				Expect(violation.Type).To(Equal(controller.ViolationTypeStorageExceeded))
				Expect(violation.Message).To(ContainSubstring("below minimum"))
				Expect(violation.Message).To(ContainSubstring("test-template"))
			})

			It("should reject storage above maximum", func() {
				violation := validateStorageSize(resource.MustParse("20Gi"), template)
				Expect(violation).NotTo(BeNil())
				Expect(violation.Type).To(Equal(controller.ViolationTypeStorageExceeded))
				Expect(violation.Message).To(ContainSubstring("exceeds maximum"))
				Expect(violation.Message).To(ContainSubstring("test-template"))
			})

			It("should allow any size when no storage config", func() {
				template.Spec.PrimaryStorage = nil
				violation := validateStorageSize(resource.MustParse("100Gi"), template)
				Expect(violation).To(BeNil())
			})
		})

		Context("storageEqual", func() {
			It("should return true for nil storages", func() {
				Expect(storageEqual(nil, nil)).To(BeTrue())
			})

			It("should return false when one is nil", func() {
				storage := &workspacev1alpha1.StorageSpec{Size: resource.MustParse("1Gi")}
				Expect(storageEqual(nil, storage)).To(BeFalse())
				Expect(storageEqual(storage, nil)).To(BeFalse())
			})

			It("should return true for equal storages", func() {
				storage1 := &workspacev1alpha1.StorageSpec{
					Size:      resource.MustParse("1Gi"),
					MountPath: "/data",
				}
				storage2 := &workspacev1alpha1.StorageSpec{
					Size:      resource.MustParse("1Gi"),
					MountPath: "/data",
				}
				Expect(storageEqual(storage1, storage2)).To(BeTrue())
			})

			It("should return false for different sizes", func() {
				storage1 := &workspacev1alpha1.StorageSpec{Size: resource.MustParse("1Gi")}
				storage2 := &workspacev1alpha1.StorageSpec{Size: resource.MustParse("2Gi")}
				Expect(storageEqual(storage1, storage2)).To(BeFalse())
			})

			It("should return false for different mount paths", func() {
				storage1 := &workspacev1alpha1.StorageSpec{
					Size:      resource.MustParse("1Gi"),
					MountPath: "/data1",
				}
				storage2 := &workspacev1alpha1.StorageSpec{
					Size:      resource.MustParse("1Gi"),
					MountPath: "/data2",
				}
				Expect(storageEqual(storage1, storage2)).To(BeFalse())
			})
		})

		Context("validateResourceBounds", func() {
			BeforeEach(func() {
				template.Spec.ResourceBounds = &workspacev1alpha1.ResourceBounds{
					CPU: &workspacev1alpha1.ResourceRange{
						Min: resource.MustParse("100m"),
						Max: resource.MustParse("2"),
					},
					Memory: &workspacev1alpha1.ResourceRange{
						Min: resource.MustParse("128Mi"),
						Max: resource.MustParse("4Gi"),
					},
				}
			})

			It("should allow resources within bounds", func() {
				resources := corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
				}
				violations := validateResourceBounds(resources, template)
				Expect(violations).To(BeEmpty())
			})

			It("should reject CPU below minimum", func() {
				resources := corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("50m"),
					},
				}
				violations := validateResourceBounds(resources, template)
				Expect(violations).To(HaveLen(1))
				Expect(violations[0].Type).To(Equal(controller.ViolationTypeResourceExceeded))
				Expect(violations[0].Message).To(ContainSubstring("below minimum"))
				Expect(violations[0].Message).To(ContainSubstring("test-template"))
			})

			It("should reject CPU above maximum", func() {
				resources := corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("4"),
					},
				}
				violations := validateResourceBounds(resources, template)
				Expect(violations).To(HaveLen(1))
				Expect(violations[0].Type).To(Equal(controller.ViolationTypeResourceExceeded))
				Expect(violations[0].Message).To(ContainSubstring("exceeds maximum"))
				Expect(violations[0].Message).To(ContainSubstring("test-template"))
			})

			It("should reject memory below minimum", func() {
				resources := corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("64Mi"),
					},
				}
				violations := validateResourceBounds(resources, template)
				Expect(violations).To(HaveLen(1))
				Expect(violations[0].Type).To(Equal(controller.ViolationTypeResourceExceeded))
				Expect(violations[0].Message).To(ContainSubstring("below minimum"))
			})

			It("should reject memory above maximum", func() {
				resources := corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("8Gi"),
					},
				}
				violations := validateResourceBounds(resources, template)
				Expect(violations).To(HaveLen(1))
				Expect(violations[0].Type).To(Equal(controller.ViolationTypeResourceExceeded))
				Expect(violations[0].Message).To(ContainSubstring("exceeds maximum"))
			})

			It("should reject CPU limit less than request", func() {
				resources := corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("1"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("500m"),
					},
				}
				violations := validateResourceBounds(resources, template)
				Expect(violations).To(HaveLen(1))
				Expect(violations[0].Message).To(ContainSubstring("CPU limit must be greater than or equal to CPU request"))
			})

			It("should reject memory limit less than request", func() {
				resources := corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
				}
				violations := validateResourceBounds(resources, template)
				Expect(violations).To(HaveLen(1))
				Expect(violations[0].Message).To(ContainSubstring("Memory limit must be greater than or equal to memory request"))
			})

			It("should allow resources when no bounds defined", func() {
				template.Spec.ResourceBounds = nil
				resources := corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("10"),
						corev1.ResourceMemory: resource.MustParse("100Gi"),
					},
				}
				violations := validateResourceBounds(resources, template)
				Expect(violations).To(BeEmpty())
			})
		})

		Context("resourcesEqual", func() {
			It("should return true for nil resources", func() {
				Expect(resourcesEqual(nil, nil)).To(BeTrue())
			})

			It("should return false when one is nil", func() {
				resources := &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
				}
				Expect(resourcesEqual(nil, resources)).To(BeFalse())
				Expect(resourcesEqual(resources, nil)).To(BeFalse())
			})

			It("should return true for equal resources", func() {
				resources1 := &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
					Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2")},
				}
				resources2 := &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
					Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2")},
				}
				Expect(resourcesEqual(resources1, resources2)).To(BeTrue())
			})

			It("should return false for different requests", func() {
				resources1 := &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
				}
				resources2 := &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2")},
				}
				Expect(resourcesEqual(resources1, resources2)).To(BeFalse())
			})

			It("should return false for different limits", func() {
				resources1 := &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
				}
				resources2 := &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2")},
				}
				Expect(resourcesEqual(resources1, resources2)).To(BeFalse())
			})
		})
	})

	Context("isAdminUser", func() {
		It("should return true for system:masters group", func() {
			groups := []string{"system:masters", "other-group"}
			Expect(isAdminUser(groups)).To(BeTrue())
		})

		It("should return true for environment variable admin group", func() {
			Expect(os.Setenv("CLUSTER_ADMIN_GROUP", "custom-admin-group")).To(Succeed())
			defer func() { _ = os.Unsetenv("CLUSTER_ADMIN_GROUP") }()

			groups := []string{"custom-admin-group", "other-group"}
			Expect(isAdminUser(groups)).To(BeTrue())
		})

		It("should return false for non-admin groups", func() {
			groups := []string{"regular-user", "data-scientists"}
			Expect(isAdminUser(groups)).To(BeFalse())
		})
	})
})

// MockClient for testing
type MockClient struct {
	ServiceAccount *corev1.ServiceAccount
	GetError       error
}

func (m *MockClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if m.GetError != nil {
		return m.GetError
	}
	if sa, ok := obj.(*corev1.ServiceAccount); ok && m.ServiceAccount != nil {
		*sa = *m.ServiceAccount
	}
	return nil
}

func (m *MockClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return nil
}

func (m *MockClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return nil
}

func (m *MockClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return nil
}

func (m *MockClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return nil
}

func (m *MockClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return nil
}

func (m *MockClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	return nil
}

func (m *MockClient) Status() client.StatusWriter {
	return nil
}

func (m *MockClient) Scheme() *runtime.Scheme {
	return nil
}

func (m *MockClient) RESTMapper() meta.RESTMapper {
	return nil
}

func (m *MockClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{}, nil
}

func (m *MockClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	return true, nil
}

func (m *MockClient) SubResource(subResource string) client.SubResourceClient {
	return nil
}
