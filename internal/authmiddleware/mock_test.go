package authmiddleware

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"

	v1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/connection/v1alpha1"
)

// MockJWTHandler implements the jwt.Handler interface for testing
type MockJWTHandler struct {
	GenerateTokenFunc          func(user string, groups []string, uid string, extra map[string][]string, path string, domain string, tokenType string) (string, error)
	ValidateTokenFunc          func(tokenString string) (*jwt.Claims, error)
	RefreshTokenFunc           func(claims *jwt.Claims) (string, error)
	UpdateSkipRefreshTokenFunc func(claims *jwt.Claims) (string, error)
	ShouldRefreshTokenFunc     func(claims *jwt.Claims) bool
}

// Ensure MockJWTHandler implements the jwt.Handler interface
var _ jwt.Handler = (*MockJWTHandler)(nil)

// GenerateToken calls the mock implementation
func (m *MockJWTHandler) GenerateToken(
	user string,
	groups []string,
	uid string,
	extra map[string][]string,
	path string,
	domain string,
	tokenType string) (string, error) {
	if m.GenerateTokenFunc != nil {
		return m.GenerateTokenFunc(user, groups, uid, extra, path, domain, tokenType)
	}
	return "mock-token", nil
}

// ValidateToken calls the mock implementation
func (m *MockJWTHandler) ValidateToken(tokenString string) (*jwt.Claims, error) {
	if m.ValidateTokenFunc != nil {
		return m.ValidateTokenFunc(tokenString)
	}
	return &jwt.Claims{User: "mock-user"}, nil
}

// RefreshToken calls the mock implementation
func (m *MockJWTHandler) RefreshToken(claims *jwt.Claims) (string, error) {
	if m.RefreshTokenFunc != nil {
		return m.RefreshTokenFunc(claims)
	}
	return "refreshed-mock-token", nil
}

// UpdateSkipRefreshToken calls the mock implementation
func (m *MockJWTHandler) UpdateSkipRefreshToken(claims *jwt.Claims) (string, error) {
	if m.RefreshTokenFunc != nil {
		return m.UpdateSkipRefreshTokenFunc(claims)
	}
	return "updated-mock-token", nil
}

// ShouldRefreshToken calls the mock implementation
func (m *MockJWTHandler) ShouldRefreshToken(claims *jwt.Claims) bool {
	if m.ShouldRefreshTokenFunc != nil {
		return m.ShouldRefreshTokenFunc(claims)
	}
	return false
}

// MockCookieHandler implements the CookieHandler interface for testing
type MockCookieHandler struct {
	SetCookieFunc   func(w http.ResponseWriter, token string, path string, domain string)
	GetCookieFunc   func(r *http.Request, path string) (string, error)
	ClearCookieFunc func(w http.ResponseWriter, path string, domain string)
}

// Ensure MockCookieHandler implements the CookieHandler interface
var _ CookieHandler = (*MockCookieHandler)(nil)

// SetCookie calls the mock implementation
func (m *MockCookieHandler) SetCookie(w http.ResponseWriter, token string, path string, domain string) {
	if m.SetCookieFunc != nil {
		m.SetCookieFunc(w, token, path, domain)
	}
}

// GetCookie calls the mock implementation
func (m *MockCookieHandler) GetCookie(r *http.Request, path string) (string, error) {
	if m.GetCookieFunc != nil {
		return m.GetCookieFunc(r, path)
	}
	return "mock-cookie-value", nil
}

// ClearCookie calls the mock implementation
func (m *MockCookieHandler) ClearCookie(w http.ResponseWriter, path string, domain string) {
	if m.ClearCookieFunc != nil {
		m.ClearCookieFunc(w, path, domain)
	}
}

// RequestRecord represents a recorded HTTP request
type RequestRecord struct {
	Method  string
	Path    string
	Body    []byte
	Headers http.Header
}

// MockK8sServer provides testing utilities for K8s REST client calls
type MockK8sServer struct {
	Server      *httptest.Server
	Handler     http.Handler
	Requests    []RequestRecord
	RequestLock sync.Mutex
	T           *testing.T
}

// NewMockK8sServer creates a new server for testing K8s REST client calls
func NewMockK8sServer(t *testing.T) *MockK8sServer {
	mock := &MockK8sServer{
		Requests: make([]RequestRecord, 0),
		T:        t,
	}

	// Default handler just records the request
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Default handler just records the request and returns 200 OK
		mock.RecordRequest(r)
		w.WriteHeader(http.StatusOK)
	})

	mock.Handler = handler
	mock.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mock.Handler.ServeHTTP(w, r)
	}))

	return mock
}

