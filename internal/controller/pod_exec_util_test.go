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

package controller

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

func TestNewPodExecUtil_Success(t *testing.T) {
	// Save originals
	originalGetConfig := getConfig
	originalNewClientset := newClientset
	defer func() {
		getConfig = originalGetConfig
		newClientset = originalNewClientset
	}()

	// Mock successful calls
	mockConfig := &rest.Config{Host: "https://test-cluster"}
	getConfig = func() (*rest.Config, error) {
		return mockConfig, nil
	}
	newClientset = func(cfg *rest.Config) (*kubernetes.Clientset, error) {
		// For testing, we'll create a clientset with the provided config
		// This will work as long as the config is valid
		return &kubernetes.Clientset{}, nil
	}

	util, err := NewPodExecUtil()

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if util == nil {
		t.Fatal("Expected non-nil PodExecUtil")
		return
	}
	if util.clientset == nil {
		t.Error("Expected non-nil clientset")
	}
	if util.config == nil {
		t.Error("Expected non-nil config")
		return
	}
	if util.config.Host != "https://test-cluster" {
		t.Errorf("Expected config host 'https://test-cluster', got: %s", util.config.Host)
	}
}

func TestNewPodExecUtil_ConfigError(t *testing.T) {
	// Save original
	original := getConfig
	defer func() { getConfig = original }()

	// Mock config error
	getConfig = func() (*rest.Config, error) {
		return nil, errors.New("mock config error")
	}

	util, err := NewPodExecUtil()

	if err == nil {
		t.Fatal("Expected error when config fails")
	}
	if util != nil {
		t.Error("Expected nil PodExecUtil when config fails")
	}
	if !strings.Contains(err.Error(), "failed to get Kubernetes config") {
		t.Errorf("Expected error to contain 'failed to get Kubernetes config', got: %v", err)
	}
}

func TestNewPodExecUtil_ClientsetError(t *testing.T) {
	// Save originals
	originalGetConfig := getConfig
	originalNewClientset := newClientset
	defer func() {
		getConfig = originalGetConfig
		newClientset = originalNewClientset
	}()

	// Mock successful config, failing clientset
	getConfig = func() (*rest.Config, error) {
		return &rest.Config{}, nil
	}
	newClientset = func(*rest.Config) (*kubernetes.Clientset, error) {
		return nil, errors.New("mock clientset error")
	}

	util, err := NewPodExecUtil()

	if err == nil {
		t.Fatal("Expected error when clientset creation fails")
	}
	if util != nil {
		t.Error("Expected nil PodExecUtil when clientset creation fails")
	}
	if !strings.Contains(err.Error(), "failed to create Kubernetes clientset") {
		t.Errorf("Expected error to contain 'failed to create Kubernetes clientset', got: %v", err)
	}
}

// Mock executor for testing
type mockExecutor struct {
	streamErr error
	stdout    string
	stderr    string
}

// Mock executor that captures StreamOptions
type mockExecutorWithCapture struct {
	streamErr         error
	stdout            string
	stderr            string
	capturedStdin     *bool
	capturedStdinData *string
}

func (m *mockExecutor) Stream(options remotecommand.StreamOptions) error {
	return m.StreamWithContext(context.Background(), options)
}

func (m *mockExecutor) StreamWithContext(ctx context.Context, options remotecommand.StreamOptions) error {
	// Write mock output to provided streams first (even if there will be an error)
	if options.Stdout != nil && m.stdout != "" {
		_, _ = options.Stdout.Write([]byte(m.stdout))
	}
	if options.Stderr != nil && m.stderr != "" {
		_, _ = options.Stderr.Write([]byte(m.stderr))
	}

	// Then return error if configured
	if m.streamErr != nil {
		return m.streamErr
	}

	return nil
}

func (m *mockExecutorWithCapture) Stream(options remotecommand.StreamOptions) error {
	return m.StreamWithContext(context.Background(), options)
}

func (m *mockExecutorWithCapture) StreamWithContext(ctx context.Context, options remotecommand.StreamOptions) error {
	// Capture whether stdin was provided
	if m.capturedStdin != nil {
		*m.capturedStdin = options.Stdin != nil
	}

	// Capture actual stdin data
	if options.Stdin != nil && m.capturedStdinData != nil {
		buf := make([]byte, 1024)
		n, _ := options.Stdin.Read(buf)
		*m.capturedStdinData = string(buf[:n])
	}

	// Write mock output to provided streams
	if options.Stdout != nil && m.stdout != "" {
		_, _ = options.Stdout.Write([]byte(m.stdout))
	}
	if options.Stderr != nil && m.stderr != "" {
		_, _ = options.Stderr.Write([]byte(m.stderr))
	}

	return m.streamErr
}

