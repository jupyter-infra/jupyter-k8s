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

package extensionapi

import (
	"context"
	"errors"

	"github.com/go-logr/logr"
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("CheckWorkspaceConnectionPermission", func() {

	Context("CheckWorkspaceConnectionPermission", func() {
		var (
			k8sClient         client.Client
			mockSarClient     *MockSarClient
			server            *ExtensionServer
			logger            logr.Logger
			testNamespace     string
			testWorkspaceName string
			testUsername      string
			testGroups        []string
			testUID           string
		)

		BeforeEach(func() {
			// Create a fake k8s client for each test
			scheme := runtime.NewScheme()
			_ = workspacev1alpha1.AddToScheme(scheme)
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()

			// Create a new mock SAR client for each test
			mockSarClient = NewMockSarClient()

			// Create the server with the fake client
			logger = logr.Discard()
			server = &ExtensionServer{
				k8sClient: k8sClient,
				sarClient: mockSarClient,
				logger:    &logger,
			}

			// Set up test values
			testNamespace = "test-namespace1"
			testWorkspaceName = "test-workspace1"
			testUsername = "test-user1"
			testGroups = []string{"system:authenticated", "github:test-group1"}
			testUID = "test-uid"
		})

		AfterEach(func() {})

		It("Should return allowed=false, notFound=false, reason.include(RBAC) when Create(SAR) returns allowed=false", func() {
			mockSarClient.SetupDenied("No RBAC permission")

			response, err := server.CheckWorkspaceConnectionPermission(
				testNamespace, testWorkspaceName, testUsername, testGroups, "", nil, &logger,
			)

			Expect(response).NotTo(BeNil())
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Allowed).To(BeFalse())
			Expect(response.NotFound).To(BeFalse())
			Expect(response.Reason).To(ContainSubstring("RBAC"))
		})

		It("Should pass the full auth context to Create(SAR)", func() {
			mockSarClient.SetupDenied("No RBAC permission")

			response, err := server.CheckWorkspaceConnectionPermission(
				testNamespace, testWorkspaceName, testUsername, testGroups, testUID, nil, &logger,
			)

			Expect(response).NotTo(BeNil())
			Expect(err).NotTo(HaveOccurred())

			// Verify the resource attributes in the SAR
			Expect(mockSarClient.LastCreateParams).NotTo(BeNil())
			ressourceAttrs := mockSarClient.LastCreateParams.Spec.ResourceAttributes
			Expect(ressourceAttrs).NotTo(BeNil())
			Expect(ressourceAttrs.Namespace).To(Equal(testNamespace))
			Expect(ressourceAttrs.Verb).To(Equal("create"))
			Expect(ressourceAttrs.Group).To(Equal("connection.workspace.jupyter.org"))
			Expect(ressourceAttrs.Resource).To(Equal("workspaceconnections"))

			Expect(mockSarClient.LastCreateParams.Spec.User).To(Equal(testUsername))
			Expect(mockSarClient.LastCreateParams.Spec.Groups).To(Equal(testGroups))
			Expect(mockSarClient.LastCreateParams.Spec.UID).To(Equal(testUID))
		})

		It("Should not call Get(Workspace) when Create(SAR) returns allowed=false", func() {
			mockSarClient.SetupDenied("No RBAC permission")

			errorClient := &mockErrorClient{
				getError: errors.New("simulated client error"),
			}

			// Create a server that will raise an error if it calls the k8s client
			potentialErrorServer := &ExtensionServer{
				k8sClient: errorClient,
				sarClient: mockSarClient,
				logger:    &logger,
			}

			_, err := potentialErrorServer.CheckWorkspaceConnectionPermission(
				testNamespace, testWorkspaceName, testUsername, testGroups, testUID, nil, &logger,
			)

			Expect(err).NotTo(HaveOccurred())
		})

		It("Should return an error and not call Get(Workspace) if Create(SAR) returns an error", func() {
			mockSarClient.SetupError(errors.New("SAR error"))

			errorClient := &mockErrorClient{
				getError: errors.New("simulated client error"),
			}

			// Create a server that will raise an error if it calls the k8s client
			potentialErrorServer := &ExtensionServer{
				k8sClient: errorClient,
				sarClient: mockSarClient,
				logger:    &logger,
			}

			result, err := potentialErrorServer.CheckWorkspaceConnectionPermission(
				testNamespace, testWorkspaceName, testUsername, testGroups, testUID, nil, &logger,
			)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("SAR error"))
			Expect(result).To(BeNil())
		})

		It("Should return allowed=true, notFound=false, reason.include(public) when Get(Workspace) indicate public", func() {
			mockSarClient.SetupAllowed("Permitted by RBAC")

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

			response, err := server.CheckWorkspaceConnectionPermission(
				testNamespace, testWorkspaceName, testUsername, testGroups, testUID, nil, &logger,
			)

			Expect(response).NotTo(BeNil())
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Allowed).To(BeTrue())
			Expect(response.NotFound).To(BeFalse())
			Expect(response.Reason).To(ContainSubstring("public"))
		})

		It("Should return allowed=false, notFound=true, reason.include(not found) when Get(Workspace) indicate notFound", func() {
			mockSarClient.SetupAllowed("Permitted by RBAC")

			response, err := server.CheckWorkspaceConnectionPermission(
				testNamespace, testWorkspaceName, testUsername, testGroups, testUID, nil, &logger,
			)

			Expect(response).NotTo(BeNil())
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Allowed).To(BeFalse())
			Expect(response.NotFound).To(BeTrue())
			Expect(response.Reason).To(ContainSubstring("not found"))
		})

		It("Should return allowed=false, notFound=false, reason.include(private) when Get(Workspace) indicate private and not owner", func() {
			mockSarClient.SetupAllowed("Permitted by RBAC")

			// Create a test workspace
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkspaceName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						OwnerAnnotation: "test-user2",
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessType: "OwnerOnly",
				},
			}
			Expect(k8sClient.Create(context.Background(), workspace)).To(Succeed())

			response, err := server.CheckWorkspaceConnectionPermission(
				testNamespace, testWorkspaceName, testUsername, testGroups, testUID, nil, &logger,
			)

			Expect(response).NotTo(BeNil())
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Allowed).To(BeFalse())
			Expect(response.NotFound).To(BeFalse())
			Expect(response.Reason).To(ContainSubstring("private"))
		})

		It("Should return allowed=true, notFound=false, reason.include(private) when Get(Workspace) indicate private and owner", func() {
			mockSarClient.SetupAllowed("Permitted by RBAC")

			// Create a test workspace
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkspaceName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						OwnerAnnotation: "test-user1",
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessType: "OwnerOnly",
				},
			}
			Expect(k8sClient.Create(context.Background(), workspace)).To(Succeed())

			response, err := server.CheckWorkspaceConnectionPermission(
				testNamespace, testWorkspaceName, testUsername, testGroups, testUID, nil, &logger,
			)

			Expect(response).NotTo(BeNil())
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Allowed).To(BeTrue())
			Expect(response.NotFound).To(BeFalse())
			Expect(response.Reason).To(ContainSubstring("private"))
		})

		It("Should return an error if CheckWorkspaceAccess returns an error", func() {
			mockSarClient.SetupAllowed("Permitted by RBAC")
			errorClient := &mockErrorClient{
				getError: errors.New("get Workspace client error"),
			}

			// Create a server that will raise an error if it calls the k8s client
			errorServer := &ExtensionServer{
				k8sClient: errorClient,
				sarClient: mockSarClient,
				logger:    &logger,
			}

			response, err := errorServer.CheckWorkspaceConnectionPermission(
				testNamespace, testWorkspaceName, testUsername, testGroups, testUID, nil, &logger,
			)

			Expect(response).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("get Workspace client error"))
		})

		It("Should pass the Extra field to CheckRBACPermission", func() {
			// Create test extra data
			testExtra := map[string]authorizationv1.ExtraValue{
				"impersonation.kubernetes.io/uid": {"12345"},
				"system.authentication.provider":  {"oidc"},
				"oidc.example.com/groups":         {"developers", "testers"},
			}

			mockSarClient.SetupAllowed("Permitted by RBAC")

			// We'll use the mockErrorClient to ensure it doesn't proceed past the RBAC check
			// This way we can verify just the RBAC part with Extra
			errorClient := &mockErrorClient{
				getError: errors.New("get Workspace client error"),
			}

			// Create a server that will raise an error if it calls the k8s client
			errorServer := &ExtensionServer{
				k8sClient: errorClient,
				sarClient: mockSarClient,
				logger:    &logger,
			}

			// This will fail after the RBAC check since we're using an error client
			_, err := errorServer.CheckWorkspaceConnectionPermission(
				testNamespace, testWorkspaceName, testUsername, testGroups, testUID, testExtra, &logger,
			)

			// The test should fail with the workspace error, but we can verify the Extra was passed
			Expect(err).To(HaveOccurred())

			// Verify that the extra data was passed to the SAR
			Expect(mockSarClient.LastCreateParams).NotTo(BeNil())
			Expect(mockSarClient.LastCreateParams.Spec.Extra).To(Equal(testExtra))
		})
	})
})
