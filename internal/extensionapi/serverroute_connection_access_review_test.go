package extensionapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/go-logr/logr"
	connectionv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/connection/v1alpha1"
	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// errorReader is a mock io.ReadCloser that always returns an error
type errorReader struct{}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("simulated read error")
}

func (e *errorReader) Close() error {
	return nil
}

// createTestRequest creates a test HTTP request with the specified method and body
// The path is fixed to the connection access review endpoint
func createTestRequest(method, path, body string) *http.Request { //nolint:unparam
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req = req.WithContext(context.TODO())
	return req
}

var _ = Describe("ServerRouteConnectionAccessReview", func() {
	Context("HandleConnectionAccessReview", func() {

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
			testUID = "test-uid1"
		})

		It("Should accept a POST request", func() {
			req := createTestRequest("POST", "/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/test-namespace1/connectionaccessreview", `{
				"spec": {
					"workspaceName": "test-workspace1",
					"user": "test-user1",
					"groups": ["system:authenticated"]
				}
			}`)
			recorder := httptest.NewRecorder()

			mockSarClient.SetupAllowed("Permitted by RBAC")

			// Create a test workspace
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkspaceName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						OwnerAnnotation: testUsername,
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessType: "OwnerOnly",
				},
			}
			Expect(k8sClient.Create(context.Background(), workspace)).To(Succeed())

			server.handleConnectionAccessReview(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusOK))
		})
		It("Should respond with 200", func() {
			req := createTestRequest("POST", "/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/test-namespace1/connectionaccessreview", `{
				"spec": {
					"workspaceName": "test-workspace1",
					"user": "test-user1",
					"groups": ["system:authenticated"]
				}
			}`)
			recorder := httptest.NewRecorder()

			mockSarClient.SetupAllowed("Permitted by RBAC")

			// Create a test workspace
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkspaceName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						OwnerAnnotation: testUsername,
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessType: "OwnerOnly",
				},
			}
			Expect(k8sClient.Create(context.Background(), workspace)).To(Succeed())

			server.handleConnectionAccessReview(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusOK))
		})
		It("Should reject any other HTTP methods", func() {
			methods := []string{"GET", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}

			for _, method := range methods {
				req := createTestRequest(method, "/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/test-namespace1/connectionaccessreview", "")
				recorder := httptest.NewRecorder()

				server.handleConnectionAccessReview(recorder, req)

				Expect(recorder.Code).To(Equal(http.StatusBadRequest))
				var response map[string]string
				err := json.NewDecoder(recorder.Body).Decode(&response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response).To(HaveKeyWithValue("error", "ConnectionAccessReview must use POST method"))
			}
		})

		It("Should read the request.body", func() {
			req := createTestRequest("POST", "/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/test-namespace1/connectionaccessreview", `{
				"spec": {
					"workspaceName": "test-workspace1",
					"user": "test-user1",
					"groups": ["system:authenticated", "github:test-group1"],
					"uid": "test-uid1"
				}
			}`)
			recorder := httptest.NewRecorder()

			mockSarClient.SetupAllowed("Permitted by RBAC")

			// Create a test workspace
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkspaceName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						OwnerAnnotation: testUsername,
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessType: "OwnerOnly",
				},
			}
			Expect(k8sClient.Create(context.Background(), workspace)).To(Succeed())

			server.handleConnectionAccessReview(recorder, req)

			// Verify that the SAR client was called with the correct parameters
			Expect(mockSarClient.LastCreateParams).NotTo(BeNil())
			Expect(mockSarClient.LastCreateParams.Spec.User).To(Equal(testUsername))
			Expect(mockSarClient.LastCreateParams.Spec.Groups).To(Equal(testGroups))
			Expect(mockSarClient.LastCreateParams.Spec.UID).To(Equal(testUID))
		})

		It("Should pass Extra from the request to SubjectAccessReview", func() {
			// Extra data is directly in the JSON request

			// Create a request with Extra in the spec
			req := createTestRequest("POST", "/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/test-namespace1/connectionaccessreview", `{
				"spec": {
					"workspaceName": "test-workspace1",
					"user": "test-user1",
					"groups": ["system:authenticated", "github:test-group1"],
					"extra": {
						"impersonation.kubernetes.io/uid": ["12345"],
						"system.authentication.provider": ["oidc"],
						"oidc.example.com/groups": ["developers", "testers"]
					}
				}
			}`)
			recorder := httptest.NewRecorder()

			mockSarClient.SetupAllowed("Permitted by RBAC")

			// Create a test workspace
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkspaceName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						OwnerAnnotation: testUsername,
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessType: "OwnerOnly",
				},
			}
			Expect(k8sClient.Create(context.Background(), workspace)).To(Succeed())

			server.handleConnectionAccessReview(recorder, req)

			// Verify that the SAR client was called with the Extra data
			Expect(mockSarClient.LastCreateParams).NotTo(BeNil())

			// Check that the Extra map exists and has the UID field
			Expect(mockSarClient.LastCreateParams.Spec.Extra).To(HaveKey("impersonation.kubernetes.io/uid"))

			// Check that the UID value is correct
			uidValues := mockSarClient.LastCreateParams.Spec.Extra["impersonation.kubernetes.io/uid"]
			Expect(uidValues).To(HaveLen(1))
			Expect(uidValues[0]).To(Equal("12345"))

			// Check the other fields
			providerValues := mockSarClient.LastCreateParams.Spec.Extra["system.authentication.provider"]
			Expect(providerValues).To(HaveLen(1))
			Expect(providerValues[0]).To(Equal("oidc"))

			groupValues := mockSarClient.LastCreateParams.Spec.Extra["oidc.example.com/groups"]
			Expect(groupValues).To(HaveLen(2))
			Expect(groupValues).To(ContainElement("developers"))
			Expect(groupValues).To(ContainElement("testers"))
		})
		It("Should return an error if reading the request.body fails", func() {
			req := createTestRequest("POST", "/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/test-namespace1/connectionaccessreview", "")

			// Replace the request body with an error reader
			req.Body = &errorReader{}

			recorder := httptest.NewRecorder()
			server.handleConnectionAccessReview(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusBadRequest))
			var response map[string]string
			err := json.NewDecoder(recorder.Body).Decode(&response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response).To(HaveKeyWithValue("error", "Failed to read request body"))
		})

		It("Should respond with allowed=true, notFound=false, set a reason, when Create(SAR) and Get(Workspace) succeed and owner matches caller", func() {
			req := createTestRequest("POST", "/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/test-namespace1/connectionaccessreview", `{
				"spec": {
					"workspaceName": "test-workspace1",
					"user": "test-user1",
					"groups": ["system:authenticated"]
				}
			}`)
			recorder := httptest.NewRecorder()

			mockSarClient.SetupAllowed("Permitted by RBAC")

			// Create a test workspace with the test user as owner
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkspaceName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						OwnerAnnotation: testUsername,
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					OwnershipType: "OwnerOnly",
				},
			}
			Expect(k8sClient.Create(context.Background(), workspace)).To(Succeed())

			server.handleConnectionAccessReview(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusOK))

			var response connectionv1alpha1.ConnectionAccessReview
			err := json.NewDecoder(recorder.Body).Decode(&response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Status.Allowed).To(BeTrue())
			Expect(response.Status.NotFound).To(BeFalse())
			Expect(response.Status.Reason).ToNot(BeEmpty())
		})

		It("Should respond with allowed=false, notFound=false, set a reason, when Create(SAR) and Get(Workspace) succeed and does not match owner", func() {
			req := createTestRequest("POST", "/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/test-namespace1/connectionaccessreview", `{
				"spec": {
					"workspaceName": "test-workspace1",
					"user": "test-user1",
					"groups": ["system:authenticated"]
				}
			}`)
			recorder := httptest.NewRecorder()

			mockSarClient.SetupAllowed("Permitted by RBAC")

			// Create a test workspace with a different user as owner
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkspaceName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						OwnerAnnotation: "different-user",
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					OwnershipType: "OwnerOnly",
				},
			}
			Expect(k8sClient.Create(context.Background(), workspace)).To(Succeed())

			server.handleConnectionAccessReview(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusOK))

			var response connectionv1alpha1.ConnectionAccessReview
			err := json.NewDecoder(recorder.Body).Decode(&response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Status.Allowed).To(BeFalse())
			Expect(response.Status.NotFound).To(BeFalse())
			Expect(response.Status.Reason).To(ContainSubstring("private"))
		})

		It("Should respond with allowed=false, notFound=true, set a reason, when Create(SAR) succeeds and Get(Workspace) returns not found", func() {
			req := createTestRequest("POST", "/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/test-namespace1/connectionaccessreview", `{
				"spec": {
					"workspaceName": "non-existent-workspace",
					"user": "test-user1",
					"groups": ["system:authenticated"]
				}
			}`)
			recorder := httptest.NewRecorder()

			mockSarClient.SetupAllowed("Permitted by RBAC")

			server.handleConnectionAccessReview(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusOK))

			var response connectionv1alpha1.ConnectionAccessReview
			err := json.NewDecoder(recorder.Body).Decode(&response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Status.Allowed).To(BeFalse())
			Expect(response.Status.NotFound).To(BeTrue())
			Expect(response.Status.Reason).To(ContainSubstring("not found"))
		})

		It("Should return an InternalServerError when Create(SAR) fails", func() {
			req := createTestRequest("POST", "/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/test-namespace1/connectionaccessreview", `{
				"spec": {
					"workspaceName": "test-workspace1",
					"user": "test-user1",
					"groups": ["system:authenticated"]
				}
			}`)
			recorder := httptest.NewRecorder()

			mockSarClient.SetupError(errors.New("SAR error"))

			server.handleConnectionAccessReview(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusInternalServerError))
			var response map[string]string
			err := json.NewDecoder(recorder.Body).Decode(&response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response).To(HaveKeyWithValue("error", "Failed to verify access permission"))
		})

		It("Should return an InternalServerError when Get(Workspace) fails", func() {
			req := createTestRequest("POST", "/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/test-namespace1/connectionaccessreview", `{
				"spec": {
					"workspaceName": "test-workspace1",
					"user": "test-user1",
					"groups": ["system:authenticated"]
				}
			}`)
			recorder := httptest.NewRecorder()

			mockSarClient.SetupAllowed("Permitted by RBAC")

			// Replace the client with one that returns an error
			errorClient := &mockErrorClient{
				getError: errors.New("get Workspace client error"),
			}

			errorServer := &ExtensionServer{
				k8sClient: errorClient,
				sarClient: mockSarClient,
				logger:    &logger,
			}

			errorServer.handleConnectionAccessReview(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusInternalServerError))
			var response map[string]string
			err := json.NewDecoder(recorder.Body).Decode(&response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response).To(HaveKeyWithValue("error", "Failed to verify access permission"))
		})
	})
})
