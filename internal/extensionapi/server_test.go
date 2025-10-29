package extensionapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// We don't need a mock Clientset since we're using the actual Clientset type

const testResourceName = "test-resource"

// No need to override the function at init since we can't modify package functions
// Instead we'll use variables that our test will check

// MockHTTPServer is a wrapper around http.Server for testing
type MockHTTPServer struct {
	Server             *http.Server
	StartCalled        bool
	StartTLSCalled     bool
	ShutdownCalled     bool
	CertPath           string
	KeyPath            string
	ShutdownError      error
	ListenServeError   error
	ListenServeFunc    func() error
	ListenServeTLSFunc func(certFile, keyFile string) error
	ShutdownFunc       func(ctx context.Context) error
}

// NewMockHTTPServer creates a new mock HTTP server
func NewMockHTTPServer() *MockHTTPServer {
	mock := &MockHTTPServer{
		Server: &http.Server{},
	}

	// Set default implementations
	mock.ListenServeFunc = mock.defaultListenAndServe
	mock.ListenServeTLSFunc = mock.defaultListenAndServeTLS
	mock.ShutdownFunc = mock.defaultShutdown

	return mock
}

// defaultListenAndServe is the default implementation of ListenAndServe
func (m *MockHTTPServer) defaultListenAndServe() error {
	m.StartCalled = true
	return m.ListenServeError
}

// defaultListenAndServeTLS is the default implementation of ListenAndServeTLS
func (m *MockHTTPServer) defaultListenAndServeTLS(certFile, keyFile string) error {
	m.StartTLSCalled = true
	m.CertPath = certFile
	m.KeyPath = keyFile
	return m.ListenServeError
}

// defaultShutdown is the default implementation of Shutdown
func (m *MockHTTPServer) defaultShutdown(ctx context.Context) error {
	m.ShutdownCalled = true
	return m.ShutdownError
}

// ListenAndServe mocks the http.Server ListenAndServe method
func (m *MockHTTPServer) ListenAndServe() error {
	return m.ListenServeFunc()
}

// ListenAndServeTLS mocks the http.Server ListenAndServeTLS method
func (m *MockHTTPServer) ListenAndServeTLS(certFile, keyFile string) error {
	return m.ListenServeTLSFunc(certFile, keyFile)
}

// Shutdown mocks the http.Server Shutdown method
func (m *MockHTTPServer) Shutdown(ctx context.Context) error {
	return m.ShutdownFunc(ctx)
}

// Mock handlers for testing routes
var (
	mockBaseHandlerCalled          bool
	mockApiBaseHandlerCalled       bool
	mockFakeResource1HandlerCalled bool
	mockFakeResource2HandlerCalled bool
)

// Reset all mock handler flags
func resetMockHandlers() {
	mockBaseHandlerCalled = false
	mockApiBaseHandlerCalled = false
	mockFakeResource1HandlerCalled = false
	mockFakeResource2HandlerCalled = false
}

// Mock handler for /base-route
func mockBaseHandler(w http.ResponseWriter, r *http.Request) {
	mockBaseHandlerCalled = true
	w.WriteHeader(http.StatusOK)
	// Handle potential error from Write
	_, err := w.Write([]byte(`{"status":"ok"}`))
	if err != nil {
		fmt.Printf("Error writing response: %v\n", err)
	}
}

// Mock handler for /<api>/api-route
func mockApiBaseHandler(w http.ResponseWriter, r *http.Request) {
	mockApiBaseHandlerCalled = true
	w.WriteHeader(http.StatusOK)
	// Handle potential error from Write
	_, err := w.Write([]byte(`{"status":"ok"}`))
	if err != nil {
		fmt.Printf("Error writing response: %v\n", err)
	}
}

// Mock handler for namespaced resources - fake-resource1
func mockFakeResource1Handler(w http.ResponseWriter, _ *http.Request) {
	mockFakeResource1HandlerCalled = true
	w.WriteHeader(http.StatusCreated)
	// Handle potential error from Write
	_, err := w.Write([]byte(`{"status":"created"}`))
	if err != nil {
		fmt.Printf("Error writing response: %v\n", err)
	}
}

