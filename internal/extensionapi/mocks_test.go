package extensionapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-logr/logr"
	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// MockManager implements the manager.Manager interface for testing
type MockManager struct {
	// Dependencies that can be injected
	client client.Client
	config *rest.Config
	logger logr.Logger
	scheme *runtime.Scheme

	// State for tracking test assertions
	runnables []manager.Runnable
	addError  error

	// Mock webhook server that satisfies the interface
	webhookServer *mockWebhookServer
}

// mockWebhookServer implements webhook.Server interface
type mockWebhookServer struct {
	mux *http.ServeMux
}

// NeedLeaderElection implements the LeaderElectionRunnable interface
func (s *mockWebhookServer) NeedLeaderElection() bool {
	return false
}

// Register registers a webhook handler
func (s *mockWebhookServer) Register(path string, hook http.Handler) {
	// Initialize the mux if needed
	if s.mux == nil {
		s.mux = http.NewServeMux()
	}
	s.mux.Handle(path, hook)
}

// Start starts the webhook server (no-op in mock)
func (s *mockWebhookServer) Start(ctx context.Context) error {
	return nil
}

// StartedChecker returns a health checker
func (s *mockWebhookServer) StartedChecker() healthz.Checker {
	return func(req *http.Request) error { return nil }
}

// WebhookMux returns the mux
func (s *mockWebhookServer) WebhookMux() *http.ServeMux {
	if s.mux == nil {
		s.mux = http.NewServeMux()
	}
	return s.mux
}

// NewMockManager creates a new MockManager for testing
func NewMockManager(k8sClient client.Client, restConfig *rest.Config, logger logr.Logger) *MockManager {
	scheme := runtime.NewScheme()
	return &MockManager{
		client:        k8sClient,
		config:        restConfig,
		logger:        logger,
		scheme:        scheme,
		runnables:     []manager.Runnable{},
		webhookServer: &mockWebhookServer{},
	}
}

// GetClient returns the mock client
func (m *MockManager) GetClient() client.Client {
	return m.client
}

// GetConfig returns the mock REST config
func (m *MockManager) GetConfig() *rest.Config {
	return m.config
}

// GetScheme returns the mock scheme
func (m *MockManager) GetScheme() *runtime.Scheme {
	return m.scheme
}

// GetLogger returns the mock logger
func (m *MockManager) GetLogger() logr.Logger {
	return m.logger
}

// Add adds a Runnable to the mock manager
func (m *MockManager) Add(runnable manager.Runnable) error {
	if m.addError != nil {
		return m.addError
	}
	m.runnables = append(m.runnables, runnable)
	return nil
}

// Elected returns a closed channel to simulate being elected leader
func (m *MockManager) Elected() <-chan struct{} {
	// Return a closed channel to simulate being elected
	ch := make(chan struct{})
	close(ch)
	return ch
}

// AddMetricsExtraHandler is provided for backward compatibility
func (m *MockManager) AddMetricsExtraHandler(path string, handler http.Handler) error {
	return nil
}

// AddMetricsServerExtraHandler implements the Manager interface
func (m *MockManager) AddMetricsServerExtraHandler(path string, handler http.Handler) error {
	return nil
}

// AddHealthzCheck implements the Manager interface
func (m *MockManager) AddHealthzCheck(name string, check healthz.Checker) error {
	return nil
}

// AddReadyzCheck implements the Manager interface
func (m *MockManager) AddReadyzCheck(name string, check healthz.Checker) error {
	return nil
}

// Start implements the Manager interface
func (m *MockManager) Start(ctx context.Context) error {
	return nil
}

// GetWebhookServer returns the mock webhook server
func (m *MockManager) GetWebhookServer() webhook.Server {
	return m.webhookServer
}

// GetControllerOptions returns mock controller options
func (m *MockManager) GetControllerOptions() config.Controller {
	return config.Controller{}
}

// GetCache returns a mock cache
func (m *MockManager) GetCache() cache.Cache {
	return nil
}

// GetFieldIndexer returns a mock field indexer
func (m *MockManager) GetFieldIndexer() client.FieldIndexer {
	return nil
}

// GetEventRecorderFor returns a mock event recorder
func (m *MockManager) GetEventRecorderFor(name string) record.EventRecorder {
	return nil
}

// GetRESTMapper returns a mock REST mapper
func (m *MockManager) GetRESTMapper() meta.RESTMapper {
	return nil
}

// GetAPIReader returns the mock client as an API reader
func (m *MockManager) GetAPIReader() client.Reader {
	return m.client
}

// GetHTTPClient returns a mock HTTP client
func (m *MockManager) GetHTTPClient() *http.Client {
	return &http.Client{}
}

// ErrorWriter is a custom ResponseWriter that returns an error on Write
// and implements the http.ResponseWriter interface
type ErrorWriter struct {
	headers    http.Header
	statusCode int
	Body       []byte
}

// Header returns the response headers
func (e *ErrorWriter) Header() http.Header {
	if e.headers == nil {
		e.headers = make(http.Header)
	}
	return e.headers
}

// WriteHeader captures the status code
func (e *ErrorWriter) WriteHeader(statusCode int) {
	e.statusCode = statusCode
}

// Write returns an error to simulate a write failure
func (e *ErrorWriter) Write(b []byte) (int, error) {
	// Simulate a write error
	return 0, errors.New("simulated write error")
}

// MockSarClient implements the v1.SubjectAccessReviewInterface for testing
type MockSarClient struct {
	// Record of calls for verification
	CreateCallCount  int
	LastCreateParams *authorizationv1.SubjectAccessReview

	// Behavior control
	CreateResponse *authorizationv1.SubjectAccessReview
	CreateError    error
}

// Create implements the SubjectAccessReviewInterface
func (m *MockSarClient) Create(ctx context.Context, sar *authorizationv1.SubjectAccessReview, opts metav1.CreateOptions) (*authorizationv1.SubjectAccessReview, error) {
	m.CreateCallCount++
	m.LastCreateParams = sar.DeepCopy() // Store a copy to avoid reference issues

	if m.CreateError != nil {
		return nil, m.CreateError
	}

	if m.CreateResponse != nil {
		return m.CreateResponse, nil
	}

	// Default behavior if no response is configured: approved SAR
	return &authorizationv1.SubjectAccessReview{
		Status: authorizationv1.SubjectAccessReviewStatus{
			Allowed: true,
			Reason:  "Default mock approval",
		},
	}, nil
}

// NewMockSarClient creates a new MockSarClient with default settings
func NewMockSarClient() *MockSarClient {
	return &MockSarClient{}
}

// SetupMockSarClientAllowed configures the mock to return an allowed response with the given reason
func (m *MockSarClient) SetupAllowed(reason string) *MockSarClient {
	m.CreateResponse = &authorizationv1.SubjectAccessReview{
		Status: authorizationv1.SubjectAccessReviewStatus{
			Allowed: true,
			Reason:  reason,
		},
	}
	return m
}

// SetupMockSarClientDenied configures the mock to return a denied response with the given reason
func (m *MockSarClient) SetupDenied(reason string) *MockSarClient {
	m.CreateResponse = &authorizationv1.SubjectAccessReview{
		Status: authorizationv1.SubjectAccessReviewStatus{
			Allowed: false,
			Reason:  reason,
		},
	}
	return m
}

// SetupMockSarClientError configures the mock to return an error
func (m *MockSarClient) SetupError(err error) *MockSarClient {
	m.CreateError = err
	return m
}
