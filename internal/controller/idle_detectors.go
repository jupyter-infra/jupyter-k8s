/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/pluginadapters"
	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	defaultResponseBodyPath = "lastActiveTimestamp"
	defaultTimestampFormat  = "RFC3339"
	transportPodExec        = "podExec"
	transportNetwork        = "network"
)

// IdleDetector interface for different detection methods.
// host is the target address (service ClusterIP for network, pod IP for podExec).
type IdleDetector interface {
	CheckIdle(ctx context.Context, workspace *workspacev1alpha1.Workspace, host string, idleConfig *workspacev1alpha1.IdleShutdownSpec) (*IdleCheckResult, error)
}

func createIdleDetectorImpl(detection *workspacev1alpha1.IdleDetectionSpec, httpClient *http.Client) (IdleDetector, error) {
	if detection.HTTPGet == nil {
		return nil, fmt.Errorf("no detection method configured")
	}
	return NewNetworkHTTPGetDetectorWithClient(httpClient), nil
}

// CreateIdleDetector is the factory used by the idle checker to instantiate a detector
// for the network transport path. The httpClient is owned by the WorkspaceIdleChecker
// and shared across checks so connections are pooled. It is a package-level var so that
// unit tests can override it with a mock detector without requiring a real HTTP server.
var CreateIdleDetector = createIdleDetectorImpl

// --- podExec transport (curl-in-pod) ---

// PodExecHTTPGetDetector implements HTTP endpoint checking via pod exec (curl)
type PodExecHTTPGetDetector struct {
	execUtil pluginadapters.PodExecInterface
	pod      *corev1.Pod
}

// NewPodExecHTTPGetDetectorWithExec creates a new PodExecHTTPGetDetector with the provided pluginadapters.PodExecInterface
func NewPodExecHTTPGetDetectorWithExec(execUtil pluginadapters.PodExecInterface, pod *corev1.Pod) *PodExecHTTPGetDetector {
	return &PodExecHTTPGetDetector{
		execUtil: execUtil,
		pod:      pod,
	}
}

// NewPodExecHTTPGetDetectorForPod creates a new PodExecHTTPGetDetector with a real PodExecUtil for the given pod
func NewPodExecHTTPGetDetectorForPod(pod *corev1.Pod) *PodExecHTTPGetDetector {
	execUtil, err := NewPodExecUtil()
	if err != nil {
		panic(fmt.Sprintf("Failed to create pod exec util: %v", err))
	}
	return NewPodExecHTTPGetDetectorWithExec(execUtil, pod)
}

// CheckIdle implements the IdleDetector interface for HTTP endpoint checking via pod exec.
// host is the pod IP (used to find the pod for exec).
func (h *PodExecHTTPGetDetector) CheckIdle(ctx context.Context, workspace *workspacev1alpha1.Workspace, host string, idleConfig *workspacev1alpha1.IdleShutdownSpec) (*IdleCheckResult, error) {
	logger := logf.FromContext(ctx).WithValues("workspace", workspace.Name)

	httpGetConfig := idleConfig.Detection.HTTPGet
	if httpGetConfig == nil {
		return &IdleCheckResult{IsIdle: false, ShouldRetry: false}, fmt.Errorf("httpGet config is nil")
	}

	fullPath := resolveIdlePath(workspace.Status.ApplicationBasePath, httpGetConfig.Path)
	probeURL := buildIdleProbeURL(httpGetConfig, "localhost", fullPath)

	cmd := []string{"curl", "-s", "-w", "\\nHTTP Status: %{http_code}\\n", probeURL}

	logger.V(1).Info("Calling idle endpoint via podExec", "url", probeURL)

	const workspaceContainerName = "workspace"
	output, err := h.execUtil.ExecInPod(ctx, h.pod, workspaceContainerName, cmd, "")
	if err != nil {
		// curl exit code 7 = connection refused (temporary failure)
		if strings.Contains(err.Error(), "exit code 7") {
			return &IdleCheckResult{IsIdle: false, ShouldRetry: true}, fmt.Errorf("connection refused")
		}
		return &IdleCheckResult{IsIdle: false, ShouldRetry: true}, fmt.Errorf("curl execution failed: %w", err)
	}

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

	return h.handleHTTPResponse(ctx, workspace.Name, statusCode, responseBody.String(), httpGetConfig, idleConfig)
}

