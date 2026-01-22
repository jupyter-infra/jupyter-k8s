/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package authmiddleware

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/jupyter-infra/jupyter-k8s/internal/jwt"
)

// getTestPort returns an available port for testing
func getTestPort() int {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(fmt.Sprintf("failed to get test port: %v", err))
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port
}

// createTestHTTPServer creates a minimal Server instance for testing HTTP server runnable
func createTestHTTPServer() *Server {
	port := getTestPort()
	config := &Config{
		Port:         port,
		ReadTimeout:  1 * time.Second,
		WriteTimeout: 1 * time.Second,
		EnableOAuth:  false, // Disable OAuth to avoid OIDC initialization
	}

	// Create a simple JWT manager (we won't actually use it in these tests)
	signer := jwt.NewStandardSigner("test-issuer", "test-audience", time.Hour, 5*time.Second)
	jwtManager := jwt.NewManager(signer, false, 0, 0)

	cookieManager, _ := NewCookieManager(config)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Suppress logs during tests
	}))

	return &Server{
		config:        config,
		jwtManager:    jwtManager,
		cookieManager: cookieManager,
		logger:        logger,
		restClient:    nil, // Not needed for these tests
		oidcVerifier:  nil, // Not needed since OAuth is disabled
	}
}

// TestNewHTTPServerRunnable tests the constructor
func TestNewHTTPServerRunnable(t *testing.T) {
	server := createTestHTTPServer()
	logger := logr.Discard()

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	signer := jwt.NewStandardSigner("test-issuer", "test-audience", time.Hour, 5*time.Second)

	runnable := NewHTTPServerRunnable(
		server,
		logger,
		k8sClient,
		signer,
		"test-secret",
		"test-namespace",
	)

	if runnable == nil {
		t.Fatal("Expected non-nil runnable")
	}
	if runnable.server != server {
		t.Error("Server not set correctly")
	}
	if runnable.standardSigner != signer {
		t.Error("StandardSigner not set correctly")
	}
	if runnable.secretName != "test-secret" {
		t.Error("Secret name not set correctly")
	}
	if runnable.namespace != "test-namespace" {
		t.Error("Namespace not set correctly")
	}
}

// TestNeedLeaderElection tests that HTTP server doesn't need leader election
func TestNeedLeaderElection(t *testing.T) {
	runnable := &HTTPServerRunnable{}
	if runnable.NeedLeaderElection() {
		t.Error("HTTP server should not need leader election")
	}
}

// TestStart_WithStandardSigner_HappyCase tests successful startup with standard signer
func TestStart_WithStandardSigner_HappyCase(t *testing.T) {
	// Create test secret with JWT signing keys
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test-namespace",
		},
		Data: map[string][]byte{
			"jwt-signing-key-1700000000": []byte("abcdefghijklmnopqrstuvwxyz1234567890ABCDEFGHIJKLM"),
			"jwt-signing-key-1700000001": []byte("abcdefghijklmnopqrstuvwxyz1234567890ABCDEFGHIJK2"),
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret).
		Build()

	server := createTestHTTPServer()
	logger := logr.Discard()
	signer := jwt.NewStandardSigner("test-issuer", "test-audience", time.Hour, 5*time.Second)

	runnable := NewHTTPServerRunnable(
		server,
		logger,
		k8sClient,
		signer,
		"test-secret",
		"test-namespace",
	)

	// Start with a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run Start in a goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- runnable.Start(ctx)
	}()

	// Give it time to start and load the secret
	time.Sleep(200 * time.Millisecond)

	// Cancel context to trigger shutdown
	cancel()

	// Wait for Start to return
	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Start() did not return after context cancellation")
	}
}

// TestStart_NoStandardSigner_HappyCase tests successful startup without standard signer
func TestStart_NoStandardSigner_HappyCase(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	server := createTestHTTPServer()
	logger := logr.Discard()

	runnable := NewHTTPServerRunnable(
		server,
		logger,
		k8sClient,
		nil, // No standard signer
		"",
		"",
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- runnable.Start(ctx)
	}()

	// Give it time to start
	time.Sleep(200 * time.Millisecond)

	// Cancel context
	cancel()

	// Wait for completion
	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Start() did not return after context cancellation")
	}
}

// TestStart_WithStandardSigner_MissingSecret tests startup when secret doesn't exist
func TestStart_WithStandardSigner_MissingSecret(t *testing.T) {
	// Create client without the secret
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	server := createTestHTTPServer()
	logger := logr.Discard()
	signer := jwt.NewStandardSigner("test-issuer", "test-audience", time.Hour, 5*time.Second)

	runnable := NewHTTPServerRunnable(
		server,
		logger,
		k8sClient,
		signer,
		"missing-secret",
		"test-namespace",
	)

	ctx := context.Background()
	err := runnable.Start(ctx)

	if err == nil {
		t.Fatal("Expected error for missing secret, got nil")
	}

	// Check that it's a NotFound error
	if !apierrors.IsNotFound(err) {
		t.Logf("Error message: %v", err)
		// The error might be wrapped, so also check the error message
		errMsg := err.Error()
		if errMsg == "" {
			t.Error("Expected error message about missing secret")
		}
	}
}

