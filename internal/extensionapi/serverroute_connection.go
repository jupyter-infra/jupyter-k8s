/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

// Package extensionapi provides extension API server functionality.
package extensionapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"text/template"

	"github.com/go-logr/logr"
	connectionv1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/connection/v1alpha1"
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/aws"
	"github.com/jupyter-infra/jupyter-k8s/internal/jwt"
	"github.com/jupyter-infra/jupyter-k8s/internal/workspace"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// Function variable for SSM strategy creation (mockable in tests)
var newSSMRemoteAccessStrategy = aws.NewSSMRemoteAccessStrategy

// noOpPodExec is a no-op implementation of PodExecInterface for connection URL generation
type noOpPodExec struct{}

func (n *noOpPodExec) ExecInPod(ctx context.Context, pod *corev1.Pod, containerName string, cmd []string, stdin string) (string, error) {
	return "", fmt.Errorf("pod exec not supported in connection URL generation")
}

// hasWebUIEnabled checks if WebUI is enabled by verifying if BearerAuthURLTemplate
// is defined in the access strategy.
func hasWebUIEnabled(accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) bool {
	if accessStrategy == nil {
		return false
	}
	return accessStrategy.Spec.BearerAuthURLTemplate != ""
}

// isWorkspaceAvailable checks if the workspace has the Available condition set to True.
func isWorkspaceAvailable(ws *workspacev1alpha1.Workspace) bool {
	for _, condition := range ws.Status.Conditions {
		if condition.Type == "Available" && condition.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

// validateWebUIConnection validates that a workspace is ready for WebUI connections.
// Returns (accessStrategy, statusCode, error).
func (s *ExtensionServer) validateWebUIConnection(ws *workspacev1alpha1.Workspace, logger logr.Logger) (*workspacev1alpha1.WorkspaceAccessStrategy, int, error) {
	// AccessStrategy should exist because the controller prevents deletion while workspaces reference it
	accessStrategy, err := s.getAccessStrategy(ws)
	if err != nil {
		logger.Error(err, "Failed to get access strategy for WebUI validation", "workspaceName", ws.Name)

		if errors.IsNotFound(err) {
			return nil, http.StatusNotFound, fmt.Errorf("access strategy not found")
		}
		return nil, http.StatusInternalServerError, fmt.Errorf("internal server error")
	}

	// Check 1: WebUI is enabled via BearerAuthURLTemplate
	if !hasWebUIEnabled(accessStrategy) {
		logger.Info("WebUI connection rejected: WebUI not enabled for workspace",
			"workspaceName", ws.Name,
			"namespace", ws.Namespace)
		return nil, http.StatusBadRequest, fmt.Errorf("web browser access is not enabled for this workspace")
	}

	// Check 2: Workspace is available
	if !isWorkspaceAvailable(ws) {
		logger.Info("WebUI connection rejected: workspace not available",
			"workspaceName", ws.Name,
			"namespace", ws.Namespace)
		return nil, http.StatusBadRequest, fmt.Errorf("workspace is not available. Check workspace status for details")
	}

	return accessStrategy, 0, nil
}

// hasSSMConfigured checks if SSM remote access is configured in the access strategy.
func hasSSMConfigured(accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) bool {
	if accessStrategy == nil {
		return false
	}
	if accessStrategy.Spec.CreateConnectionContext == nil {
		return false
	}
	return accessStrategy.Spec.CreateConnectionContext[aws.SSMDocumentNameKey] != ""
}

// validateVSCodeConnection validates that a workspace is ready for VSCode remote connections.
// Returns (accessStrategy, statusCode, error). If validation passes, returns (as, 0, nil).
func (s *ExtensionServer) validateVSCodeConnection(ws *workspacev1alpha1.Workspace) (*workspacev1alpha1.WorkspaceAccessStrategy, int, error) {
	logger := ctrl.Log.WithName("vscode-validation")

	// Workspace already provided from authZ check, no need to fetch

	if !isWorkspaceAvailable(ws) {
		logger.Info("VSCode connection rejected: workspace not available",
			"workspaceName", ws.Name,
			"namespace", ws.Namespace)
		return nil, http.StatusBadRequest, fmt.Errorf("workspace is not available. Check workspace status for details")
	}

	// AccessStrategy should exist because the controller prevents deletion while workspaces reference it
	accessStrategy, err := s.getAccessStrategy(ws)
	if err != nil {
		logger.Error(err, "Failed to get access strategy for VSCode validation", "workspaceName", ws.Name)

		if errors.IsNotFound(err) {
			return nil, http.StatusNotFound, fmt.Errorf("access strategy not found")
		}
		return nil, http.StatusInternalServerError, fmt.Errorf("internal server error")
	}

	if !hasSSMConfigured(accessStrategy) {
		logger.Info("VSCode connection rejected: SSM not configured",
			"workspaceName", ws.Name,
			"namespace", ws.Namespace)
		return nil, http.StatusBadRequest, fmt.Errorf("remote connection is not configured for this workspace")
	}

	return accessStrategy, 0, nil
}

// generateWebUIBearerTokenURL generates a Web UI connection URL with JWT token
// Returns (connectionType, connectionURL, error)
func (s *ExtensionServer) generateWebUIBearerTokenURL(r *http.Request, ws *workspacev1alpha1.Workspace, accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) (string, string, error) {
	user := GetUser(r)
	if user == "" {
		return "", "", fmt.Errorf("user information not found in request headers")
	}

	// Validate inputs
	if accessStrategy == nil {
		return "", "", fmt.Errorf("no AccessStrategy configured for workspace")
	}
	if accessStrategy.Spec.BearerAuthURLTemplate == "" {
		return "", "", fmt.Errorf("BearerAuthURLTemplate not configured in AccessStrategy")
	}

	// Create signer based on access strategy
	signer, err := s.signerFactory.CreateSigner(accessStrategy)
	if err != nil {
		return "", "", fmt.Errorf("failed to create signer: %w", err)
	}

	// Generate URL from template
	webUIURL, err := s.renderBearerAuthURL(accessStrategy.Spec.BearerAuthURLTemplate, ws, accessStrategy)
	if err != nil {
		return "", "", fmt.Errorf("failed to render bearer auth URL: %w", err)
	}

	// Parse URL to extract domain and path for JWT claims
	parsedURL, err := url.Parse(webUIURL)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse generated URL: %w", err)
	}

	domain := parsedURL.Host // Full host including subdomain
	path := parsedURL.Path   // Path part

	// Strip /bearer-auth from the end if present, as we don't want to include that in Claims
	if strings.HasSuffix(path, "/bearer-auth") {
		path = strings.TrimSuffix(path, "/bearer-auth")
		if path == "" {
			path = "/"
		}
	}

	// Generate JWT token with domain and path using access strategy-specific signer
	token, err := signer.GenerateToken(user, []string{}, user, map[string][]string{}, path, domain, jwt.TokenTypeBootstrap)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate JWT token: %w", err)
	}

	// Add token to URL
	finalURL := fmt.Sprintf("%s?token=%s", webUIURL, token)

	return connectionv1alpha1.ConnectionTypeWebUI, finalURL, nil
}