// --- network transport (direct HTTP from operator) ---

// NetworkHTTPGetDetector implements HTTP endpoint checking via direct network call
type NetworkHTTPGetDetector struct {
	httpClient *http.Client
}

// NewNetworkHTTPGetDetectorWithClient creates a NetworkHTTPGetDetector that reuses the
// provided http.Client. The client is owned by the WorkspaceIdleChecker and shared across
// checks so TCP/TLS connections are pooled rather than re-established every cycle.
func NewNetworkHTTPGetDetectorWithClient(client *http.Client) *NetworkHTTPGetDetector {
	return &NetworkHTTPGetDetector{httpClient: client}
}

// CheckIdle implements the IdleDetector interface via direct HTTP call to the service ClusterIP.
func (n *NetworkHTTPGetDetector) CheckIdle(ctx context.Context, workspace *workspacev1alpha1.Workspace, host string, idleConfig *workspacev1alpha1.IdleShutdownSpec) (*IdleCheckResult, error) {
	logger := logf.FromContext(ctx).WithValues("workspace", workspace.Name)

	httpGetConfig := idleConfig.Detection.HTTPGet
	if httpGetConfig == nil {
		return &IdleCheckResult{IsIdle: false, ShouldRetry: false}, fmt.Errorf("httpGet config is nil")
	}

	fullPath := resolveIdlePath(workspace.Status.ApplicationBasePath, httpGetConfig.Path)
	probeURL := buildIdleProbeURL(httpGetConfig, host, fullPath)

	logger.V(1).Info("Calling idle endpoint via network", "url", probeURL)

	reqCtx, cancel := context.WithTimeout(ctx, IdleProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, probeURL, nil)
	if err != nil {
		return &IdleCheckResult{IsIdle: false, ShouldRetry: false}, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return &IdleCheckResult{IsIdle: false, ShouldRetry: true}, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return &IdleCheckResult{IsIdle: false, ShouldRetry: true}, fmt.Errorf("failed to read response body: %w", err)
	}

	statusCode := strconv.Itoa(resp.StatusCode)
	return handleHTTPResponseCommon(ctx, workspace.Name, statusCode, string(body), httpGetConfig, idleConfig)
}

// --- shared response handling ---

func (h *PodExecHTTPGetDetector) handleHTTPResponse(ctx context.Context, workspaceName, statusCode, body string, httpGetConfig *workspacev1alpha1.IdleHTTPGetAction, idleConfig *workspacev1alpha1.IdleShutdownSpec) (*IdleCheckResult, error) {
	return handleHTTPResponseCommon(ctx, workspaceName, statusCode, body, httpGetConfig, idleConfig)
}

func handleHTTPResponseCommon(ctx context.Context, workspaceName, statusCode, body string, httpGetConfig *workspacev1alpha1.IdleHTTPGetAction, idleConfig *workspacev1alpha1.IdleShutdownSpec) (*IdleCheckResult, error) {
	logger := logf.FromContext(ctx)

	switch statusCode {
	case "404":
		// Permanent failure — endpoint doesn't exist, no point retrying
		return &IdleCheckResult{IsIdle: false, ShouldRetry: false}, fmt.Errorf("endpoint not found")
	case "200":
		fieldPath := defaultResponseBodyPath
		format := defaultTimestampFormat
		if httpGetConfig.LastActivityTimestamp != nil {
			if httpGetConfig.LastActivityTimestamp.ResponseBodyPath != "" {
				fieldPath = httpGetConfig.LastActivityTimestamp.ResponseBodyPath
			}
			if httpGetConfig.LastActivityTimestamp.Format != "" {
				format = httpGetConfig.LastActivityTimestamp.Format
			}
		}

		timestampStr, err := extractJSONField(body, fieldPath)
		if err != nil {
			logger.Error(err, "Failed to extract timestamp from response", "fieldPath", fieldPath, "body", body)
			return &IdleCheckResult{IsIdle: false, ShouldRetry: true}, fmt.Errorf("failed to extract timestamp field %q: %w", fieldPath, err)
		}

		if timestampStr == "" {
			logger.Error(nil, "Empty timestamp value in response", "fieldPath", fieldPath)
			return &IdleCheckResult{IsIdle: false, ShouldRetry: true}, fmt.Errorf("empty timestamp value at %q", fieldPath)
		}

		lastActivity, err := parseTimestamp(timestampStr, format)
		if err != nil {
			logger.Error(err, "Failed to parse timestamp", "value", timestampStr, "format", format)
			return &IdleCheckResult{IsIdle: false, ShouldRetry: true}, fmt.Errorf("failed to parse timestamp: %w", err)
		}

		isIdle := checkIdleTimeout(ctx, workspaceName, lastActivity, idleConfig)
		logger.V(1).Info("Successfully checked idle status", "lastActivity", lastActivity, "isIdle", isIdle)
		return &IdleCheckResult{IsIdle: isIdle, ShouldRetry: true}, nil
	default:
		// Treat other HTTP errors (5xx, etc.) as retryable
		return &IdleCheckResult{IsIdle: false, ShouldRetry: true}, fmt.Errorf("unexpected HTTP status: %s", statusCode)
	}
}

