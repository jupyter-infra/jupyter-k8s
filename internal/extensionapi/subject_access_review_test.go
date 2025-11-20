/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package extensionapi

import (
	"errors"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authorizationv1 "k8s.io/api/authorization/v1"
)

var _ = Describe("SubjectAccessReview", func() {
	Context("CheckRBACPermission", func() {
		var (
			server     *ExtensionServer
			mockClient *MockSarClient
			testLogger logr.Logger
			testNS     string
			testUser   string
			testGroups []string
			testUID    string
		)

		BeforeEach(func() {
			// Create a new mock SAR client for each test
			mockClient = NewMockSarClient()

			// Create a server instance with our mock client
			server = &ExtensionServer{
				sarClient: mockClient,
			}

			// Set up test data
			testLogger = logr.Discard()
			testNS = "test-namespace"
			testUser = "test-user"
			testGroups = []string{"system:authenticated", "test-group"}
			testUID = "test-uid"
		})

		It("Should use the server SAR client to submit a SAR", func() {
			// Call the function under test
			_, err := server.CheckRBACPermission(testNS, testUser, testGroups, testUID, nil, &testLogger)

			// Verify no error
			Expect(err).NotTo(HaveOccurred())

			// Verify that the SAR client was called
			Expect(mockClient.CreateCallCount).To(Equal(1), "Create should be called exactly once")
		})

		It("Should pass the username and groups to the SAR", func() {
			// Call the function under test
			_, err := server.CheckRBACPermission(testNS, testUser, testGroups, testUID, nil, &testLogger)

			// Verify no error
			Expect(err).NotTo(HaveOccurred())

			// Verify that the username and groups were passed to the SAR
			Expect(mockClient.LastCreateParams).NotTo(BeNil())
			Expect(mockClient.LastCreateParams.Spec.User).To(Equal(testUser))
			Expect(mockClient.LastCreateParams.Spec.Groups).To(Equal(testGroups))
			Expect(mockClient.LastCreateParams.Spec.UID).To(Equal(testUID))
		})

		It("Should include the correct resource attributes in the SAR", func() {
			// Call the function under test
			_, err := server.CheckRBACPermission(testNS, testUser, testGroups, testUID, nil, &testLogger)

			// Verify no error
			Expect(err).NotTo(HaveOccurred())

			// Verify the resource attributes in the SAR
			Expect(mockClient.LastCreateParams).NotTo(BeNil())
			attrs := mockClient.LastCreateParams.Spec.ResourceAttributes
			Expect(attrs).NotTo(BeNil())
			Expect(attrs.Namespace).To(Equal(testNS))
			Expect(attrs.Verb).To(Equal("create"))
			Expect(attrs.Group).To(Equal("connection.workspace.jupyter.org"))
			Expect(attrs.Resource).To(Equal("workspaceconnections"))
		})

		It("Should return allowed=true and the SAR reason if the SAR returns allowed", func() {
			// Configure mock to return allowed
			mockClient.SetupAllowed("test reason allowed")

			// Call the function under test
			result, err := server.CheckRBACPermission(testNS, testUser, testGroups, testUID, nil, &testLogger)

			// Verify no error
			Expect(err).NotTo(HaveOccurred())

			// Verify the result
			Expect(result).NotTo(BeNil())
			Expect(result.Allowed).To(BeTrue())
			Expect(result.Reason).To(Equal("test reason allowed"))
		})

		It("Should return allowed=false and the SAR reason if the SAR returns disallowed", func() {
			// Configure mock to return disallowed
			mockClient.SetupDenied("test reason denied")

			// Call the function under test
			result, err := server.CheckRBACPermission(testNS, testUser, testGroups, testUID, nil, &testLogger)

			// Verify no error
			Expect(err).NotTo(HaveOccurred())

			// Verify the result
			Expect(result).NotTo(BeNil())
			Expect(result.Allowed).To(BeFalse())
			Expect(result.Reason).To(Equal("test reason denied"))
		})

		It("Should return nil and an error if the SAR fails", func() {
			// Configure mock to return an error
			mockErr := errors.New("SAR creation error")
			mockClient.SetupError(mockErr)

			// Call the function under test
			result, err := server.CheckRBACPermission(testNS, testUser, testGroups, testUID, nil, &testLogger)

			// Verify error is returned
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create SubjectAccessReview"))
			Expect(err.Error()).To(ContainSubstring(mockErr.Error()))

			// Verify the result is nil
			Expect(result).To(BeNil())
		})

		It("Should properly handle different namespace values", func() {
			// Test with different namespaces
			namespaces := []string{"default", "kube-system", "test-workspace-123"}

			for _, ns := range namespaces {
				// Reset the mock
				mockClient = NewMockSarClient()
				server.sarClient = mockClient

				// Call the function under test with this namespace
				_, err := server.CheckRBACPermission(ns, testUser, testGroups, testUID, nil, &testLogger)

				// Verify no error
				Expect(err).NotTo(HaveOccurred())

				// Verify that the namespace was passed correctly
				Expect(mockClient.LastCreateParams).NotTo(BeNil())
				Expect(mockClient.LastCreateParams.Spec.ResourceAttributes.Namespace).To(Equal(ns))
			}
		})

		It("Should pass extra auth data to the SAR", func() {
			// Create test extra data
			testExtra := map[string]authorizationv1.ExtraValue{
				"impersonation.kubernetes.io/uid": {"12345"},
				"system.authentication.provider":  {"oidc"},
				"oidc.example.com/groups":         {"developers", "testers"},
			}

			// Call the function under test with extra data
			_, err := server.CheckRBACPermission(testNS, testUser, testGroups, testUID, testExtra, &testLogger)

			// Verify no error
			Expect(err).NotTo(HaveOccurred())

			// Verify that the extra data was passed to the SAR
			Expect(mockClient.LastCreateParams).NotTo(BeNil())
			Expect(mockClient.LastCreateParams.Spec.Extra).To(Equal(testExtra))
		})
	})
})