// RecordRequest stores a request for later inspection
func (m *MockK8sServer) RecordRequest(r *http.Request) {
	m.RequestLock.Lock()
	defer m.RequestLock.Unlock()

	// Read the body
	var body []byte
	if r.Body != nil {
		body = make([]byte, r.ContentLength)
		// We're ignoring the error here because this is test code,
		// and we're more interested in recording the request than
		// handling read errors.
		_, _ = r.Body.Read(body)
	}

	m.Requests = append(m.Requests, RequestRecord{
		Method:  r.Method,
		Path:    r.URL.Path,
		Body:    body,
		Headers: r.Header,
	})
}

// Close shuts down the test server
func (m *MockK8sServer) Close() {
	if m.Server != nil {
		m.Server.Close()
	}
}

// GetLastRequest returns the most recent request
func (m *MockK8sServer) GetLastRequest() *RequestRecord {
	m.RequestLock.Lock()
	defer m.RequestLock.Unlock()

	if len(m.Requests) == 0 {
		return nil
	}
	return &m.Requests[len(m.Requests)-1]
}

// CreateRESTClient creates a K8s REST client pointing to this test server
func (m *MockK8sServer) CreateRESTClient() (rest.Interface, error) {
	// Create a scheme and register your types
	scheme := runtime.NewScheme()
	err := v1alpha1.AddToScheme(scheme)

	if err != nil {
		return nil, err
	}

	// Create codec factory
	codecFactory := serializer.NewCodecFactory(scheme)

	config := &rest.Config{
		Host: m.Server.URL,
		ContentConfig: rest.ContentConfig{
			ContentType: runtime.ContentTypeJSON,
			GroupVersion: &schema.GroupVersion{
				Group:   v1alpha1.SchemeGroupVersion.Group,
				Version: v1alpha1.SchemeGroupVersion.Version},
			NegotiatedSerializer: codecFactory.WithoutConversion(),
		},
	}
	return rest.RESTClientFor(config)
}

// SetupServerWithHandler configures server with specific handler function
func (m *MockK8sServer) SetupServerWithHandler(handlerFunc http.HandlerFunc) {
	m.Handler = handlerFunc
}

type ConnectionAccessReviewResponse struct {
	Kind       string                                 `json:"kind"`
	ApiVersion string                                 `json:"apiVersion"`
	Metadata   *metav1.ObjectMeta                     `json:"metadata"`
	Spec       *v1alpha1.ConnectionAccessReviewSpec   `json:"spec"`
	Status     *v1alpha1.ConnectionAccessReviewStatus `json:"status"`
}

// CreateConnectionAccessReview returns the fully formatted ConnectionAccessReview
func CreateConnectionAccessReviewResponse(
	namespace string,
	workspaceName string,
	user string,
	groups []string,
	uid string,
	allowed bool,
	notFound bool,
	reason string,
	extra ...map[string][]string) *ConnectionAccessReviewResponse {

	// Default extra to nil if not provided
	var extraMap map[string][]string
	if len(extra) > 0 {
		extraMap = extra[0]
	}

	return &ConnectionAccessReviewResponse{
		Kind:       "ConnectionAccessReview",
		ApiVersion: fmt.Sprintf("%s/%s", v1alpha1.SchemeGroupVersion.Group, v1alpha1.SchemeGroupVersion.Version),
		Metadata:   &metav1.ObjectMeta{Namespace: namespace},
		Spec: &v1alpha1.ConnectionAccessReviewSpec{
			WorkspaceName: workspaceName,
			User:          user,
			Groups:        groups,
			UID:           uid,
			Extra:         extraMap,
		},
		Status: &v1alpha1.ConnectionAccessReviewStatus{
			Allowed:  allowed,
			NotFound: notFound,
			Reason:   reason,
		},
	}
}