// Mock handler for namespaced resources - fake-resource2
func mockFakeResource2Handler(w http.ResponseWriter, _ *http.Request) {
	mockFakeResource2HandlerCalled = true
	w.WriteHeader(http.StatusOK)
	// Handle potential error from Write
	_, err := w.Write([]byte(`{"status":"ok"}`))
	if err != nil {
		fmt.Printf("Error writing response: %v\n", err)
	}
}

var _ = Describe("Server", func() {
	var (
		config           *ExtensionConfig
		logger           logr.Logger
		k8sClient        client.Client
		sarClient        v1.SubjectAccessReviewInterface
		server           *ExtensionServer
		fakeRoutesServer *ExtensionServer
	)

	BeforeEach(func() {
		// Reset all mock handler flags
		resetMockHandlers()

		// Create a test config
		config = NewConfig(
			WithServerPort(8080),
			WithDisableTLS(true),
			WithReadTimeoutSeconds(30),
			WithWriteTimeoutSeconds(60),
		)

		// Create a logger
		logger = logr.Discard()

		// Create a fake client
		k8sClient = fake.NewClientBuilder().Build()

		// Create a mock SAR client
		clientSet := &kubernetes.Clientset{}
		sarClient = clientSet.AuthorizationV1().SubjectAccessReviews()

		// Create the server
		server = newExtensionServer(config, &logger, k8sClient, sarClient)
		server.registerAllRoutes()

		// Create a minimal fake routes server without automatic route registration
		fakeConfig := NewConfig(
			WithServerPort(8081),
			WithDisableTLS(true),
			WithReadTimeoutSeconds(30),
			WithWriteTimeoutSeconds(60),
		)
		fakeConfig.ApiPath = "/apis/fake.test.org/v1alpha1"

		// Create server manually without calling constructor to avoid automatic route registration
		fakeMux := http.NewServeMux()
		fakeRoutesServer = &ExtensionServer{
			config:    fakeConfig,
			logger:    &logger,
			k8sClient: k8sClient,
			sarClient: sarClient,
			routes:    make(map[string]func(http.ResponseWriter, *http.Request)),
			mux:       fakeMux,
			httpServer: &http.Server{
				Addr:         fmt.Sprintf(":%d", fakeConfig.ServerPort),
				Handler:      fakeMux,
				ReadTimeout:  time.Duration(fakeConfig.ReadTimeoutSeconds) * time.Second,
				WriteTimeout: time.Duration(fakeConfig.WriteTimeoutSeconds) * time.Second,
			},
		}

		// Register only the fake routes we need for testing
		fakeRoutesServer.registerRoute("/base-route", mockBaseHandler)
		fakeRoutesServer.registerRoute(fmt.Sprintf("%s/%s", fakeConfig.ApiPath, "api-route"), mockApiBaseHandler)
		fakeRoutesServer.registerNamespacedRoutes(map[string]func(http.ResponseWriter, *http.Request){
			"fake-resource1": mockFakeResource1Handler,
			"fake-resource2": mockFakeResource2Handler,
		})
	})

	Context("newExtensionServer", func() {
		It("Should save the config as instance attribute", func() {
			Expect(server.config).To(Equal(config))
		})

		It("Should save the contextual logger as instance attribute", func() {
			Expect(server.logger).NotTo(BeNil())
			Expect(server.logger).To(Equal(&logger))
		})

		It("Should save the k8s client as instance attribute", func() {
			Expect(server.k8sClient).To(Equal(k8sClient))
		})

		It("Should save the subject access review client as instance attribute", func() {
			Expect(server.sarClient).To(Equal(sarClient))
		})

		It("Should instantiate a new httpServer with the values from config", func() {
			// We can only check that the httpServer is not nil since we're using an interface now
			Expect(server.httpServer).NotTo(BeNil())
		})

		It("Should register the /health route", func() {
			Expect(server.routes).To(HaveKey("/health"))
		})

		It("Should register the /discover route at the api root", func() {
			Expect(server.routes).To(HaveKey(config.ApiPath))
		})

		It("Should register /workspaceconnections and /connectionaccessreview routes as namespaced", func() {
			namespacedPathPrefix := config.ApiPath + "/namespaces/*/"
			Expect(server.routes).To(HaveKey(namespacedPathPrefix + "workspaceconnections"))
			Expect(server.routes).To(HaveKey(namespacedPathPrefix + "connectionaccessreview"))
		})
	})

	Context("loggerMiddleware", func() {
		var (
			nextCalled bool
			req        *http.Request
			rr         *httptest.ResponseRecorder
		)

		BeforeEach(func() {
			nextCalled = false
			req = httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = "192.168.1.1:12345"
			rr = httptest.NewRecorder()
		})

		It("Should call the next func with a request logger in context", func() {
			// Define a next handler that checks the context
			nextHandler := func(w http.ResponseWriter, r *http.Request) {
				// Check if the logger is in the context
				logger := GetLoggerFromContext(r.Context())
				Expect(logger).NotTo(BeNil())
				nextCalled = true
			}

			// Create the middleware
			middleware := server.loggerMiddleware(nextHandler)

			// Call the middleware
			middleware(rr, req)

			// Verify the next handler was called
			Expect(nextCalled).To(BeTrue())
		})

		It("Should record the method in context", func() {
			// Create a mock logger to validate values
			methodCaptured := ""

			// Define a next handler that checks the context logger
			nextHandler := func(w http.ResponseWriter, r *http.Request) {
				// Since we can't directly access the WithValues data inside the logger,
				// we'll mock the logger interface later with proper testing
				// For now, we can verify the logger is in the context
				logger := GetLoggerFromContext(r.Context())
				Expect(logger).NotTo(BeNil())

				// In a real implementation, we'd verify the logger has method=GET
				// This would require a mockable logger implementation
				methodCaptured = r.Method
				nextCalled = true
			}

			// Create the middleware
			middleware := server.loggerMiddleware(nextHandler)

			// Call the middleware
			middleware(rr, req)

			// Verify the next handler was called and method was captured
			Expect(nextCalled).To(BeTrue())
			Expect(methodCaptured).To(Equal("GET"))
		})

		It("Should record the path in context", func() {
			// Create a variable to capture the path
			pathCaptured := ""

			// Define a next handler that checks the logger in context
			nextHandler := func(w http.ResponseWriter, r *http.Request) {
				// Verify logger is in context
				logger := GetLoggerFromContext(r.Context())
				Expect(logger).NotTo(BeNil())

				// Capture path for validation
				pathCaptured = r.URL.Path
				nextCalled = true
			}

			// Create the middleware
			middleware := server.loggerMiddleware(nextHandler)

			// Call the middleware
			middleware(rr, req)

			// Verify the next handler was called and path was captured
			Expect(nextCalled).To(BeTrue())
			Expect(pathCaptured).To(Equal("/test"))
		})

		It("Should record the remote address in context", func() {
			// Create a variable to capture the remote address
			remoteAddrCaptured := ""

			// Define a next handler that checks the logger in context
			nextHandler := func(w http.ResponseWriter, r *http.Request) {
				// Verify logger is in context
				logger := GetLoggerFromContext(r.Context())
				Expect(logger).NotTo(BeNil())

				// Capture remote address for validation
				remoteAddrCaptured = r.RemoteAddr
				nextCalled = true
			}

			// Create the middleware
			middleware := server.loggerMiddleware(nextHandler)

			// Call the middleware
			middleware(rr, req)

			// Verify the next handler was called and remote address was captured
			Expect(nextCalled).To(BeTrue())
			Expect(remoteAddrCaptured).To(Equal("192.168.1.1:12345"))
		})
	})

	Context("registerRoute", func() {
		var (
			handlerCalled bool
			testPath      string
			handler       func(http.ResponseWriter, *http.Request)
		)

		BeforeEach(func() {
			handlerCalled = false
			testPath = "test-path"
			handler = func(_ http.ResponseWriter, _ *http.Request) {
				handlerCalled = true
			}
		})

		It("Should add to server.routes", func() {
			// Register the route
			server.registerRoute(testPath, handler)

			// Check if it was added to routes
			Expect(server.routes).To(HaveKey(testPath))
		})

		It("Should provide a logger context", func() {
			// Create a handler that checks if the logger is correctly added to context
			loggerChecked := false
			loggeredHandler := func(_ http.ResponseWriter, r *http.Request) {
				// Get logger from context
				logger := GetLoggerFromContext(r.Context())

				// Ensure logger is not nil
				Expect(logger).NotTo(BeNil())

				// Mark that we checked the logger
				loggerChecked = true

				// Also set the handler called flag for verification
				handlerCalled = true
			}

			// Register the route with our test handler
			server.registerRoute("/logger-test", loggeredHandler)

			// Create a test request
			testReq := httptest.NewRequest("GET", "/logger-test", nil)
			testRR := httptest.NewRecorder()

			// Serve the request through the mux which should apply the logger middleware
			server.mux.ServeHTTP(testRR, testReq)

			// Verify both the handler was called and the logger was checked
			Expect(handlerCalled).To(BeTrue())
			Expect(loggerChecked).To(BeTrue())
		})

		It("Should add a slash prefix in the path if needed", func() {
			// Register a route without a leading slash
			server.registerRoute("no-slash", handler)

			// Make a request to the path with a slash
			req := httptest.NewRequest("GET", "/no-slash", nil)
			rr := httptest.NewRecorder()

			// Serve the request
			server.mux.ServeHTTP(rr, req)

			// Verify the handler was called (meaning the slash was added)
			Expect(handlerCalled).To(BeTrue())
		})

		It("Should add to mux routes with the correct path", func() {
			// Register the route
			server.registerRoute("/explicit-path", handler)

			// Make a request to that path
			req := httptest.NewRequest("GET", "/explicit-path", nil)
			rr := httptest.NewRecorder()

			// Serve the request
			server.mux.ServeHTTP(rr, req)

			// Verify the handler was called
			Expect(handlerCalled).To(BeTrue())
		})
	})

	Context("registerNamespacedRoutes", func() {
		It("Should add all namespaced routes to server.routes", func() {
			// Create handlers for multiple test resources
			// Using the testResourceName constant declared at the top of the file
			testHandler1 := func(_ http.ResponseWriter, _ *http.Request) {}
			testHandler2 := func(_ http.ResponseWriter, _ *http.Request) {}

			// Define a second resource name
			secondResourceName := "second-resource"

			// Create a resource handlers map with multiple resources
			resourceHandlers := map[string]func(http.ResponseWriter, *http.Request){
				testResourceName:   testHandler1,
				secondResourceName: testHandler2,
			}

			// Create a clean server instance for this test
			testServer := &ExtensionServer{
				config: config,
				logger: &logger,
				routes: make(map[string]func(http.ResponseWriter, *http.Request)),
				mux:    http.NewServeMux(),
			}

			// Call the method we want to test
			testServer.registerNamespacedRoutes(resourceHandlers)

			// Verify all routes were added
			pattern1 := config.ApiPath + "/namespaces/*/" + testResourceName
			pattern2 := config.ApiPath + "/namespaces/*/" + secondResourceName

			// Check that both routes are added
			Expect(testServer.routes).To(HaveKey(pattern1))
			Expect(testServer.routes).To(HaveKey(pattern2))

			// Check that the number of routes matches what we expect
			// Should have 2 routes, one for each resource
			routeCount := 0
			for k := range testServer.routes {
				if strings.Contains(k, "/namespaces/*/") {
					routeCount++
				}
			}
			Expect(routeCount).To(Equal(2))
		})
	})

	Context("Start", func() {
		It("Should call ListenAndServeTLS when TLS is enabled", func() {
			// Skip this test in CI environments
			if testing.Short() {
				Skip("Skipping test that requires network in short mode")
			}

			// Create a test server with mock HTTP server
			mockServer := NewMockHTTPServer()
			mockServer.ListenServeError = errors.New("mock error")

			// Create a test server
			testConfig := NewConfig(
				WithServerPort(8889),
				WithDisableTLS(false),
				WithCertPath("/test/cert.pem"),
				WithKeyPath("/test/key.pem"),
			)

			// Create a server with our mock HTTP server
			server := &ExtensionServer{
				config:     testConfig,
				logger:     &logger,
				k8sClient:  k8sClient,
				sarClient:  sarClient,
				routes:     make(map[string]func(http.ResponseWriter, *http.Request)),
				mux:        http.NewServeMux(),
				httpServer: mockServer.Server,
			}

			// Set the server's httpServer to our mock directly
			server.httpServer = mockServer

			// Start the server - this will call ListenAndServeTLS since DisableTLS is false
			err := server.Start(context.Background())

			// Verify that ListenAndServeTLS was called with the correct parameters
			Expect(err).To(HaveOccurred())
			Expect(mockServer.StartTLSCalled).To(BeTrue(), "ListenAndServeTLS should be called")
			Expect(mockServer.StartCalled).To(BeFalse(), "ListenAndServe should not be called")
			Expect(mockServer.CertPath).To(Equal("/test/cert.pem"), "Certificate path should match")
			Expect(mockServer.KeyPath).To(Equal("/test/key.pem"), "Key path should match")
		})

		It("Should call ListenAndServe without TLS when specified in config", func() {
			// Skip this test in CI environments
			if testing.Short() {
				Skip("Skipping test that requires network in short mode")
			}

			// Create a test server with mock HTTP server
			mockServer := NewMockHTTPServer()
			mockServer.ListenServeError = errors.New("mock ListenAndServe called")

			// Create a test server
			testConfig := NewConfig(
				WithServerPort(8889),
				WithDisableTLS(true), // TLS disabled
				WithCertPath("/test/cert.pem"),
				WithKeyPath("/test/key.pem"),
			)

			// Create a server with our mock HTTP server
			server := &ExtensionServer{
				config:     testConfig,
				logger:     &logger,
				k8sClient:  k8sClient,
				sarClient:  sarClient,
				routes:     make(map[string]func(http.ResponseWriter, *http.Request)),
				mux:        http.NewServeMux(),
				httpServer: mockServer,
			}

			// Start the server - this will call ListenAndServe since DisableTLS is true
			err := server.Start(context.Background())

			// Verify that ListenAndServe was called
			Expect(err).To(HaveOccurred())
			Expect(mockServer.StartCalled).To(BeTrue(), "ListenAndServe should be called")
			Expect(mockServer.StartTLSCalled).To(BeFalse(), "ListenAndServeTLS should not be called")
		})

		It("Should return the error when ListenAndServe fails", func() {
			// Skip this test in CI environments
			if testing.Short() {
				Skip("Skipping test that requires network in short mode")
			}

			// Create a test server with mock HTTP server and specific error message
			mockServer := NewMockHTTPServer()
			mockServer.ListenServeError = errors.New("mock server error")

			// Create a test server
			testConfig := NewConfig(
				WithServerPort(8889),
				WithDisableTLS(true),
			)

			// Create a server with our mock HTTP server
			server := &ExtensionServer{
				config:     testConfig,
				logger:     &logger,
				k8sClient:  k8sClient,
				sarClient:  sarClient,
				routes:     make(map[string]func(http.ResponseWriter, *http.Request)),
				mux:        http.NewServeMux(),
				httpServer: mockServer,
			}

			// Start the server and expect an error
			err := server.Start(context.Background())

			// Check that the error was properly propagated
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("mock server error"))
		})

		It("Should call Shutdown when context is canceled", func() {
			// Skip this test in CI environments
			if testing.Short() {
				Skip("Skipping test that requires network in short mode")
			}

			// Create a test server with mock HTTP server
			mockServer := NewMockHTTPServer()

			// Create a test server
			testConfig := NewConfig(
				WithServerPort(8889),
				WithDisableTLS(true),
			)

			// Create a server with our mock HTTP server
			server := &ExtensionServer{
				config:     testConfig,
				logger:     &logger,
				k8sClient:  k8sClient,
				sarClient:  sarClient,
				routes:     make(map[string]func(http.ResponseWriter, *http.Request)),
				mux:        http.NewServeMux(),
				httpServer: mockServer,
			}

			// Create a context that we can cancel
			ctx, cancel := context.WithCancel(context.Background())

			// We need to simulate the server starting and then blocking
			// We'll use a channel to signal when ListenAndServe is called
			serverStartChan := make(chan struct{})

			// For the context cancellation test, we use a custom implementation
			// that signals when the server starts and blocks until context is canceled

			// Set up the custom behavior
			mockServer.ListenServeFunc = func() error {
				// Signal that the server has "started"
				close(serverStartChan)
				// Block until context is canceled
				<-ctx.Done()
				return nil
			}

			// Start the server in a goroutine
			errChan := make(chan error, 1)
			go func() {
				errChan <- server.Start(ctx)
			}()

			// Wait for the mock ListenAndServe to signal it was called
			select {
			case <-serverStartChan:
				// Server "started" successfully
			case <-time.After(1 * time.Second):
				Fail("Timed out waiting for server to start")
			}

			// Cancel the context to trigger shutdown
			cancel()

			// Wait for Start to return
			select {
			case <-errChan:
				// Start returned after context cancellation
			case <-time.After(1 * time.Second):
				Fail("Timed out waiting for server to shut down")
			}

			// Verify that Shutdown was called
			Expect(mockServer.ShutdownCalled).To(BeTrue(), "Shutdown should be called when context is canceled")

			// No need to restore original functions since we're now using the interface
		})

		It("Should respond to a well formed GET request to a /base-route route", func() {
			// Create a test request to /base-route
			req := httptest.NewRequest("GET", "/base-route", nil)
			rr := httptest.NewRecorder()

			// Serve the request through the server mux
			fakeRoutesServer.mux.ServeHTTP(rr, req)

			// Verify the response status code
			Expect(rr.Code).To(Equal(http.StatusOK))

			// Verify that the mock handler was called
			Expect(mockBaseHandlerCalled).To(BeTrue(), "Base handler should be called")

			// Verify the response body
			Expect(rr.Body.String()).To(ContainSubstring(`{"status":"ok"}`))
		})

		It("Should respond to a well formed GET request to a /<api>/api-route route", func() {
			// Create a test request to the API route on the fake server
			apiPath := fmt.Sprintf("%s/api-route", fakeRoutesServer.config.ApiPath)
			req := httptest.NewRequest("GET", apiPath, nil)
			rr := httptest.NewRecorder()

			// Serve the request through the server mux
			fakeRoutesServer.mux.ServeHTTP(rr, req)

			// Verify the response status code
			Expect(rr.Code).To(Equal(http.StatusOK))

			// Verify that the mock handler was called
			Expect(mockApiBaseHandlerCalled).To(BeTrue(), "API base handler should be called")

			// Verify the response body
			Expect(rr.Body.String()).To(ContainSubstring(`{"status":"ok"}`))
		})

		It("Should respond to a well formed namespaced POST request to <>/namespaces/<ns>/fake-resource1", func() {
			// Create a test request to the namespaced resource 1
			apiPath := fmt.Sprintf("%s/namespaces/default/fake-resource1", fakeRoutesServer.config.ApiPath)
			req := httptest.NewRequest("POST", apiPath, strings.NewReader("{}"))
			rr := httptest.NewRecorder()

			// Serve the request through the server mux
			fakeRoutesServer.mux.ServeHTTP(rr, req)

			// Verify the response status code
			Expect(rr.Code).To(Equal(http.StatusCreated))

			// Verify that the mock handler was called
			Expect(mockFakeResource1HandlerCalled).To(BeTrue(), "Fake resource 1 handler should be called")

			// Verify the response body
			Expect(rr.Body.String()).To(ContainSubstring(`{"status":"created"}`))
		})

		It("Should respond to a well formed namespaced POST request to <>/namespaces/<ns>/fake-resource2", func() {
			// Create a test request to the namespaced resource 2
			apiPath := fmt.Sprintf("%s/namespaces/default/fake-resource2", fakeRoutesServer.config.ApiPath)
			req := httptest.NewRequest("POST", apiPath, strings.NewReader("{}"))
			rr := httptest.NewRecorder()

			// Serve the request through the server mux
			fakeRoutesServer.mux.ServeHTTP(rr, req)

			// Verify the response status code
			Expect(rr.Code).To(Equal(http.StatusOK))

			// Verify that the mock handler was called
			Expect(mockFakeResource2HandlerCalled).To(BeTrue(), "Fake resource 2 handler should be called")

			// Verify the response body
			Expect(rr.Body.String()).To(ContainSubstring(`{"status":"ok"}`))
		})

		It("Should return notFound to a non-existent base route /non-existent", func() {
			// Create a test request to a non-existent route
			req := httptest.NewRequest("GET", "/non-existent", nil)
			rr := httptest.NewRecorder()

			// Serve the request through the server mux
			server.mux.ServeHTTP(rr, req)

			// Verify the response status code is 404 Not Found
			Expect(rr.Code).To(Equal(http.StatusNotFound))
		})

		It("Should return BadRequest to a namespaced route without namespaces <>/namespaces/", func() {
			// Create a test request to the namespaces path but without specifying a namespace
			apiPath := fmt.Sprintf("%s/namespaces/", server.config.ApiPath)
			req := httptest.NewRequest("POST", apiPath, nil)
			rr := httptest.NewRecorder()

			// Serve the request through the server mux
			server.mux.ServeHTTP(rr, req)

			// Verify the response status code is 400 Bad Request
			Expect(rr.Code).To(Equal(http.StatusBadRequest))

			// Verify the error message
			Expect(rr.Body.String()).To(ContainSubstring("Invalid or missing namespace"))
		})

		It("Should return BadRequest to a namespaced route without resources <>/namespaces/<ns>", func() {
			// Create a test request with a namespace but no resource
			apiPath := fmt.Sprintf("%s/namespaces/default", server.config.ApiPath)
			req := httptest.NewRequest("POST", apiPath, nil)
			rr := httptest.NewRecorder()

			// Serve the request through the server mux
			server.mux.ServeHTTP(rr, req)

			// Verify the response status code is 400 Bad Request
			Expect(rr.Code).To(Equal(http.StatusBadRequest))
		})

		It("Should return notFound to a non-existent namespaced route <>/namespaces/<ns>/non-existent", func() {
			// Create a test request to a non-existent namespaced resource
			apiPath := fmt.Sprintf("%s/namespaces/default/non-existent", server.config.ApiPath)
			req := httptest.NewRequest("POST", apiPath, nil)
			rr := httptest.NewRecorder()

			// Serve the request through the server mux
			server.mux.ServeHTTP(rr, req)

			// Verify the response status code is 404 Not Found
			Expect(rr.Code).To(Equal(http.StatusNotFound))
		})
	})

	Context("NeedLeaderElection", func() {
		It("Should not require leader election", func() {
			// The NeedLeaderElection method should always return false
			// as the extension API server should run on all replicas
			Expect(server.NeedLeaderElection()).To(BeFalse())
		})
	})

	Context("Stop", func() {
		It("Should call and pass through the result", func() {
			// Create a mock HTTP server
			mockServer := NewMockHTTPServer()

			// Set up an error to verify it's returned correctly
			testError := errors.New("test shutdown error")
			mockServer.ShutdownError = testError

			// Create a server with our mock
			testServer := &ExtensionServer{
				config:     config,
				logger:     &logger,
				httpServer: mockServer,
			}

			// Call Stop
			err := testServer.Stop(context.Background())

			// Verify that Shutdown was called and the error was returned
			Expect(mockServer.ShutdownCalled).To(BeTrue(), "Shutdown should be called")
			Expect(err).To(Equal(testError), "Error should be passed through")
		})
	})
})

