/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// EndpointIdleResponse represents the response from /api/idle endpoint
type EndpointIdleResponse struct {
	LastActivity string `json:"lastActiveTimestamp"`
}

// PodExecInterface defines the interface for pod execution
type PodExecInterface interface {
	ExecInPod(ctx context.Context, pod *corev1.Pod, containerName string, cmd []string, stdin string) (string, error)
}

// IdleDetector interface for different detection methods
type IdleDetector interface {
	CheckIdle(ctx context.Context, workspaceName string, pod *corev1.Pod, idleConfig *workspacev1alpha1.IdleShutdownSpec) (*IdleCheckResult, error)
}

func createIdleDetectorImpl(detection *workspacev1alpha1.IdleDetectionSpec) (IdleDetector, error) {
	switch {
	case detection.HTTPGet != nil:
		return NewHTTPGetDetector(), nil
	default:
		return nil, fmt.Errorf("no detection method configured")
	}
}

// CreateIdleDetector factory function variable (allows for unit testing)
var CreateIdleDetector = createIdleDetectorImpl

// HTTPGetDetector implements HTTP endpoint checking
type HTTPGetDetector struct {
	execUtil PodExecInterface
}

// NewHTTPGetDetectorWithExec creates a new HTTPGetDetector with the provided PodExecInterface
func NewHTTPGetDetectorWithExec(execUtil PodExecInterface) *HTTPGetDetector {
	return &HTTPGetDetector{
		execUtil: execUtil,
	}
}

// NewHTTPGetDetector creates a new HTTPGetDetector with a real PodExecUtil
func NewHTTPGetDetector() *HTTPGetDetector {
	execUtil, err := NewPodExecUtil()
	if err != nil {
		// In production, this should not happen if k8s config is available
		panic(fmt.Sprintf("Failed to create pod exec util: %v", err))
	}
	return NewHTTPGetDetectorWithExec(execUtil)
}

// CheckIdle implements the IdleDetector interface for HTTP endpoint checking
func (h *HTTPGetDetector) CheckIdle(ctx context.Context, workspaceName string, pod *corev1.Pod, idleConfig *workspacev1alpha1.IdleShutdownSpec) (*IdleCheckResult, error) {
	logger := logf.FromContext(ctx).WithValues("pod", pod.Name)

	// Get HTTP config from resolved idle config
	httpGetConfig := idleConfig.Detection.HTTPGet
	if httpGetConfig == nil {
		return &IdleCheckResult{IsIdle: false, ShouldRetry: false}, fmt.Errorf("httpGet config is nil")
	}

	// Build URL with scheme support
	scheme := string(httpGetConfig.Scheme)
	if scheme == "" {
		scheme = "http"
	}
	port := httpGetConfig.Port.String()
	url := fmt.Sprintf("%s://localhost:%s%s", scheme, port, httpGetConfig.Path)

	// Single curl call with status code
	cmd := []string{"curl", "-s", "-w", "\\nHTTP Status: %{http_code}\\n", url}

	logger.V(1).Info("Calling idle endpoint", "port", port, "path", httpGetConfig.Path)

	output, err := h.execUtil.ExecInPod(ctx, pod, "", cmd, "")
	if err != nil {
		// Handle curl exit codes - connection refused (temporary failure)
		if strings.Contains(err.Error(), "exit code 7") {
			return &IdleCheckResult{IsIdle: false, ShouldRetry: true}, fmt.Errorf("connection refused")
		}
		return &IdleCheckResult{IsIdle: false, ShouldRetry: true}, fmt.Errorf("curl execution failed: %w", err)
	}

	// Parse output to separate response body and status code
	lines := strings.Split(output, "\n")
	var responseBody strings.Builder
	var statusCode string

	for _, line := range lines {
		if strings.HasPrefix(line, "HTTP Status: ") {
			statusCode = strings.TrimPrefix(line, "HTTP Status: ")
		} else if line != "" {
			if responseBody.Len() > 0 {
				responseBody.WriteString("\n")
			}
			responseBody.WriteString(line)
		}
	}

	switch statusCode {
	case "404":
		// 404 is a permanent failure - endpoint doesn't exist
		return &IdleCheckResult{IsIdle: false, ShouldRetry: false}, fmt.Errorf("endpoint not found")
	case "200":
		// Parse the JSON response
		var idleResp EndpointIdleResponse
		if err := json.Unmarshal([]byte(responseBody.String()), &idleResp); err != nil {
			logger.Error(err, "Failed to parse idle response", "output", responseBody.String())
			return &IdleCheckResult{IsIdle: false, ShouldRetry: true}, fmt.Errorf("failed to parse idle response: %w", err)
		}

		// Validate the response
		if idleResp.LastActivity == "" {
			logger.Error(nil, "Empty lastActiveTimestamp in response", "output", responseBody.String())
			return &IdleCheckResult{IsIdle: false, ShouldRetry: true}, fmt.Errorf("invalid idle response: empty lastActiveTimestamp")
		}

		// Check if workspace is idle based on timeout
		isIdle := h.checkIdleTimeout(ctx, workspaceName, &idleResp, idleConfig)
		logger.V(1).Info("Successfully retrieved idle status", "lastActivity", idleResp.LastActivity, "isIdle", isIdle)
		return &IdleCheckResult{IsIdle: isIdle, ShouldRetry: true}, nil
	default:
		// treat other HTTP errors as retryable
		return &IdleCheckResult{IsIdle: false, ShouldRetry: true}, fmt.Errorf("unexpected HTTP status: %s", statusCode)
	}
}

// checkIdleTimeout checks if workspace should be stopped due to idle timeout
func (h *HTTPGetDetector) checkIdleTimeout(ctx context.Context, workspaceName string, idleResp *EndpointIdleResponse, idleConfig *workspacev1alpha1.IdleShutdownSpec) bool {
	logger := logf.FromContext(ctx).WithValues("workspace", workspaceName)

	// Parse last activity time with case-insensitive timezone
	// Some Jupyter servers return lowercase 'z' instead of uppercase 'Z' for UTC timezone
	// RFC3339 requires uppercase 'Z', so we normalize it here
	lastActivityStr := strings.ToUpper(idleResp.LastActivity) // Convert 'z' to 'Z'
	lastActivity, err := time.Parse(time.RFC3339, lastActivityStr)
	if err != nil {
		logger.Error(err, "Failed to parse last activity time", "lastActivity", idleResp.LastActivity)
		return false
	}

	timeout := time.Duration(idleConfig.IdleTimeoutInMinutes) * time.Minute
	idleTime := time.Since(lastActivity)

	if idleTime > timeout {
		logger.Info("Idle timeout reached", "idleTime", idleTime, "timeout", timeout, "lastActivity", lastActivity)
		return true
	}

	logger.V(1).Info("Workspace still active, timeout not reached",
		"idleTime", idleTime,
		"timeout", timeout,
		"remaining", timeout-idleTime,
		"lastActivity", lastActivity)
	return false
}
