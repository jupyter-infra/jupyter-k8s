/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
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
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

	testTemplateName         = "test-template"
	testWorkspaceName        = "test-workspace"
	testDisplayName          = "Test"
	testWorkspaceDisplayName = "Test Workspace"
	testTemplateDisplayName  = "Test Template"
	testStrategyDisplayName  = "Test Strategy"
	testOwnershipPublic      = "Public"
	testOwnershipOwnerOnly   = "OwnerOnly"
	testStatusRunning        = "Running"

	testExistingKey    = "existing"
	testLabelValue     = "value"
	testLabelKeyEnv    = "env"
	testDataScience    = "data-science"
	testEnvDevelopment = "development"
	testEnvProduction  = "production"

	testNamespaceTeamA  = "team-a"
	testNamespaceTeamB  = "team-b"
	testSharedNamespace = "jupyter-k8s-shared"
	testOtherNamespace  = "other-ns"
	testNamespaceName   = "test-namespace"

	testSomeStrategy      = "some-strategy"
	testSomeTemplate      = "some-template"
	testSharedTemplate    = "shared-template"
	testWebAccessStrategy = "web-access"
	testTemplateNameTmpl  = "tmpl"

	testBinBash          = "/bin/bash"
	testBinSh            = "/bin/sh"
	testStartNotebookCmd = "start-notebook.sh"
	testEchoHiCmd        = "echo hi"
	testDataMountPath    = "/data"

	testImageBusybox    = "busybox:latest"
	testImageAlpine     = "alpine:latest"
	testImageTestLatest = "test:latest"
	testScipyNotebook   = "jupyter/scipy-notebook:latest"

	testEnvNameEnv    = "ENV"
	testEnvNameRegion = "REGION"
	testEnvValueProd  = "prod"

	testWorkspacePodName   = "workspace-pod"
	testServiceAccountName = "test-sa"
	testUser1              = "user1"
	testOriginalUser       = "original-user"
	testOwnerUser          = "owner-user"
	testOtherAnnotation    = "other-annotation"

	testInitContainerSetup    = "setup"
	testTemplateInitContainer = "template-init"
	testLabelKeyTeam          = "team"
	testOtherValue            = "other-value"
	testVolumeNameData        = "data"
	testPVCNameData           = "data-pvc"
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
				Name:      testWorkspaceName,
				Namespace: testDefaultNamespace,
			},
			Spec: workspacev1alpha1.WorkspaceSpec{
				DisplayName:   testWorkspaceDisplayName,
				Image:         testValidBaseNotebook,
				DesiredStatus: testStatusRunning,
				OwnershipType: testOwnershipPublic,
			},
		}

		mockClient := &MockClient{}
		defaulter = WorkspaceCustomDefaulter{
			templateDefaulter:       NewTemplateDefaulter(mockClient, ""),
			serviceAccountDefaulter: NewServiceAccountDefaulter(mockClient),
			templateGetter:          NewTemplateGetter(mockClient, ""),
			templateValidator:       NewTemplateValidator(mockClient, ""),
			accessStrategyValidator: NewAccessStrategyValidator(""),
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
			workspace.Annotations = map[string]string{controller.AnnotationCreatedBy: testOriginalUser}
			ctx = createUserContext(ctx, "UPDATE", "new-user")

			err := defaulter.Default(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(workspace.Annotations[controller.AnnotationCreatedBy]).To(Equal(testOriginalUser))
			Expect(workspace.Annotations[controller.AnnotationLastUpdatedBy]).To(Equal("new-user"))
		})

		It("should preserve existing annotations", func() {
			workspace.Annotations = map[string]string{
				"custom-annotation":            "custom-value",
				controller.AnnotationCreatedBy: testOriginalUser,
			}
			ctx = createUserContext(ctx, "UPDATE", "new-user")

			err := defaulter.Default(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(workspace.Annotations["custom-annotation"]).To(Equal("custom-value"))
			Expect(workspace.Annotations[controller.AnnotationCreatedBy]).To(Equal(testOriginalUser))
			Expect(workspace.Annotations[controller.AnnotationLastUpdatedBy]).To(Equal("new-user"))
		})

		It("should handle missing user info gracefully", func() {
			err := defaulter.Default(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			// Webhook initializes empty annotations map but doesn't add user annotations
			Expect(workspace.Annotations).To(Equal(map[string]string{}))
		})

		It("should default desiredStatus to Running when not set", func() {
			workspace.Spec.DesiredStatus = ""
			err := defaulter.Default(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(workspace.Spec.DesiredStatus).To(Equal(controller.DefaultDesiredStatus))
		})

		It("should not override desiredStatus when already set", func() {
			workspace.Spec.DesiredStatus = controller.DesiredStateStopped
			err := defaulter.Default(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(workspace.Spec.DesiredStatus).To(Equal(controller.DesiredStateStopped))
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
					DisplayName: testStrategyDisplayName,
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
					DisplayName: testStrategyDisplayName,
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
				controller.AnnotationCreatedBy: testOriginalUser,
			}
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Spec.OwnershipType = webhookconst.OwnershipTypeOwnerOnly
			newWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: testOriginalUser,
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
			newWorkspace.Spec.Image = testScipyNotebook

			warnings, err := validator.ValidateUpdate(userCtx, oldWorkspace, newWorkspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should allow OwnerOnly workspace update by owner", func() {
			userCtx := createUserContext(ctx, "UPDATE", testOwnerUser)

			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Spec.OwnershipType = webhookconst.OwnershipTypeOwnerOnly
			oldWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: testOwnerUser,
			}
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Spec.OwnershipType = webhookconst.OwnershipTypeOwnerOnly
			newWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: testOwnerUser,
			}

			warnings, err := validator.ValidateUpdate(userCtx, oldWorkspace, newWorkspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should allow OwnerOnly workspace deletion by owner", func() {
			userCtx := createUserContext(ctx, "DELETE", testOwnerUser)

			ownerOnlyWorkspace := workspace.DeepCopy()
			ownerOnlyWorkspace.Spec.OwnershipType = webhookconst.OwnershipTypeOwnerOnly
			ownerOnlyWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: testOwnerUser,
			}

			warnings, err := validator.ValidateDelete(userCtx, ownerOnlyWorkspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should reject update that changes created-by annotation", func() {
			userCtx := createUserContext(ctx, "UPDATE", "different-user")

			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: testOriginalUser,
			}
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "malicious-user",
			}

			warnings, err := validator.ValidateUpdate(userCtx, oldWorkspace, newWorkspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("annotation 'workspace.jupyter.org/created-by' is immutable"))
			Expect(warnings).To(BeEmpty())
		})

		It("should allow changing ownershipType from OwnerOnly to Public", func() {
			ownerCtx := createUserContext(ctx, "UPDATE", testOwnerUser)

			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Spec.OwnershipType = webhookconst.OwnershipTypeOwnerOnly
			oldWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: testOwnerUser,
			}
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Spec.OwnershipType = webhookconst.OwnershipTypePublic
			newWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: testOwnerUser,
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
				controller.AnnotationCreatedBy: testOriginalUser,
			}
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Spec.OwnershipType = webhookconst.OwnershipTypeOwnerOnly
			newWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: testOriginalUser,
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
				controller.AnnotationCreatedBy: testOriginalUser,
			}
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Spec.OwnershipType = webhookconst.OwnershipTypeOwnerOnly
			newWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: testOriginalUser,
			}

			warnings, err := validator.ValidateUpdate(adminCtx, oldWorkspace, newWorkspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should reject update that removes created-by annotation", func() {
			userCtx := createUserContext(ctx, "UPDATE", "different-user")

			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: testOriginalUser,
				testOtherAnnotation:            testOtherValue,
			}
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Annotations = map[string]string{
				testOtherAnnotation: testOtherValue,
			}

			warnings, err := validator.ValidateUpdate(userCtx, oldWorkspace, newWorkspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("annotation 'workspace.jupyter.org/created-by' cannot be removed"))
			Expect(warnings).To(BeEmpty())
		})

		It("should reject update that removes all annotations", func() {
			userCtx := createUserContext(ctx, "UPDATE", "different-user")

			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: testOriginalUser,
				testOtherAnnotation:            testOtherValue,
			}
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Annotations = nil

			warnings, err := validator.ValidateUpdate(userCtx, oldWorkspace, newWorkspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("annotation 'workspace.jupyter.org/created-by' cannot be removed"))
			Expect(warnings).To(BeEmpty())
		})

		It("should allow admin to modify created-by annotation", func() {
			adminCtx := createUserContext(ctx, "UPDATE", "admin-user", "system:masters")

			oldWorkspace := workspace.DeepCopy()
			oldWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: testOriginalUser,
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
				controller.AnnotationCreatedBy: testOriginalUser,
			}
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: "",
			}

			warnings, err := validator.ValidateUpdate(userCtx, oldWorkspace, newWorkspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("annotation 'workspace.jupyter.org/created-by' is immutable"))
			Expect(warnings).To(BeEmpty())
		})

		It("should validate workspace deletion successfully", func() {
			warnings, err := validator.ValidateDelete(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

	})

	Context("validateOwnershipPermission", func() {
		var ownerOnlyWorkspace *workspacev1alpha1.Workspace

		BeforeEach(func() {
			ownerOnlyWorkspace = workspace.DeepCopy()
			ownerOnlyWorkspace.Spec.OwnershipType = webhookconst.OwnershipTypeOwnerOnly
			ownerOnlyWorkspace.Annotations = map[string]string{
				controller.AnnotationCreatedBy: testOwnerUser,
			}
		})

		It("should allow owner access", func() {
			userInfo := &authenticationv1.UserInfo{Username: testOwnerUser}
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
					Name:      testTemplateName,
					Namespace: testDefaultNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					AllowedImages: []string{testValidBaseNotebook, testScipyNotebook},
					DefaultImage:  testValidBaseNotebook,
				},
			}
		})

		Context("validateImageAllowed", func() {
			It("should allow image in allowed list", func() {
				violation := validateImageAllowed(testValidBaseNotebook, template)
				Expect(violation).To(BeNil())
			})

			It("should reject image not in allowed list", func() {
				violation := validateImageAllowed("malicious/image:latest", template)
				Expect(violation).NotTo(BeNil())
				Expect(violation.Type).To(Equal(ViolationTypeImageNotAllowed))
				Expect(violation.Message).To(ContainSubstring("malicious/image:latest"))
				Expect(violation.Message).To(ContainSubstring(testTemplateName))
			})

			It("should use default image when allowed list is empty", func() {
				template.Spec.AllowedImages = []string{}
				violation := validateImageAllowed(testValidBaseNotebook, template)
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
				Expect(violation.Message).To(ContainSubstring(testTemplateName))
			})

			It("should reject storage above maximum", func() {
				violation := validateStorageSize(resource.MustParse("20Gi"), template)
				Expect(violation).NotTo(BeNil())
				Expect(violation.Type).To(Equal(ViolationTypeStorageExceeded))
				Expect(violation.Message).To(ContainSubstring("exceeds maximum"))
				Expect(violation.Message).To(ContainSubstring(testTemplateName))
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
					{Name: testVolumeNameData, PersistentVolumeClaimName: testPVCNameData, MountPath: testDataMountPath},
				}
				violation := validateSecondaryStorages(volumes, template)
				Expect(violation).To(BeNil())
			})

			It("should allow volumes when AllowSecondaryStorages is nil (default true)", func() {
				template.Spec.AllowSecondaryStorages = nil
				volumes := []workspacev1alpha1.VolumeSpec{
					{Name: testVolumeNameData, PersistentVolumeClaimName: testPVCNameData, MountPath: testDataMountPath},
				}
				violation := validateSecondaryStorages(volumes, template)
				Expect(violation).To(BeNil())
			})

			It("should reject volumes when AllowSecondaryStorages is false", func() {
				allowSecondaryStorages := false
				template.Spec.AllowSecondaryStorages = &allowSecondaryStorages
				volumes := []workspacev1alpha1.VolumeSpec{
					{Name: testVolumeNameData, PersistentVolumeClaimName: testPVCNameData, MountPath: testDataMountPath},
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
					MountPath: testDataMountPath,
				}
				storage2 := &workspacev1alpha1.StorageSpec{
					Size:      resource.MustParse("1Gi"),
					MountPath: testDataMountPath,
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
			Expect(os.Setenv(controller.ControllerPodNamespaceEnv, testDefaultNamespace)).To(Succeed())
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
			oldWorkspace.Labels = map[string]string{testLabelKeyEnv: testEnvValueProd}
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Labels = map[string]string{testLabelKeyEnv: testEnvValueProd, testLabelKeyTeam: testDataScience}

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
			oldWorkspace.Labels = map[string]string{testLabelKeyEnv: "dev"}
			newWorkspace := workspace.DeepCopy()
			newWorkspace.Labels = map[string]string{testLabelKeyEnv: testEnvValueProd}
			newWorkspace.Spec.Image = testScipyNotebook // Spec changed

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
				templateValidator: NewTemplateValidator(k8sClient, testDefaultNamespace),
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
				Namespace: testDefaultNamespace,
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

func (m *MockClient) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
	return nil
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

// Webhook-level coverage of the integration path: the WorkspaceCustomDefaulter stamping refs via the
// IntegrationRefDefaulter, and the WorkspaceCustomValidator invoking the IntegrationTemplateRefValidator
// on create/update. The IntegrationRefDefaulter and IntegrationTemplateRefValidator each have their own
// focused unit tests; these assert the two are correctly WIRED into the Workspace webhook handlers.
var _ = Describe("Workspace Webhook integration refs", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(workspacev1alpha1.AddToScheme(scheme)).To(Succeed())
		// corev1 is needed because Default() runs the service-account defaulter, which lists ServiceAccounts.
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
	})

	integrationTemplateIn := func(namespace string, params ...string) *workspacev1alpha1.WorkspaceIntegrationTemplate {
		declared := make([]workspacev1alpha1.IntegrationTemplateParameter, 0, len(params))
		for _, p := range params {
			declared = append(declared, workspacev1alpha1.IntegrationTemplateParameter{Name: p})
		}
		return &workspacev1alpha1.WorkspaceIntegrationTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: testIntegrationName, Namespace: namespace},
			Spec:       workspacev1alpha1.WorkspaceIntegrationTemplateSpec{Parameters: declared},
		}
	}

	// wsInTeamA builds a team-a workspace with a single integrationTemplateRef (name testIntegrationName).
	wsInTeamA := func(refNamespace string, params ...workspacev1alpha1.IntegrationParameter) *workspacev1alpha1.Workspace {
		return &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceName, Namespace: testNamespaceTeamA},
			Spec: workspacev1alpha1.WorkspaceSpec{
				Image:         testValidBaseNotebook,
				DesiredStatus: testStatusRunning,
				IntegrationTemplateRefs: []workspacev1alpha1.IntegrationTemplateRef{{
					Name:       testIntegrationName,
					Namespace:  refNamespace,
					Parameters: params,
				}},
			},
		}
	}

	newDefaulter := func(objs ...runtime.Object) WorkspaceCustomDefaulter {
		c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()
		return WorkspaceCustomDefaulter{
			templateDefaulter:       NewTemplateDefaulter(c, testSharedNamespace),
			serviceAccountDefaulter: NewServiceAccountDefaulter(c),
			templateGetter:          NewTemplateGetter(c, testSharedNamespace),
			templateValidator:       NewTemplateValidator(c, testSharedNamespace),
			accessStrategyValidator: NewAccessStrategyValidator(testSharedNamespace),
			integrationRefDefaulter: NewIntegrationRefDefaulter(c, testSharedNamespace),
			client:                  c,
		}
	}

	newValidator := func(objs ...runtime.Object) WorkspaceCustomValidator {
		c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()
		return WorkspaceCustomValidator{
			templateValidator:               NewTemplateValidator(c, testSharedNamespace),
			serviceAccountValidator:         NewServiceAccountValidator(c),
			volumeValidator:                 NewVolumeValidator(c),
			integrationTemplateRefValidator: NewIntegrationTemplateRefValidator(c, testSharedNamespace),
		}
	}

	Context("Defaulter stamps the integrationTemplateRef namespace", func() {
		It("stamps the workspace's own namespace when the template lives there", func() {
			ws := wsInTeamA("") // omitted namespace
			d := newDefaulter(integrationTemplateIn(testNamespaceTeamA))
			Expect(d.Default(createUserContext(ctx, "CREATE", "test-user"), ws)).To(Succeed())
			Expect(ws.Spec.IntegrationTemplateRefs[0].Namespace).To(Equal(testNamespaceTeamA))
		})

		It("falls back to and stamps the shared namespace when the template lives only there", func() {
			ws := wsInTeamA("")
			d := newDefaulter(integrationTemplateIn(testSharedNamespace))
			Expect(d.Default(createUserContext(ctx, "CREATE", "test-user"), ws)).To(Succeed())
			Expect(ws.Spec.IntegrationTemplateRefs[0].Namespace).To(Equal(testSharedNamespace))
		})

		It("leaves an explicit ref namespace untouched", func() {
			ws := wsInTeamA(testNamespaceTeamA)
			d := newDefaulter(integrationTemplateIn(testSharedNamespace)) // only shared has it; explicit must NOT be rewritten
			Expect(d.Default(createUserContext(ctx, "CREATE", "test-user"), ws)).To(Succeed())
			Expect(ws.Spec.IntegrationTemplateRefs[0].Namespace).To(Equal(testNamespaceTeamA))
		})

		It("leaves the ref unstamped when the template is absent everywhere (validator will reject)", func() {
			ws := wsInTeamA("")
			d := newDefaulter() // empty client
			Expect(d.Default(createUserContext(ctx, "CREATE", "test-user"), ws)).To(Succeed())
			Expect(ws.Spec.IntegrationTemplateRefs[0].Namespace).To(BeEmpty())
		})
	})

	Context("ValidateCreate wires in the integration ref validator", func() {
		It("admits a workspace whose ref resolves and supplies all parameters", func() {
			ws := wsInTeamA(testNamespaceTeamA, workspacev1alpha1.IntegrationParameter{Name: testRayClusterParam, Value: testClusterAValue})
			v := newValidator(integrationTemplateIn(testNamespaceTeamA, testRayClusterParam))
			_, err := v.ValidateCreate(ctx, ws)
			Expect(err).NotTo(HaveOccurred())
		})

		It("rejects a workspace whose ref targets another team's namespace (scope)", func() {
			ws := wsInTeamA("team-b")
			v := newValidator(integrationTemplateIn("team-b"))
			_, err := v.ValidateCreate(ctx, ws)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("team-b"))
		})

		It("rejects a workspace whose referenced template does not exist", func() {
			ws := wsInTeamA(testNamespaceTeamA)
			v := newValidator() // empty client -> not found
			_, err := v.ValidateCreate(ctx, ws)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("rejects a workspace missing a declared parameter", func() {
			ws := wsInTeamA(testNamespaceTeamA) // supplies nothing
			v := newValidator(integrationTemplateIn(testNamespaceTeamA, testRayClusterParam))
			_, err := v.ValidateCreate(ctx, ws)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(testRayClusterParam))
		})

		It("surfaces a non-blocking warning for an undeclared supplied parameter", func() {
			ws := wsInTeamA(testNamespaceTeamA,
				workspacev1alpha1.IntegrationParameter{Name: testRayClusterParam, Value: testClusterAValue},
				workspacev1alpha1.IntegrationParameter{Name: "rayClustrName", Value: "typo"},
			)
			v := newValidator(integrationTemplateIn(testNamespaceTeamA, testRayClusterParam))
			warnings, err := v.ValidateCreate(ctx, ws)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(ContainElement(ContainSubstring("rayClustrName")))
		})
	})

	Context("ValidateUpdate only re-validates when the refs change", func() {
		It("re-validates and rejects when a user changes the ref to a missing template", func() {
			old := wsInTeamA(testNamespaceTeamA, workspacev1alpha1.IntegrationParameter{Name: testRayClusterParam, Value: testClusterAValue})
			updated := old.DeepCopy()
			updated.Spec.IntegrationTemplateRefs[0].Name = "does-not-exist"
			v := newValidator(integrationTemplateIn(testNamespaceTeamA, testRayClusterParam))
			userCtx := createUserContext(ctx, "UPDATE", "different-user")
			_, err := v.ValidateUpdate(userCtx, old, updated)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("skips integration validation when the refs are unchanged (no wedge on metadata-only update)", func() {
			// Even though the template is ABSENT from the client, an unchanged-refs update must NOT be
			// rejected -- integrationRefsChanged is false, so the validator never fetches. This is the
			// wedge-avoidance contract: a controller finalizer/label update on a workspace whose template
			// was later deleted still passes.
			old := wsInTeamA(testNamespaceTeamA, workspacev1alpha1.IntegrationParameter{Name: testRayClusterParam, Value: testClusterAValue})
			updated := old.DeepCopy()
			updated.Labels = map[string]string{"touched": "true"} // metadata-only change; refs identical
			v := newValidator()                                   // empty client: a fetch WOULD fail with not-found
			userCtx := createUserContext(ctx, "UPDATE", "different-user")
			_, err := v.ValidateUpdate(userCtx, old, updated)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	It("accepts a detach (integrationTemplateRefs cleared to empty)", func() {
		// Clearing the refs is a valid transition (detach): the empty-parameter/missing-template checks
		// only run per-ref, so an empty ref list passes admission.
		old := wsInTeamA(testNamespaceTeamA, workspacev1alpha1.IntegrationParameter{Name: testRayClusterParam, Value: testClusterAValue})
		updated := old.DeepCopy()
		updated.Spec.IntegrationTemplateRefs = []workspacev1alpha1.IntegrationTemplateRef{}
		v := newValidator() // empty client is fine; nothing to fetch
		userCtx := createUserContext(ctx, "UPDATE", "different-user")
		var warnings admission.Warnings
		warnings, err := v.ValidateUpdate(userCtx, old, updated)
		Expect(err).NotTo(HaveOccurred())
		Expect(warnings).To(BeEmpty())
	})
})