func TestExecInPod_Success(t *testing.T) {
	// This test would require complex REST client mocking
	// For now, we'll test the integration with real kubeconfig if available
	util, err := NewPodExecUtil()
	if err != nil {
		t.Skipf("Skipping integration test - requires valid Kubernetes config: %v", err)
		return
	}

	// Save original
	original := newSPDYExecutor
	defer func() { newSPDYExecutor = original }()

	// Mock successful executor
	mockExec := &mockExecutor{
		stdout: "  command output  \n", // Test trimming
		stderr: "some stderr",
	}
	newSPDYExecutor = func(config *rest.Config, method string, url *url.URL) (remotecommand.Executor, error) {
		return mockExec, nil
	}

	// Create test pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
		},
	}

	// Test successful execution without stdin
	noStdin := ""
	output, err := util.ExecInPod(context.Background(), pod, "test-container", []string{"echo", "hello"}, noStdin)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if output != "command output" {
		t.Errorf("Expected 'command output', got: '%s'", output)
	}
}

func TestExecInPod_ExecutorCreationFailure(t *testing.T) {
	util, err := NewPodExecUtil()
	if err != nil {
		t.Skipf("Skipping integration test - requires valid Kubernetes config: %v", err)
		return
	}

	// Save original
	original := newSPDYExecutor
	defer func() { newSPDYExecutor = original }()

	// Mock executor creation failure
	newSPDYExecutor = func(config *rest.Config, method string, url *url.URL) (remotecommand.Executor, error) {
		return nil, errors.New("mock executor creation error")
	}

	// Create test pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
		},
	}

	// Test executor creation failure
	noStdin := ""
	output, err := util.ExecInPod(context.Background(), pod, "test-container", []string{"echo", "hello"}, noStdin)

	if err == nil {
		t.Fatal("Expected error when executor creation fails")
	}
	if !strings.Contains(err.Error(), "failed to create executor") {
		t.Errorf("Expected error to contain 'failed to create executor', got: %v", err)
	}
	if output != "" {
		t.Errorf("Expected empty output on error, got: '%s'", output)
	}
}

func TestExecInPod_StreamExecutionFailure(t *testing.T) {
	util, err := NewPodExecUtil()
	if err != nil {
		t.Skipf("Skipping integration test - requires valid Kubernetes config: %v", err)
		return
	}

	// Save original
	original := newSPDYExecutor
	defer func() { newSPDYExecutor = original }()

	// Mock executor that fails during stream
	mockExec := &mockExecutor{
		streamErr: errors.New("mock stream execution error"),
		stdout:    "partial output",
	}
	newSPDYExecutor = func(config *rest.Config, method string, url *url.URL) (remotecommand.Executor, error) {
		return mockExec, nil
	}

	// Create test pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
		},
	}

	// Test stream execution failure
	noStdin := ""
	output, err := util.ExecInPod(context.Background(), pod, "test-container", []string{"failing-command"}, noStdin)

	if err == nil {
		t.Fatal("Expected error when stream execution fails")
	}
	if !strings.Contains(err.Error(), "mock stream execution error") {
		t.Errorf("Expected error to contain 'mock stream execution error', got: %v", err)
	}
	// Should still return partial output even on error
	if output != "partial output" {
		t.Errorf("Expected 'partial output', got: '%s'", output)
	}
}

func TestExecInPod_WithStdin(t *testing.T) {
	util, err := NewPodExecUtil()
	if err != nil {
		t.Skipf("Skipping integration test - requires valid Kubernetes config: %v", err)
		return
	}

	// Save original
	original := newSPDYExecutor
	defer func() { newSPDYExecutor = original }()

	// Capture stdin data from StreamOptions
	var stdinProvided bool
	var stdinData string
	mockExec := &mockExecutorWithCapture{
		stdout:            "stdin processed",
		capturedStdin:     &stdinProvided,
		capturedStdinData: &stdinData,
	}
	newSPDYExecutor = func(config *rest.Config, method string, url *url.URL) (remotecommand.Executor, error) {
		return mockExec, nil
	}

	// Create test pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
		},
	}

	// Test execution with stdin
	inputData := "test-input\nsecond-line\n"
	output, err := util.ExecInPod(context.Background(), pod, "test-container", []string{"bash", "-c", "read line1 && read line2 && echo processed"}, inputData)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if output != "stdin processed" {
		t.Errorf("Expected 'stdin processed', got: '%s'", output)
	}
	if !stdinProvided {
		t.Error("Expected StreamOptions.Stdin to be provided when stdin data is given")
	}
	expectedData := "test-input\nsecond-line\n"
	if stdinData != expectedData {
		t.Errorf("Expected stdin data '%s', got '%s'", expectedData, stdinData)
	}
}
