package extensionapi

import (
	"context"
	"errors"

	"github.com/go-logr/logr"
	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("WorkspaceAdmission", func() {

	Context("CheckWorkspaceAccess", func() {
		var (
			k8sClient         client.Client
			server            *ExtensionServer
			logger            logr.Logger
			testNamespace     string
			testWorkspaceName string
			testUsername      string
		)

		BeforeEach(func() {
			scheme := runtime.NewScheme()
			_ = workspacev1alpha1.AddToScheme(scheme)
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()

			// Create the server with the fake client
			logger = logr.Discard()
			server = &ExtensionServer{
				k8sClient: k8sClient,
				logger:    &logger,
			}

			// Set up test values
			testNamespace = "test-namespace"
			testWorkspaceName = "test-workspace"
			testUsername = "test-user"
		})

		It("Should use the client to Get the Workspace in the namespace", func() {
			// Create a test workspace
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkspaceName,
					Namespace: testNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessType: "Public",
				},
			}
			Expect(k8sClient.Create(context.Background(), workspace)).To(Succeed())

			// Call the function under test
			result, err := server.CheckWorkspaceAccess(testNamespace, testWorkspaceName, testUsername, &logger)

			// Check expectations
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			// The real assertion here is that we successfully called the client to get the workspace
			// which is verified by the fact that we got a successful result
			Expect(result.NotFound).To(BeFalse())
		})

		It("Should return allowed=false, notFound=true if Workspace cannot be found", func() {
			// Call with non-existent workspace
			result, err := server.CheckWorkspaceAccess(testNamespace, "non-existent-workspace", testUsername, &logger)

			// Check expectations
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Allowed).To(BeFalse())
			Expect(result.NotFound).To(BeTrue())
			Expect(result.Reason).To(ContainSubstring("not found"))
		})

		It("Should return allowed=true, notFound=false if Workspace exists and is public", func() {
			// Create a public workspace
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkspaceName,
					Namespace: testNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessType: "Public",
				},
			}
			Expect(k8sClient.Create(context.Background(), workspace)).To(Succeed())

			// Call the function
			result, err := server.CheckWorkspaceAccess(testNamespace, testWorkspaceName, testUsername, &logger)

			// Check expectations
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Allowed).To(BeTrue())
			Expect(result.NotFound).To(BeFalse())
			Expect(result.Reason).To(ContainSubstring("public"))
			Expect(result.AccessType).To(Equal(AccessTypePublic))
		})

		It("Should return allowed=false, notFound=false if Workspace exists, is private, annotation does not match caller username", func() {
			// Create a private workspace with a different owner
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkspaceName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						OwnerAnnotation: "different-user",
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessType: "OwnerOnly", // Private
				},
			}
			Expect(k8sClient.Create(context.Background(), workspace)).To(Succeed())

			// Call the function
			result, err := server.CheckWorkspaceAccess(testNamespace, testWorkspaceName, testUsername, &logger)

			// Check expectations
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Allowed).To(BeFalse())
			Expect(result.NotFound).To(BeFalse())
			Expect(result.Reason).To(ContainSubstring("not the workspace owner"))
			Expect(result.AccessType).To(Equal(AccessTypePrivate))
			Expect(result.OwnerUsername).To(Equal("different-user"))
		})

		It("Should return allowed=true, notFound=false if Workspace exists, is private, matches caller username", func() {
			// Create a private workspace owned by the caller
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkspaceName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						OwnerAnnotation: testUsername,
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessType: "OwnerOnly", // Private
				},
			}
			Expect(k8sClient.Create(context.Background(), workspace)).To(Succeed())

			// Call the function
			result, err := server.CheckWorkspaceAccess(testNamespace, testWorkspaceName, testUsername, &logger)

			// Check expectations
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Allowed).To(BeTrue())
			Expect(result.NotFound).To(BeFalse())
			Expect(result.Reason).To(ContainSubstring("workspace owner"))
			Expect(result.AccessType).To(Equal(AccessTypePrivate))
			Expect(result.OwnerUsername).To(Equal(testUsername))
		})

		It("Should return an error if the k8s client fails", func() {
			// Create a fake client that returns errors
			errorClient := &mockErrorClient{
				getError: errors.New("simulated client error"),
			}

			// Create a server with the error client
			errorServer := &ExtensionServer{
				k8sClient: errorClient,
				logger:    &logger,
			}

			// Call the function
			result, err := errorServer.CheckWorkspaceAccess(testNamespace, testWorkspaceName, testUsername, &logger)

			// Check expectations
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("simulated client error"))
			Expect(result).To(BeNil())
		})
	})

	Context("getWorkspaceAccessType", func() {
		It("Should return private if Workspace.Spec.AccessType=Private", func() {
			workspace := &workspacev1alpha1.Workspace{
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessType: "Private",
				},
			}

			result := getWorkspaceAccessType(workspace)
			Expect(result).To(Equal(AccessTypePrivate))
		})

		It("Should return public if Workspace.Spec.AccessType=Public", func() {
			workspace := &workspacev1alpha1.Workspace{
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessType: "Public",
				},
			}

			result := getWorkspaceAccessType(workspace)
			Expect(result).To(Equal(AccessTypePublic))
		})

		It("Should return private if Workspace.Spec.AccessType=OwnerOnly", func() {
			workspace := &workspacev1alpha1.Workspace{
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessType: "OwnerOnly",
				},
			}

			result := getWorkspaceAccessType(workspace)
			Expect(result).To(Equal(AccessTypePrivate))
		})

		It("Should return private if Workspace.Spec.AccessType=SomethingElse", func() {
			workspace := &workspacev1alpha1.Workspace{
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessType: "SomethingElse",
				},
			}

			result := getWorkspaceAccessType(workspace)
			Expect(result).To(Equal(AccessTypePrivate))
		})

		It("Should return public if Workspace.Spec.AccessType is missing", func() {
			workspace := &workspacev1alpha1.Workspace{
				Spec: workspacev1alpha1.WorkspaceSpec{},
			}

			result := getWorkspaceAccessType(workspace)
			Expect(result).To(Equal(AccessTypePublic))
		})
	})

	Context("getWorkspaceOwner", func() {
		It("Should return the value of the created-by annotation when present", func() {
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						OwnerAnnotation: "test-owner",
					},
				},
			}

			result := getWorkspaceOwner(workspace)
			Expect(result).To(Equal("test-owner"))
		})

		It("Should return the empty string if created-by annotation does not exist", func() {
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"some-other-annotation": "value",
					},
				},
			}

			result := getWorkspaceOwner(workspace)
			Expect(result).To(BeEmpty())
		})

		It("Should return the empty string if there are no annotations", func() {
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{},
			}

			result := getWorkspaceOwner(workspace)
			Expect(result).To(BeEmpty())
		})

		It("Should return the empty string if workspace is nil", func() {
			result := getWorkspaceOwner(nil)
			Expect(result).To(BeEmpty())
		})
	})
})

// mockErrorClient is a mock client that always returns errors for testing error paths
type mockErrorClient struct {
	client.Client
	getError error
}

// Get implements client.Client
func (m *mockErrorClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return m.getError
}

// GroupVersionKindFor implements client.Client
func (m *mockErrorClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{}, nil
}

// IsObjectNamespaced implements client.Client
func (m *mockErrorClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	return true, nil
}