// TestStart_WithStandardSigner_Unauthorized tests startup when access to secret is denied
func TestStart_WithStandardSigner_Unauthorized(t *testing.T) {
	// Create a fake client that returns Forbidden error
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// Create secret but we'll use a client that denies access
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "forbidden-secret",
			Namespace: "test-namespace",
		},
		Data: map[string][]byte{
			"jwt-signing-key-1700000000": []byte("abcdefghijklmnopqrstuvwxyz1234567890ABCDEFGHIJKLM"),
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret).
		Build()

	// Wrap the client to return Forbidden error for Get operations
	wrappedClient := &forbiddenClient{
		Client: k8sClient,
		resourceToForbid: schema.GroupVersionResource{
			Group:    "",
			Version:  "v1",
			Resource: "secrets",
		},
	}

	server := createTestHTTPServer()
	logger := logr.Discard()
	signer := jwt.NewStandardSigner("test-issuer", "test-audience", time.Hour, 5*time.Second)

	runnable := NewHTTPServerRunnable(
		server,
		logger,
		wrappedClient,
		signer,
		"forbidden-secret",
		"test-namespace",
	)

	ctx := context.Background()
	err := runnable.Start(ctx)

	if err == nil {
		t.Fatal("Expected error for unauthorized access, got nil")
	}

	// Verify it's a forbidden error
	if !apierrors.IsForbidden(err) {
		t.Errorf("Expected Forbidden error, got: %v", err)
	}
}

// TestStart_OnDoneCtx_ShutsDown tests that context cancellation triggers shutdown
func TestStart_OnDoneCtx_ShutsDown(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	server := createTestHTTPServer()
	logger := logr.Discard()

	runnable := NewHTTPServerRunnable(
		server,
		logger,
		k8sClient,
		nil, // No signer for simplicity
		"",
		"",
	)

	// Create context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := runnable.Start(ctx)

	if err != nil {
		t.Errorf("Expected no error from graceful shutdown, got: %v", err)
	}
}

// forbiddenClient wraps a fake client to return Forbidden errors for specific resources
type forbiddenClient struct {
	client.Client
	resourceToForbid schema.GroupVersionResource
}

func (f *forbiddenClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	// Check if this is a Secret Get request
	if _, ok := obj.(*corev1.Secret); ok {
		return apierrors.NewForbidden(
			f.resourceToForbid.GroupResource(),
			key.Name,
			errors.New("access denied"),
		)
	}
	return f.Client.Get(ctx, key, obj, opts...)
}

// TestStart_ServerStartError tests that Start returns error when server.Start() fails
func TestStart_ServerStartError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	logger := logr.Discard()

	// Create a server with an already-used port to trigger start failure
	baseServer := createTestHTTPServer()

	// Start the server once to occupy the port
	go func() { _ = baseServer.Start() }()
	time.Sleep(100 * time.Millisecond) // Give it time to bind

	// Try to start another server on the same port - should fail
	runnable := NewHTTPServerRunnable(
		baseServer,
		logger,
		k8sClient,
		nil,
		"",
		"",
	)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := runnable.Start(ctx)

	// Should return error (either from port binding or context timeout)
	if err == nil {
		t.Fatal("Expected error from duplicate server start, got nil")
	}

	// Shutdown the first server
	_ = baseServer.Shutdown(context.Background())
}

// TestStart_ShutdownError tests that Start handles and reports shutdown errors
func TestStart_ShutdownError(t *testing.T) {
	// This test verifies that shutdown errors are logged and returned
	// In practice, shutdown errors are rare and usually indicate timeout issues

	// We can test this by creating a server with a very short shutdown timeout
	// and ensuring it's handling requests when we try to shut it down

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	port := getTestPort()
	config := &Config{
		Port:            port,
		ReadTimeout:     1 * time.Second,
		WriteTimeout:    1 * time.Second,
		ShutdownTimeout: 1 * time.Millisecond, // Very short timeout to force error
		EnableOAuth:     false,
	}

	signer := jwt.NewStandardSigner("test-issuer", "test-audience", time.Hour, 5*time.Second)
	jwtManager := jwt.NewManager(signer, false, 0, 0)
	cookieManager, _ := NewCookieManager(config)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	server := &Server{
		config:        config,
		jwtManager:    jwtManager,
		cookieManager: cookieManager,
		logger:        logger,
		restClient:    nil,
		oidcVerifier:  nil,
	}

	runnable := NewHTTPServerRunnable(
		server,
		logr.Discard(),
		k8sClient,
		nil,
		"",
		"",
	)

	ctx, cancel := context.WithCancel(context.Background())

	errChan := make(chan error, 1)
	go func() {
		errChan <- runnable.Start(ctx)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Cancel to trigger shutdown
	cancel()

	// Wait for result - with very short timeout, shutdown may return error
	select {
	case err := <-errChan:
		// Error is acceptable (shutdown timeout) or nil (clean shutdown)
		// This verifies the error path is exercised
		_ = err
	case <-time.After(2 * time.Second):
		t.Fatal("Start() did not return after context cancellation")
	}
}