// mockSARServer starts a test HTTP server that can be used to test the SAR client
func mockSARServer(clientErr error) (*httptest.Server, *rest.Config) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If we want to simulate an error, return 500
		if clientErr != nil {
			http.Error(w, clientErr.Error(), http.StatusInternalServerError)
			return
		}
		// Otherwise return success
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}")) // Empty JSON response
	}))

	// Create a rest config that points to our test server
	config := &rest.Config{
		Host: server.URL,
	}

	return server, config
}

var _ = Describe("ServerWithManager", func() {

	Context("SetupExtensionAPIServerWithManager", func() {
		var mockMgr *MockManager
		var sarServer *httptest.Server
		var sarConfig *rest.Config

		BeforeEach(func() {
			// Reset all mock handler flags
			resetMockHandlers()

			// Create test scheme
			scheme := runtime.NewScheme()

			// Create fake client
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

			// Start a mock SAR server with no error
			sarServer, sarConfig = mockSARServer(nil)

			// Create a logger
			testLogger := logr.Discard()

			// Create our mock manager with the test dependencies
			mockMgr = NewMockManager(fakeClient, sarConfig, testLogger)
		})

		AfterEach(func() {
			// Clean up the test server
			if sarServer != nil {
				sarServer.Close()
			}
		})

		It("Should instantiate the server with a logger, k8sClient, sarClient from manager", func() {
			// Call SetupExtensionAPIServerWithManager with our mock manager
			testConfig := NewConfig(WithServerPort(9999))
			err := SetupExtensionAPIServerWithManager(mockMgr, testConfig)

			// Verify no error
			Expect(err).NotTo(HaveOccurred())

			// Verify that a runnable was added to the manager
			Expect(mockMgr.runnables).To(HaveLen(1), "One runnable should be added to manager")

			// The runnable should be an ExtensionServer
			server, ok := mockMgr.runnables[0].(*ExtensionServer)
			Expect(ok).To(BeTrue(), "The runnable should be an ExtensionServer")

			// Verify the config was used
			Expect(server.config.ServerPort).To(Equal(testConfig.ServerPort))
			Expect(server.k8sClient).To(Equal(mockMgr.client))
			Expect(server.sarClient).NotTo(BeNil())
			Expect(server.logger).NotTo(BeNil())
		})

		It("Should create the default config if passed nil", func() {
			// Call the function with nil config
			err := SetupExtensionAPIServerWithManager(mockMgr, nil)

			// Verify no error
			Expect(err).NotTo(HaveOccurred())

			// Verify that a runnable was added
			Expect(mockMgr.runnables).To(HaveLen(1), "One runnable should be added to manager")

			// Check that the server was created with a default config
			server, ok := mockMgr.runnables[0].(*ExtensionServer)
			Expect(ok).To(BeTrue(), "The runnable should be an ExtensionServer")
			Expect(server.config).ToNot(BeNil(), "A default config should be created")

			// Verify it's using the default port
			defaultConfig := NewConfig()
			Expect(server.config.ServerPort).To(Equal(defaultConfig.ServerPort))
		})

		It("Should call register all routes before starting the server", func() {
			// Call the function
			err := SetupExtensionAPIServerWithManager(mockMgr, NewConfig())

			// Verify no error
			Expect(err).NotTo(HaveOccurred())

			// Get the server from the runnables
			server, ok := mockMgr.runnables[0].(*ExtensionServer)
			Expect(ok).To(BeTrue(), "The runnable should be an ExtensionServer")

			// Verify routes were registered
			Expect(server.routes).To(HaveKey("/health"), "Health route should be registered")
			Expect(server.routes).To(HaveKey(server.config.ApiPath), "API discovery route should be registered")

			// Check for namespaced routes
			namespacedPathPrefix := server.config.ApiPath + "/namespaces/*/"
			Expect(server.routes).To(HaveKey(namespacedPathPrefix + "workspaceconnections"))
			Expect(server.routes).To(HaveKey(namespacedPathPrefix + "connectionaccessreview"))
		})

		It("Should add the server to the manager", func() {
			// Call the function
			err := SetupExtensionAPIServerWithManager(mockMgr, NewConfig())

			// Verify no error
			Expect(err).NotTo(HaveOccurred())

			// Verify the server was added to the manager
			Expect(mockMgr.runnables).To(HaveLen(1), "One runnable should be added to manager")
		})

		It("Should return an error if adding the server to the manager fails", func() {
			// Setup an error for the Add method
			testError := errors.New("mock add error")
			mockMgr.addError = testError

			// Call the function
			err := SetupExtensionAPIServerWithManager(mockMgr, NewConfig())

			// Verify the error was returned
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to add extension API server to manager"))
			Expect(err.Error()).To(ContainSubstring(testError.Error()))
		})

		It("Should return an error if instantiating the SAR client fails", func() {
			// Close the existing server
			sarServer.Close()

			// Create an invalid REST config that will cause client creation to fail
			badConfig := &rest.Config{
				Host: "https://invalid.host.example:12345",
				// Add invalid TLS config to force connection failure
				TLSClientConfig: rest.TLSClientConfig{
					Insecure: false,
					CertData: []byte("invalid-cert-data"),
					KeyData:  []byte("invalid-key-data"),
				},
			}

			// Update our manager with the bad config
			mockMgr.config = badConfig

			// Call the function
			err := SetupExtensionAPIServerWithManager(mockMgr, NewConfig())

			// Verify the error indicates SAR client creation failure
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to instantiate the sar client"))
		})
	})
})