// HandleConnectionCreate handles POST requests to create a connection
func (s *ExtensionServer) HandleConnectionCreate(w http.ResponseWriter, r *http.Request) {
	logger := GetLoggerFromContext(r.Context())

	if r.Method != http.MethodPost {
		logger.Error(nil, "Invalid HTTP method", "method", r.Method)
		WriteKubernetesError(w, http.StatusBadRequest, "Connection must use POST method")
		return
	}

	// Extract namespace from URL path
	namespace, err := GetNamespaceFromPath(r.URL.Path)
	if err != nil {
		logger.Error(err, "Failed to extract namespace from URL path", "path", r.URL.Path)
		WriteKubernetesError(w, http.StatusBadRequest, "Invalid URL path")
		return
	}

	// Parse request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error(err, "Failed to read request body")
		WriteKubernetesError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	var req connectionv1alpha1.WorkspaceConnectionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		logger.Error(err, "Failed to parse JSON request body")
		WriteKubernetesError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Validate request
	if err := validateWorkspaceConnectionRequest(&req); err != nil {
		logger.Error(err, "Invalid workspace connection request")
		WriteKubernetesError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Check if CLUSTER_ID is configured early for VSCode connections
	if req.Spec.WorkspaceConnectionType == connectionv1alpha1.ConnectionTypeVSCodeRemote {
		if s.config.ClusterId == "" {
			logger.Error(nil, "CLUSTER_ID environment variable not configured")
			WriteKubernetesError(w, http.StatusBadRequest, "CLUSTER_ID not configured. Please set controllerManager.container.env.CLUSTER_ID in helm values")
			return
		}
		// Ensure AWS resources are initialized (only happens once, only when AWS is configured)
		if err := aws.EnsureResourcesInitialized(r.Context()); err != nil {
			WriteKubernetesError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to initialize resources: %v", err))
			return
		}
	}

	logger.Info("Creating workspace connection",
		"namespace", namespace,
		"workspaceName", req.Spec.WorkspaceName,
		"connectionType", req.Spec.WorkspaceConnectionType)

	// Check authorization for private workspaces
	ws, result, err := s.checkWorkspaceAuthorization(r, req.Spec.WorkspaceName, namespace)
	if err != nil {
		logger.Error(err, "Authorization failed", "workspaceName", req.Spec.WorkspaceName)
		WriteKubernetesError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if result.NotFound {
		WriteKubernetesError(w, http.StatusNotFound, result.Reason)
		return
	}

	if !result.Allowed {
		WriteKubernetesError(w, http.StatusForbidden, result.Reason)
		return
	}

	// Validate connection readiness and fetch access strategy
	var accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy

	switch req.Spec.WorkspaceConnectionType {
	case connectionv1alpha1.ConnectionTypeWebUI:
		var statusCode int
		accessStrategy, statusCode, err = s.validateWebUIConnection(ws, logger)
		if err != nil {
			WriteKubernetesError(w, statusCode, err.Error())
			return
		}
	case connectionv1alpha1.ConnectionTypeVSCodeRemote:
		var statusCode int
		accessStrategy, statusCode, err = s.validateVSCodeConnection(ws)
		if err != nil {
			WriteKubernetesError(w, statusCode, err.Error())
			return
		}
	}

	// Generate response based on connection type
	var responseType, responseURL string
	switch req.Spec.WorkspaceConnectionType {
	case connectionv1alpha1.ConnectionTypeVSCodeRemote:
		responseType, responseURL, err = s.generateVSCodeURL(r, ws, accessStrategy, namespace)
	case connectionv1alpha1.ConnectionTypeWebUI:
		responseType, responseURL, err = s.generateWebUIBearerTokenURL(r, ws, accessStrategy)
	default:
		logger.Error(nil, "Invalid workspace connection type", "connectionType", req.Spec.WorkspaceConnectionType)
		WriteKubernetesError(w, http.StatusBadRequest, "Invalid workspace connection type")
		return
	}

	if err != nil {
		logger.Error(err, "Failed to generate connection URL", "connectionType", req.Spec.WorkspaceConnectionType)
		WriteKubernetesError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Create response
	response := connectionv1alpha1.WorkspaceConnectionResponse{
		TypeMeta: metav1.TypeMeta{
			APIVersion: connectionv1alpha1.WorkspaceConnectionAPIVersion,
			Kind:       connectionv1alpha1.WorkspaceConnectionKind,
		},
		ObjectMeta: req.ObjectMeta,
		Spec:       req.Spec,
		Status: connectionv1alpha1.WorkspaceConnectionResponseStatus{
			WorkspaceConnectionType: responseType,
			WorkspaceConnectionURL:  responseURL,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.Error(err, "Failed to encode response")
	}
}

// validateWorkspaceConnectionRequest validates the workspace connection request
func validateWorkspaceConnectionRequest(req *connectionv1alpha1.WorkspaceConnectionRequest) error {
	if req.Spec.WorkspaceName == "" {
		return fmt.Errorf("workspaceName is required")
	}

	if req.Spec.WorkspaceConnectionType == "" {
		return fmt.Errorf("workspaceConnectionType is required")
	}

	// Validate connection type against enum
	switch req.Spec.WorkspaceConnectionType {
	case connectionv1alpha1.ConnectionTypeVSCodeRemote, connectionv1alpha1.ConnectionTypeWebUI:
		// Valid types
	default:
		return fmt.Errorf("invalid workspaceConnectionType: '%s'. Valid types are: 'vscode-remote', 'web-ui'", req.Spec.WorkspaceConnectionType)
	}

	return nil
}

// generateVSCodeURL generates a VSCode connection URL using SSM remote access strategy
// Returns (connectionType, connectionURL, error)
func (s *ExtensionServer) generateVSCodeURL(r *http.Request, ws *workspacev1alpha1.Workspace, accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy, namespace string) (string, string, error) {
	logger := ctrl.Log.WithName("vscode-handler")

	// Validate inputs
	if accessStrategy == nil {
		return "", "", fmt.Errorf("no access strategy configured for workspace")
	}

	clusterId := s.config.ClusterId

	// Get pod UID from workspace name using existing k8sClient
	podUID, err := workspace.GetPodUIDFromWorkspaceName(s.k8sClient, ws.Name)
	if err != nil {
		logger.Error(err, "Failed to get pod UID", "workspaceName", ws.Name)
		return "", "", err
	}

	logger.Info("Found pod UID for workspace", "workspaceName", ws.Name, "podUID", podUID)

	// Create SSM remote access strategy
	ssmStrategy, err := newSSMRemoteAccessStrategy(nil, &noOpPodExec{})
	if err != nil {
		return "", "", err
	}

	// Generate VSCode connection URL using SSM strategy with access strategy
	connectionURL, err := ssmStrategy.GenerateVSCodeConnectionURL(r.Context(), ws.Name, namespace, podUID, clusterId, accessStrategy)
	if err != nil {
		return "", "", err
	}

	return connectionv1alpha1.ConnectionTypeVSCodeRemote, connectionURL, nil
}

// checkWorkspaceAuthorization checks if the user is authorized to access the workspace
func (s *ExtensionServer) checkWorkspaceAuthorization(r *http.Request, workspaceName, namespace string) (*workspacev1alpha1.Workspace, *WorkspaceAdmissionResult, error) {
	user := GetUser(r)
	if user == "" {
		return nil, nil, fmt.Errorf("user not found in request headers")
	}

	return s.CheckWorkspaceAccess(namespace, workspaceName, user, s.logger)
}

// renderBearerAuthURL renders the BearerAuthURLTemplate with workspace variables
func (s *ExtensionServer) renderBearerAuthURL(templateStr string, ws *workspacev1alpha1.Workspace, accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) (string, error) {
	tmpl, err := template.New("bearerauth").Funcs(template.FuncMap{
		"b32encode": workspace.EncodeNamespaceB32,
	}).Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var result strings.Builder
	data := map[string]interface{}{
		"Workspace":      ws,
		"AccessStrategy": accessStrategy,
	}

	if err := tmpl.Execute(&result, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return result.String(), nil
}