// extractJSONField extracts a value from a JSON body using a dot-separated path.
func extractJSONField(body, path string) (string, error) {
	var data any
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		return "", fmt.Errorf("invalid JSON: %w", err)
	}

	parts := strings.Split(path, ".")
	current := data
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return "", fmt.Errorf("field %q: expected object, got %T", part, current)
		}
		current, ok = m[part]
		if !ok {
			return "", fmt.Errorf("field %q not found", part)
		}
	}

	switch v := current.(type) {
	case string:
		return v, nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

// parseTimestamp parses a timestamp string according to the specified format.
func parseTimestamp(value, format string) (time.Time, error) {
	switch format {
	case "unix":
		epoch, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid unix timestamp %q: %w", value, err)
		}
		sec := int64(epoch)
		nsec := int64((epoch - float64(sec)) * 1e9)
		return time.Unix(sec, nsec), nil
	default:
		// Some Jupyter servers return lowercase 'z' instead of uppercase 'Z' for UTC.
		// RFC3339 requires uppercase, so normalize before parsing.
		normalized := strings.ToUpper(value)
		t, err := time.Parse(time.RFC3339, normalized)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid RFC3339 timestamp %q: %w", value, err)
		}
		return t, nil
	}
}

// resolveIdlePath joins the application base path with the httpGet path.
func resolveIdlePath(basePath, httpGetPath string) string {
	if basePath == "" || basePath == "/" {
		return httpGetPath
	}
	return strings.TrimRight(basePath, "/") + "/" + strings.TrimLeft(httpGetPath, "/")
}

// buildIdleProbeURL assembles the probe URL from components rather than string
// concatenation. Building via url.URL keeps the workspace-controlled path confined
// to the URL path: a value like "@evil-host/x" cannot collapse the host into userinfo
// and steer the probe off the pinned host, and a path missing its leading slash
// ("api/status") still yields a well-formed URL instead of an unparseable one.
// net.JoinHostPort also produces correct bracketing for IPv6 hosts.
func buildIdleProbeURL(httpGetConfig *workspacev1alpha1.IdleHTTPGetAction, host, fullPath string) string {
	scheme := strings.ToLower(string(httpGetConfig.Scheme))
	if scheme == "" {
		scheme = "http"
	}
	if !strings.HasPrefix(fullPath, "/") {
		fullPath = "/" + fullPath
	}
	u := url.URL{
		Scheme: scheme,
		Host:   net.JoinHostPort(host, httpGetConfig.Port.String()),
		Path:   fullPath,
	}
	return u.String()
}

// checkIdleTimeout checks if workspace should be stopped due to idle timeout
func checkIdleTimeout(ctx context.Context, workspaceName string, lastActivity time.Time, idleConfig *workspacev1alpha1.IdleShutdownSpec) bool {
	logger := logf.FromContext(ctx).WithValues("workspace", workspaceName)

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
