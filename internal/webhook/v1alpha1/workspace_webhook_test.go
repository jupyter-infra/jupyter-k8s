/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package v1alpha1

import (
	"context"
	"fmt"
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

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
	webhookconst "github.com/jupyter-infra/jupyter-k8s/internal/webhook"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
)

// Test constants
const (
	testInvalidImage      = "malicious/hacked:latest"
	testValidBaseNotebook = "jupyter/base-notebook:latest"
	testDefaultNamespace  = "default"
	testStrategyName      = "test-strategy"
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
				Namespace: testDefaultNamespace,
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
			templateDefaulter:       NewTemplateDefaulter(mockClient, ""),
			serviceAccountDefaulter: NewServiceAccountDefaulter(mockClient),
			templateGetter:          NewTemplateGetter(mockClient),
			client:                  mockClient, // Add client field for testing
		}
		validator = WorkspaceCustomValidator{
			templateValidator:       NewTemplateValidator(mockClient, ""),
			serviceAccountValidator: NewServiceAccountValidator(mockClient),
			volumeValidator:         NewVolumeValidator(mockClient),
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

		It("should call Get(AccessStrategy) and Update(AccessStrategy) with finalizer", func() {
			// Create a test workspace with AccessStrategy reference
			workspace.Spec.AccessStrategy = &workspacev1alpha1.AccessStrategyRef{
				Name:      testStrategyName,
				Namespace: testDefaultNamespace,
			}

			// Create a context with user info for CREATE operation
			userCtx := createUserContext(ctx, "CREATE", "test-user")

			// Create an access strategy for the test
			accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testStrategyName,
					Namespace: testDefaultNamespace,
					// No finalizers initially
				},
				Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
					DisplayName: "Test Strategy",
				},
			}

			// Create a tracking client to monitor calls
			getCalled := false
			updateCalled := false
			var updatedObj client.Object

			mockClient := &MockClientWithTracking{
				Client: &MockClient{},
				getFunc: func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if key.Name == testStrategyName && key.Namespace == testDefaultNamespace {
						getCalled = true
						accessStrategy.DeepCopyInto(obj.(*workspacev1alpha1.WorkspaceAccessStrategy))
						return nil
					}
					return nil
				},
				updateFunc: func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
					if as, ok := obj.(*workspacev1alpha1.WorkspaceAccessStrategy); ok && as.Name == testStrategyName {
						updateCalled = true
						accessStrategyCopy := as.DeepCopy()
						updatedObj = accessStrategyCopy
						return nil
					}
					return nil
				},
			}

			// Use our mock client in the defaulter
			defaulter.client = mockClient

			// Call the defaulter
			err := defaulter.Default(userCtx, workspace)

			// Verify no errors
			Expect(err).NotTo(HaveOccurred())

			// Verify that Get was called for AccessStrategy
			Expect(getCalled).To(BeTrue(), "Get should be called for AccessStrategy")

			// Verify that Update was called
			Expect(updateCalled).To(BeTrue(), "Update should be called to add finalizer")

			// Verify that finalizer was added
			updatedAccessStrategy, ok := updatedObj.(*workspacev1alpha1.WorkspaceAccessStrategy)
			Expect(ok).To(BeTrue())

			finalizerName := workspaceutil.AccessStrategyFinalizerName
			hasExpectedFinalizer := false
			for _, finalizer := range updatedAccessStrategy.Finalizers {
				if finalizer == finalizerName {
					hasExpectedFinalizer = true
					break
				}
			}
			Expect(hasExpectedFinalizer).To(BeTrue(), "Finalizer should be added to AccessStrategy")
		})

		It("should return an error if Update(AccessStrategy) fails", func() {
			// Create a test workspace with AccessStrategy reference
			workspace.Spec.AccessStrategy = &workspacev1alpha1.AccessStrategyRef{
				Name:      testStrategyName,
				Namespace: testDefaultNamespace,
			}

			// Create a context with user info for CREATE operation
			userCtx := createUserContext(ctx, "CREATE", "test-user")

			// Create an access strategy for the test
			accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testStrategyName,
					Namespace: testDefaultNamespace,
					// No finalizers initially
				},
				Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
					DisplayName: "Test Strategy",
				},
			}

			// Create a client that successfully gets the AccessStrategy but fails on update
			mockClient := &MockClientWithTracking{
				Client: &MockClient{},
				getFunc: func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if key.Name == testStrategyName && key.Namespace == testDefaultNamespace {
						accessStrategy.DeepCopyInto(obj.(*workspacev1alpha1.WorkspaceAccessStrategy))
						return nil
					}
					return nil
				},
				updateFunc: func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
					if as, ok := obj.(*workspacev1alpha1.WorkspaceAccessStrategy); ok && as.Name == testStrategyName {
						return fmt.Errorf("simulated update error")
					}
					return nil
				},
			}

			// Use our mock client in the defaulter
			defaulter.client = mockClient

			// Call the defaulter
			err := defaulter.Default(userCtx, workspace)

			// Verify that the error was returned
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("simulated update error"))
			Expect(err.Error()).To(ContainSubstring("failed to add finalizer to AccessStrategy"))
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
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-template",
					Namespace: "default",
				},
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
				Expect(violation.Type).To(Equal(ViolationTypeImageNotAllowed))
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
				Expect(violation.Type).To(Equal(ViolationTypeImageNotAllowed))
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
				Expect(violation.Type).To(Equal(ViolationTypeImageNotAllowed))
			})

			It("should enforce restrictions when AllowCustomImages is nil (default)", func() {
				template.Spec.AllowCustomImages = nil
				violation := validateImageAllowed("malicious/image:latest", template)
				Expect(violation).NotTo(BeNil())
				Expect(violation.Type).To(Equal(ViolationTypeImageNotAllowed))
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
				Expect(violation.Type).To(Equal(ViolationTypeStorageExceeded))
				Expect(violation.Message).To(ContainSubstring("below minimum"))
				Expect(violation.Message).To(ContainSubstring("test-template"))
			})

			It("should reject storage above maximum", func() {
				violation := validateStorageSize(resource.MustParse("20Gi"), template)
				Expect(violation).NotTo(BeNil())
				Expect(violation.Type).To(Equal(ViolationTypeStorageExceeded))
				Expect(violation.Message).To(ContainSubstring("exceeds maximum"))
				Expect(violation.Message).To(ContainSubstring("test-template"))
			})

			It("should allow any size when no storage config", func() {
				template.Spec.PrimaryStorage = nil
				violation := validateStorageSize(resource.MustParse("100Gi"), template)
				Expect(violation).To(BeNil())
			})
		})

		Context("validateSecondaryStorages", func() {
			It("should allow volumes when AllowSecondaryStorages is true", func() {
				allowSecondaryStorages := true
				template.Spec.AllowSecondaryStorages = &allowSecondaryStorages
				volumes := []workspacev1alpha1.VolumeSpec{
					{Name: "data", PersistentVolumeClaimName: "data-pvc", MountPath: "/data"},
				}
				violation := validateSecondaryStorages(volumes, template)
				Expect(violation).To(BeNil())
			})

			It("should allow volumes when AllowSecondaryStorages is nil (default true)", func() {
				template.Spec.AllowSecondaryStorages = nil
				volumes := []workspacev1alpha1.VolumeSpec{
					{Name: "data", PersistentVolumeClaimName: "data-pvc", MountPath: "/data"},
				}
				violation := validateSecondaryStorages(volumes, template)
				Expect(violation).To(BeNil())
			})

			It("should reject volumes when AllowSecondaryStorages is false", func() {
				allowSecondaryStorages := false
				template.Spec.AllowSecondaryStorages = &allowSecondaryStorages
				volumes := []workspacev1alpha1.VolumeSpec{
					{Name: "data", PersistentVolumeClaimName: "data-pvc", MountPath: "/data"},
				}
				violation := validateSecondaryStorages(volumes, template)
				Expect(violation).NotTo(BeNil())
				Expect(violation.Type).To(Equal(ViolationTypeSecondaryStorageNotAllowed))
				Expect(violation.Field).To(Equal("spec.volumes"))
			})

			It("should allow empty volumes when AllowSecondaryStorages is false", func() {
				allowSecondaryStorages := false
				template.Spec.AllowSecondaryStorages = &allowSecondaryStorages
				volumes := []workspacev1alpha1.VolumeSpec{}
				violation := validateSecondaryStorages(volumes, template)
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
					Resources: map[corev1.ResourceName]workspacev1alpha1.ResourceRange{
						corev1.ResourceCPU: {
							Min: resource.MustParse("100m"),
							Max: resource.MustParse("2"),
						},
						corev1.ResourceMemory: {
							Min: resource.MustParse("128Mi"),
							Max: resource.MustParse("4Gi"),
						},
						corev1.ResourceName("nvidia.com/gpu"): {
							Min: resource.MustParse("0"),
							Max: resource.MustParse("4"),
						},
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
				Expect(violations[0].Type).To(Equal(ViolationTypeResourceExceeded))
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
				Expect(violations[0].Type).To(Equal(ViolationTypeResourceExceeded))
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
				Expect(violations[0].Type).To(Equal(ViolationTypeResourceExceeded))
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
				Expect(violations[0].Type).To(Equal(ViolationTypeResourceExceeded))
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

			Context("GPU bounds validation (GAP-6)", func() {
				It("should allow GPU within bounds", func() {
					resources := corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("2"),
						},
					}
					violations := validateResourceBounds(resources, template)
					Expect(violations).To(BeEmpty())
				})

				It("should reject GPU below minimum", func() {
					template.Spec.ResourceBounds.Resources[corev1.ResourceName("nvidia.com/gpu")] = workspacev1alpha1.ResourceRange{
						Min: resource.MustParse("1"),
						Max: resource.MustParse("4"),
					}
					resources := corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("0"),
						},
					}
					violations := validateResourceBounds(resources, template)
					Expect(violations).To(HaveLen(1))
					Expect(violations[0].Type).To(Equal(ViolationTypeResourceExceeded))
					Expect(violations[0].Message).To(ContainSubstring("below minimum"))
					Expect(violations[0].Message).To(ContainSubstring("test-template"))
				})

				It("should reject GPU above maximum", func() {
					resources := corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("8"),
						},
					}
					violations := validateResourceBounds(resources, template)
					Expect(violations).To(HaveLen(1))
					Expect(violations[0].Type).To(Equal(ViolationTypeResourceExceeded))
					Expect(violations[0].Message).To(ContainSubstring("exceeds maximum"))
					Expect(violations[0].Message).To(ContainSubstring("test-template"))
				})

				It("should allow GPU when no GPU bounds specified", func() {
					delete(template.Spec.ResourceBounds.Resources, corev1.ResourceName("nvidia.com/gpu"))
					resources := corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("100"),
						},
					}
					violations := validateResourceBounds(resources, template)
					Expect(violations).To(BeEmpty())
				})

				It("should validate GPU bounds independently of CPU/Memory", func() {
					resources := corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:                    resource.MustParse("500m"), // Valid
							corev1.ResourceMemory:                 resource.MustParse("1Gi"),  // Valid
							corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("10"),   // Invalid - exceeds max
						},
					}
					violations := validateResourceBounds(resources, template)
					Expect(violations).To(HaveLen(1))
					Expect(violations[0].Field).To(Equal("spec.resources.requests.nvidia.com/gpu"))
					Expect(violations[0].Message).To(ContainSubstring("exceeds maximum"))
				})
			})

			Context("Multi-vendor GPU support", func() {
				BeforeEach(func() {
					template.Spec.ResourceBounds = &workspacev1alpha1.ResourceBounds{
						Resources: map[corev1.ResourceName]workspacev1alpha1.ResourceRange{
							corev1.ResourceName("amd.com/gpu"): {
								Min: resource.MustParse("0"),
								Max: resource.MustParse("2"),
							},
							corev1.ResourceName("intel.com/gpu"): {
								Min: resource.MustParse("0"),
								Max: resource.MustParse("1"),
							},
						},
					}
				})

				It("should allow AMD GPU within bounds", func() {
					resources := corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceName("amd.com/gpu"): resource.MustParse("1"),
						},
					}
					violations := validateResourceBounds(resources, template)
					Expect(violations).To(BeEmpty())
				})

				It("should reject AMD GPU above maximum", func() {
					resources := corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceName("amd.com/gpu"): resource.MustParse("3"),
						},
					}
					violations := validateResourceBounds(resources, template)
					Expect(violations).To(HaveLen(1))
					Expect(violations[0].Field).To(Equal("spec.resources.requests.amd.com/gpu"))
					Expect(violations[0].Message).To(ContainSubstring("exceeds maximum"))
				})

				It("should allow Intel GPU within bounds", func() {
					resources := corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceName("intel.com/gpu"): resource.MustParse("1"),
						},
					}
					violations := validateResourceBounds(resources, template)
					Expect(violations).To(BeEmpty())
				})

				It("should reject Intel GPU above maximum", func() {
					resources := corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceName("intel.com/gpu"): resource.MustParse("2"),
						},
					}
					violations := validateResourceBounds(resources, template)
					Expect(violations).To(HaveLen(1))
					Expect(violations[0].Field).To(Equal("spec.resources.requests.intel.com/gpu"))
					Expect(violations[0].Message).To(ContainSubstring("exceeds maximum"))
				})

				It("should validate multiple GPU types simultaneously", func() {
					template.Spec.ResourceBounds.Resources[corev1.ResourceName("nvidia.com/gpu")] = workspacev1alpha1.ResourceRange{
						Min: resource.MustParse("0"),
						Max: resource.MustParse("4"),
					}
					resources := corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("2"), // Valid
							corev1.ResourceName("amd.com/gpu"):    resource.MustParse("1"), // Valid
							corev1.ResourceName("intel.com/gpu"):  resource.MustParse("2"), // Invalid - exceeds max
						},
					}
					violations := validateResourceBounds(resources, template)
					Expect(violations).To(HaveLen(1))
					Expect(violations[0].Field).To(Equal("spec.resources.requests.intel.com/gpu"))
				})
			})

			Context("NVIDIA MIG profile support", func() {
				BeforeEach(func() {
					template.Spec.ResourceBounds = &workspacev1alpha1.ResourceBounds{
						Resources: map[corev1.ResourceName]workspacev1alpha1.ResourceRange{
							corev1.ResourceName("nvidia.com/mig-1g.5gb"): {
								Min: resource.MustParse("0"),
								Max: resource.MustParse("2"),
							},
							corev1.ResourceName("nvidia.com/mig-2g.10gb"): {
								Min: resource.MustParse("0"),
								Max: resource.MustParse("1"),
							},
						},
					}
				})

				It("should allow MIG 1g.5gb within bounds", func() {
					resources := corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceName("nvidia.com/mig-1g.5gb"): resource.MustParse("1"),
						},
					}
					violations := validateResourceBounds(resources, template)
					Expect(violations).To(BeEmpty())
				})

				It("should reject MIG 1g.5gb above maximum", func() {
					resources := corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceName("nvidia.com/mig-1g.5gb"): resource.MustParse("3"),
						},
					}
					violations := validateResourceBounds(resources, template)
					Expect(violations).To(HaveLen(1))
					Expect(violations[0].Message).To(ContainSubstring("exceeds maximum"))
				})

				It("should allow MIG 2g.10gb within bounds", func() {
					resources := corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceName("nvidia.com/mig-2g.10gb"): resource.MustParse("1"),
						},
					}
					violations := validateResourceBounds(resources, template)
					Expect(violations).To(BeEmpty())
				})

				It("should validate multiple MIG profiles independently", func() {
					resources := corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceName("nvidia.com/mig-1g.5gb"):  resource.MustParse("2"), // Valid
							corev1.ResourceName("nvidia.com/mig-2g.10gb"): resource.MustParse("2"), // Invalid
						},
					}
					violations := validateResourceBounds(resources, template)
					Expect(violations).To(HaveLen(1))
					Expect(violations[0].Field).To(Equal("spec.resources.requests.nvidia.com/mig-2g.10gb"))
				})
			})

			Context("Edge cases and advanced scenarios", func() {
				It("should allow unbounded resources (not in template bounds)", func() {
					template.Spec.ResourceBounds = &workspacev1alpha1.ResourceBounds{
						Resources: map[corev1.ResourceName]workspacev1alpha1.ResourceRange{
							corev1.ResourceCPU: {
								Min: resource.MustParse("100m"),
								Max: resource.MustParse("2"),
							},
						},
					}
					resources := corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:                    resource.MustParse("1"),     // Bounded - valid
							corev1.ResourceMemory:                 resource.MustParse("100Gi"), // Unbounded - allowed
							corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("100"),   // Unbounded - allowed
							corev1.ResourceName("custom.io/tpu"):  resource.MustParse("1000"),  // Unbounded - allowed
						},
					}
					violations := validateResourceBounds(resources, template)
					Expect(violations).To(BeEmpty())
				})

				It("should validate mixed standard and extended resources", func() {
					template.Spec.ResourceBounds = &workspacev1alpha1.ResourceBounds{
						Resources: map[corev1.ResourceName]workspacev1alpha1.ResourceRange{
							corev1.ResourceCPU: {
								Min: resource.MustParse("100m"),
								Max: resource.MustParse("2"),
							},
							corev1.ResourceMemory: {
								Min: resource.MustParse("128Mi"),
								Max: resource.MustParse("4Gi"),
							},
							corev1.ResourceName("nvidia.com/gpu"): {
								Min: resource.MustParse("0"),
								Max: resource.MustParse("2"),
							},
							corev1.ResourceName("custom.io/accelerator"): {
								Min: resource.MustParse("0"),
								Max: resource.MustParse("1"),
							},
						},
					}
					resources := corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:                           resource.MustParse("500m"),
							corev1.ResourceMemory:                        resource.MustParse("1Gi"),
							corev1.ResourceName("nvidia.com/gpu"):        resource.MustParse("1"),
							corev1.ResourceName("custom.io/accelerator"): resource.MustParse("1"),
						},
					}
					violations := validateResourceBounds(resources, template)
					Expect(violations).To(BeEmpty())
				})

				It("should report multiple violations for multiple resources", func() {
					template.Spec.ResourceBounds = &workspacev1alpha1.ResourceBounds{
						Resources: map[corev1.ResourceName]workspacev1alpha1.ResourceRange{
							corev1.ResourceCPU: {
								Min: resource.MustParse("100m"),
								Max: resource.MustParse("2"),
							},
							corev1.ResourceMemory: {
								Min: resource.MustParse("128Mi"),
								Max: resource.MustParse("4Gi"),
							},
							corev1.ResourceName("nvidia.com/gpu"): {
								Min: resource.MustParse("0"),
								Max: resource.MustParse("2"),
							},
						},
					}
					resources := corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:                    resource.MustParse("10"),   // Exceeds max
							corev1.ResourceMemory:                 resource.MustParse("50Mi"), // Below min
							corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("5"),    // Exceeds max
						},
					}
					violations := validateResourceBounds(resources, template)
					Expect(violations).To(HaveLen(3))
				})
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

	Context("isControllerOrAdminUser", func() {
		It("should return true for system:masters group", func() {
			adminCtx := createUserContext(ctx, "UPDATE", "admin-user", "system:masters")
			Expect(isControllerOrAdminUser(adminCtx)).To(BeTrue())
		})

		It("should return true for environment variable admin group", func() {
			Expect(os.Setenv("CLUSTER_ADMIN_GROUP", "custom-admin-group")).To(Succeed())
			defer func() { _ = os.Unsetenv("CLUSTER_ADMIN_GROUP") }()

			adminCtx := createUserContext(ctx, "UPDATE", "admin-user", "custom-admin-group")
			Expect(isControllerOrAdminUser(adminCtx)).To(BeTrue())
		})

		It("should return true for controller service account", func() {
			Expect(os.Setenv(controller.ControllerPodNamespaceEnv, "default")).To(Succeed())
			Expect(os.Setenv(controller.ControllerPodServiceAccountEnv, "controller")).To(Succeed())
			defer func() {
				_ = os.Unsetenv(controller.ControllerPodNamespaceEnv)
				_ = os.Unsetenv(controller.ControllerPodServiceAccountEnv)
			}()

			controllerCtx := createUserContext(ctx, "UPDATE", "system:serviceaccount:default:controller")
			Expect(isControllerOrAdminUser(controllerCtx)).To(BeTrue())
		})

		It("should return false for non-admin groups", func() {
			userCtx := createUserContext(ctx, "UPDATE", "regular-user", "regular-user", "data-scientists")
			Expect(isControllerOrAdminUser(userCtx)).To(BeFalse())
		})

		It("should return false when no request context", func() {
			Expect(isControllerOrAdminUser(ctx)).To(BeFalse())
		})
	})

	Context("Metadata-Only Updates (GAP-7)", func() {
		It("should skip validation for metadata-only updates (labels)", func() {
			userCtx := createUserContext(ctx, "UPDATE", "test-user")

			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Labels = map[string]string{"env": "prod"}
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Labels = map[string]string{"env": "prod", "team": "data-science"}

			// Both workspaces have identical spec - only labels changed
			warnings, err := validator.ValidateUpdate(userCtx, oldWorkspace, newWorkspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should skip validation for metadata-only updates (annotations)", func() {
			userCtx := createUserContext(ctx, "UPDATE", "test-user")

			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "test-user",
			}
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "test-user",
				"custom-annotation":            "custom-value",
			}

			// Both workspaces have identical spec - only annotations changed
			warnings, err := validator.ValidateUpdate(userCtx, oldWorkspace, newWorkspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should validate spec when both metadata and spec change", func() {
			userCtx := createUserContext(ctx, "UPDATE", "test-user")

			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Labels = map[string]string{"env": "dev"}
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Labels = map[string]string{"env": "prod"}
			newWorkspace.Spec.Image = "jupyter/scipy-notebook:latest" // Spec changed

			// Should validate because spec changed (not just metadata)
			warnings, err := validator.ValidateUpdate(userCtx, oldWorkspace, newWorkspace)
			// Validation should occur - if there's no template, it won't fail
			// but the validation path was exercised
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})
	})

	Context("Stopping Workspace Always Allowed", func() {
		var template *workspacev1alpha1.WorkspaceTemplate
		var validatorWithTemplate *WorkspaceCustomValidator

		BeforeEach(func() {
			// Create template with strict constraints
			minSize := resource.MustParse("1Gi")
			maxSize := resource.MustParse("100Gi")
			template = &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "strict-template",
					Namespace: testDefaultNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:   "Strict Template",
					DefaultImage:  testValidBaseNotebook,
					AllowedImages: []string{testValidBaseNotebook},
					ResourceBounds: &workspacev1alpha1.ResourceBounds{
						Resources: map[corev1.ResourceName]workspacev1alpha1.ResourceRange{
							corev1.ResourceCPU: {
								Min: resource.MustParse("100m"),
								Max: resource.MustParse("1"),
							},
						},
					},
					PrimaryStorage: &workspacev1alpha1.StorageConfig{
						MinSize: &minSize,
						MaxSize: &maxSize,
					},
				},
			}
			Expect(k8sClient.Create(ctx, template)).To(Succeed())

			// Create validator with template validator initialized
			validatorWithTemplate = &WorkspaceCustomValidator{
				templateValidator: NewTemplateValidator(k8sClient, "default"),
				volumeValidator:   NewVolumeValidator(k8sClient),
			}
		})

		AfterEach(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, template))).To(Succeed())
		})

		It("should reject stopping workspace when image also changes to invalid value", func() {
			userCtx := createUserContext(ctx, "UPDATE", "test-user")

			// Old workspace with valid image and Running status
			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Spec.Image = testValidBaseNotebook // Valid
			oldWorkspace.Spec.DesiredStatus = controller.DesiredStateRunning
			oldWorkspace.Spec.TemplateRef = &workspacev1alpha1.TemplateRef{
				Name:      template.Name,
				Namespace: template.Namespace,
			}

			// New workspace with invalid image AND stopping
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Spec.Image = testInvalidImage                       // Invalid - violates template
			newWorkspace.Spec.DesiredStatus = controller.DesiredStateStopped // Stopping
			newWorkspace.Spec.TemplateRef = &workspacev1alpha1.TemplateRef{
				Name:      template.Name,
				Namespace: "default",
			}

			// Should reject because image changed to invalid value (not just stopping)
			_, err := validatorWithTemplate.ValidateUpdate(userCtx, oldWorkspace, newWorkspace)
			Expect(err).To(HaveOccurred(), "stopping with invalid image change should be rejected")
			Expect(err.Error()).To(ContainSubstring("not allowed by template"))
		})

		It("should reject stopping workspace when resources also change to invalid value", func() {
			userCtx := createUserContext(ctx, "UPDATE", "test-user")

			// Old workspace with valid resources and Running status
			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Spec.Resources = &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("500m"), // Valid
				},
			}
			oldWorkspace.Spec.DesiredStatus = controller.DesiredStateRunning
			oldWorkspace.Spec.TemplateRef = &workspacev1alpha1.TemplateRef{
				Name:      template.Name,
				Namespace: template.Namespace,
			}

			// New workspace with invalid resources AND stopping
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Spec.Resources = &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("10"), // Invalid - exceeds max
				},
			}
			newWorkspace.Spec.DesiredStatus = controller.DesiredStateStopped // Stopping
			newWorkspace.Spec.TemplateRef = &workspacev1alpha1.TemplateRef{
				Name:      template.Name,
				Namespace: template.Namespace,
			}

			// Should reject because resources changed to invalid value (not just stopping)
			_, err := validatorWithTemplate.ValidateUpdate(userCtx, oldWorkspace, newWorkspace)
			Expect(err).To(HaveOccurred(), "stopping with invalid resource change should be rejected")
			Expect(err.Error()).To(ContainSubstring("exceeds maximum"))
		})

		It("should allow stopping workspace when only status changes (compliant workspace)", func() {
			userCtx := createUserContext(ctx, "UPDATE", "test-user")

			// Old workspace with valid configuration and Running status
			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Spec.Image = testValidBaseNotebook // Valid
			oldWorkspace.Spec.DesiredStatus = controller.DesiredStateRunning
			oldWorkspace.Spec.TemplateRef = &workspacev1alpha1.TemplateRef{
				Name:      template.Name,
				Namespace: template.Namespace,
			}

			// New workspace - ONLY changing status to Stopped
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Spec.Image = testValidBaseNotebook                  // Same valid image
			newWorkspace.Spec.DesiredStatus = controller.DesiredStateStopped // Stopping (only change)
			newWorkspace.Spec.TemplateRef = &workspacev1alpha1.TemplateRef{
				Name:      template.Name,
				Namespace: template.Namespace,
			}

			// Should allow stop without validation when only status changes
			warnings, err := validatorWithTemplate.ValidateUpdate(userCtx, oldWorkspace, newWorkspace)
			Expect(err).NotTo(HaveOccurred(), "stopping with status-only change should be allowed")
			Expect(warnings).To(BeEmpty())
		})

		It("should allow stopping workspace that is already non-compliant (status-only change)", func() {
			userCtx := createUserContext(ctx, "UPDATE", "test-user")

			// Old workspace is already non-compliant (e.g., template was updated after workspace creation)
			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Spec.Image = testInvalidImage // Invalid
			oldWorkspace.Spec.DesiredStatus = controller.DesiredStateRunning
			oldWorkspace.Spec.TemplateRef = &workspacev1alpha1.TemplateRef{
				Name:      template.Name,
				Namespace: template.Namespace,
			}

			// New workspace - ONLY changing status to Stopped, keeping same invalid image
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Spec.Image = testInvalidImage                       // Still invalid (unchanged)
			newWorkspace.Spec.DesiredStatus = controller.DesiredStateStopped // Stopping (only change)
			newWorkspace.Spec.TemplateRef = &workspacev1alpha1.TemplateRef{
				Name:      template.Name,
				Namespace: template.Namespace,
			}

			// Should allow stop because ONLY DesiredStatus changed (image stayed the same)
			warnings, err := validatorWithTemplate.ValidateUpdate(userCtx, oldWorkspace, newWorkspace)
			Expect(err).NotTo(HaveOccurred(), "stopping non-compliant workspace should be allowed when only status changes")
			Expect(warnings).To(BeEmpty())
		})

		It("should reject stopping workspace when storage also changes to invalid value", func() {
			userCtx := createUserContext(ctx, "UPDATE", "test-user")

			// Old workspace with valid storage and Running status
			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Spec.Storage = &workspacev1alpha1.StorageSpec{
				Size: resource.MustParse("10Gi"), // Valid (within 1Gi-100Gi bounds)
			}
			oldWorkspace.Spec.DesiredStatus = controller.DesiredStateRunning
			oldWorkspace.Spec.TemplateRef = &workspacev1alpha1.TemplateRef{
				Name:      template.Name,
				Namespace: template.Namespace,
			}

			// New workspace with invalid storage AND stopping
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Spec.Storage = &workspacev1alpha1.StorageSpec{
				Size: resource.MustParse("500Gi"), // Invalid - exceeds max (100Gi)
			}
			newWorkspace.Spec.DesiredStatus = controller.DesiredStateStopped // Stopping
			newWorkspace.Spec.TemplateRef = &workspacev1alpha1.TemplateRef{
				Name:      template.Name,
				Namespace: template.Namespace,
			}

			// Should reject because storage changed to invalid value (not just stopping)
			_, err := validatorWithTemplate.ValidateUpdate(userCtx, oldWorkspace, newWorkspace)
			Expect(err).To(HaveOccurred(), "stopping with invalid storage change should be rejected")
			Expect(err.Error()).To(ContainSubstring("exceeds maximum"))
		})

		It("should reject non-stop updates with invalid image", func() {
			userCtx := createUserContext(ctx, "UPDATE", "test-user")

			// Old workspace with valid image
			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Spec.Image = testValidBaseNotebook // Valid
			oldWorkspace.Spec.DesiredStatus = controller.DesiredStateRunning
			oldWorkspace.Spec.TemplateRef = &workspacev1alpha1.TemplateRef{
				Name:      template.Name,
				Namespace: template.Namespace,
			}

			// New workspace with invalid image and NOT stopping
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Spec.Image = testInvalidImage                       // Invalid
			newWorkspace.Spec.DesiredStatus = controller.DesiredStateRunning // Still Running
			newWorkspace.Spec.TemplateRef = &workspacev1alpha1.TemplateRef{
				Name:      template.Name,
				Namespace: template.Namespace,
			}

			// Should reject because not stopping
			_, err := validatorWithTemplate.ValidateUpdate(userCtx, oldWorkspace, newWorkspace)
			Expect(err).To(HaveOccurred(), "non-stop update with invalid image should be rejected")
			Expect(err.Error()).To(ContainSubstring("not allowed by template"))
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

// MockClientWithTracking allows tracking of specific client operations for testing
type MockClientWithTracking struct {
	client.Client
	getFunc    func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error
	updateFunc func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error
	listFunc   func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error
	createFunc func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error
	deleteFunc func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error
}

// Get overrides the Client Get method to track calls
func (m *MockClientWithTracking) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if m.getFunc != nil {
		return m.getFunc(ctx, key, obj, opts...)
	}
	return m.Client.Get(ctx, key, obj, opts...)
}

// Update overrides the Client Update method to track calls
func (m *MockClientWithTracking) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, obj, opts...)
	}
	return m.Client.Update(ctx, obj, opts...)
}

// List overrides the Client List method to track calls
func (m *MockClientWithTracking) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if m.listFunc != nil {
		return m.listFunc(ctx, list, opts...)
	}
	return m.Client.List(ctx, list, opts...)
}

// Create overrides the Client Create method to track calls
func (m *MockClientWithTracking) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if m.createFunc != nil {
		return m.createFunc(ctx, obj, opts...)
	}
	return m.Client.Create(ctx, obj, opts...)
}

// Delete overrides the Client Delete method to track calls
func (m *MockClientWithTracking) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, obj, opts...)
	}
	return m.Client.Delete(ctx, obj, opts...)
}