// SetupServerEmpty200OK configures server to return 200 OK
func (m *MockK8sServer) SetupServerEmpty200OK() {
	m.SetupServerWithHandler(func(w http.ResponseWriter, r *http.Request) {
		status := &metav1.Status{
			Status: "Ok",
			Code:   http.StatusAccepted,
		}
		statusJSON, _ := json.Marshal(status)
		m.RecordRequest(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(statusJSON)
	})
}

// SetupServer200OK configures server to return 200 OK
func (m *MockK8sServer) SetupServer200OK(response *ConnectionAccessReviewResponse) {
	// Ignoring the errors (test code)
	bytesResponse, _ := json.Marshal(response)

	m.SetupServerWithHandler(func(w http.ResponseWriter, r *http.Request) {
		m.RecordRequest(r)
		w.Header().Set("Content-Type", runtime.ContentTypeJSON)
		w.WriteHeader(http.StatusOK)
		// We're ignoring the error (test code)
		_, _ = w.Write(bytesResponse)
	})
}

// SetupServer403Forbidden configures server to return 403 Forbidden
func (m *MockK8sServer) SetupServer403Forbidden() {
	m.SetupServerWithHandler(func(w http.ResponseWriter, r *http.Request) {
		m.RecordRequest(r)

		// Create a Kubernetes style Forbidden error
		status := &metav1.Status{
			Status:  "Failure",
			Message: "forbidden: the authmiddleware may not perform this action",
			Reason:  metav1.StatusReasonForbidden,
			Code:    http.StatusForbidden,
		}

		statusJSON, _ := json.Marshal(status)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		// We're ignoring the error here because this is test code
		_, _ = w.Write(statusJSON)
	})
}

// SetupServer404NotFound configures server to return 404 Not Found
func (m *MockK8sServer) SetupServer404NotFound() {
	m.SetupServerWithHandler(func(w http.ResponseWriter, r *http.Request) {
		m.RecordRequest(r)

		// Create a Kubernetes style Not Found error
		status := &metav1.Status{
			Status:  "Not Found",
			Message: "not found: extensionapi not properly configured",
			Reason:  metav1.StatusReasonNotFound,
			Code:    http.StatusNotFound,
		}

		statusJSON, _ := json.Marshal(status)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		// We're ignoring the error here because this is test code
		_, _ = w.Write(statusJSON)
	})
}

// SetupServer401Unauthorized configures server to return 401 Unauthorized
func (m *MockK8sServer) SetupServer401Unauthorized() {
	m.SetupServerWithHandler(func(w http.ResponseWriter, r *http.Request) {
		m.RecordRequest(r)

		// Create a Kubernetes style Unauthorized error
		status := &metav1.Status{
			Status:  "Unauthorized",
			Message: "unauthorized: authmiddleware is missing k8s permissions",
			Reason:  metav1.StatusReasonUnauthorized,
			Code:    http.StatusUnauthorized,
		}

		statusJSON, _ := json.Marshal(status)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		// We're ignoring the error here because this is test code
		_, _ = w.Write(statusJSON)
	})
}

// SetupServer500InternalServerError configures server to return 500 Internal Server Error
func (m *MockK8sServer) SetupServer500InternalServerError() {
	m.SetupServerWithHandler(func(w http.ResponseWriter, r *http.Request) {
		m.RecordRequest(r)

		// Create a Kubernetes style Internal Server Error
		status := &metav1.Status{
			Status:  "Failure",
			Message: "Internal Server Error",
			Reason:  metav1.StatusReasonInternalError,
			Code:    http.StatusInternalServerError,
		}

		statusJSON, _ := json.Marshal(status)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		// We're ignoring the error here because this is test code
		_, _ = w.Write(statusJSON)
	})
}

// AssertRequestPath checks if the path in the request matches the expected path
func (m *MockK8sServer) AssertRequestPath(expectedPath string) {
	req := m.GetLastRequest()
	require.NotNil(m.T, req, "No request was recorded")
	assert.Equal(m.T, expectedPath, req.Path, "Request path does not match")
}

// AssertRequestMethod checks if the method in the request matches the expected method
func (m *MockK8sServer) AssertRequestMethod(expectedMethod string) {
	req := m.GetLastRequest()
	require.NotNil(m.T, req, "No request was recorded")
	assert.Equal(m.T, expectedMethod, req.Method, "Request method does not match")
}

// AssertRequestHeader checks if the header in the request matches the expected value
func (m *MockK8sServer) AssertRequestHeader(header, expectedValue string) {
	req := m.GetLastRequest()
	require.NotNil(m.T, req, "No request was recorded")
	assert.Equal(m.T, expectedValue, req.Headers.Get(header), "Request header does not match")
}

// AssertRequestBody checks if the body in the request matches the expected body
func (m *MockK8sServer) AssertRequestBody(expectedBody string) {
	req := m.GetLastRequest()
	require.NotNil(m.T, req, "No request was recorded")
	assert.Equal(m.T, expectedBody, string(req.Body), "Request body does not match")
}

// MockOIDCVerifier is a mock implementation of an OIDC verifier for testing
type MockOIDCVerifier struct {
	VerifyTokenFunc func(ctx context.Context, tokenString string, logger *slog.Logger) (*OIDCClaims, bool, error)
	StartFunc       func(ctx context.Context) error
}

// VerifyToken calls the mock implementation function
func (m *MockOIDCVerifier) VerifyToken(ctx context.Context, tokenString string, logger *slog.Logger) (*OIDCClaims, bool, error) {
	if m.VerifyTokenFunc != nil {
		return m.VerifyTokenFunc(ctx, tokenString, logger)
	}
	// Default implementation with successful verification
	claims := &OIDCClaims{
		Subject:  "test-subject",
		Username: "test-user",
		Groups:   []string{"test-group"},
	}
	return claims, false, nil
}

// Start implements the Start method required by OIDCVerifierInterface
func (m *MockOIDCVerifier) Start(ctx context.Context) error {
	if m.StartFunc != nil {
		return m.StartFunc(ctx)
	}
	// Default implementation with successful initialization
	return nil
}
