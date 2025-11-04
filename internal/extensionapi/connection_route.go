// Package extensionapi provides extension API server functionality.
package extensionapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	connectionv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/connection/v1alpha1"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/aws"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/workspace"
	corev1 "k8s.io/api/core/v1"
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

// generateWebUIBearerTokenURL generates a Web UI connection URL with JWT token
// Returns (connectionType, connectionURL, error)
func (s *ExtensionServer) generateWebUIBearerTokenURL(r *http.Request, workspaceName, namespace string) (string, string, error) {
	user := GetUserFromHeaders(r)
	if user == "" {
		return "", "", fmt.Errorf("user information not found in request headers")
	}

	// Generate JWT token for the user and workspace
	token, err := s.jwtManager.GenerateToken(user, []string{}, user, map[string][]string{}, workspaceName, namespace, "webui")
	if err != nil {
		return "", "", fmt.Errorf("failed to generate JWT token: %w", err)
	}

	// Construct the Web UI URL using the format: $DOMAIN/workspaces/{{ .Workspace.Namespace }}/{{ .Workspace.Name }}/bearer-auth?token=<token>
	webUIURL := fmt.Sprintf(WebUIURLFormat, s.config.Domain, namespace, workspaceName, token)

	return connectionv1alpha1.ConnectionTypeWebUI, webUIURL, nil
}

// HandleConnectionCreate handles POST requests to create a connection
func (s *ExtensionServer) HandleConnectionCreate(w http.ResponseWriter, r *http.Request) {
	logger := GetLoggerFromContext(r.Context())

	if r.Method != "POST" {
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
	}

	logger.Info("Creating workspace connection",
		"namespace", namespace,
		"workspaceName", req.Spec.WorkspaceName,
		"connectionType", req.Spec.WorkspaceConnectionType)

	// Check authorization for private workspaces
	result, err := s.checkWorkspaceAuthorization(r, req.Spec.WorkspaceName, namespace)
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

	// Generate response based on connection type
	var responseType, responseURL string
	switch req.Spec.WorkspaceConnectionType {
	case connectionv1alpha1.ConnectionTypeVSCodeRemote:
		responseType, responseURL, err = s.generateVSCodeURL(r, req.Spec.WorkspaceName, namespace)
	case connectionv1alpha1.ConnectionTypeWebUI:
		responseType, responseURL, err = s.generateWebUIBearerTokenURL(r, req.Spec.WorkspaceName, namespace)
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
func (s *ExtensionServer) generateVSCodeURL(r *http.Request, workspaceName, namespace string) (string, string, error) {
	logger := ctrl.Log.WithName("vscode-handler")

	// Get cluster ID from config (already validated earlier)
	clusterId := s.config.ClusterId

	// Get pod UID from workspace name using existing k8sClient
	podUID, err := workspace.GetPodUIDFromWorkspaceName(s.k8sClient, workspaceName)
	if err != nil {
		logger.Error(err, "Failed to get pod UID", "workspaceName", workspaceName)
		return "", "", err
	}

	logger.Info("Found pod UID for workspace", "workspaceName", workspaceName, "podUID", podUID)

	// Create SSM remote access strategy
	ssmStrategy, err := newSSMRemoteAccessStrategy(nil, &noOpPodExec{})
	if err != nil {
		return "", "", err
	}

	// Generate VSCode connection URL using SSM strategy
	url, err := ssmStrategy.GenerateVSCodeConnectionURL(r.Context(), workspaceName, namespace, podUID, clusterId)
	if err != nil {
		return "", "", err
	}

	return connectionv1alpha1.ConnectionTypeVSCodeRemote, url, nil
}

// generateWebUIURL generates a Web UI connection URL
// Returns (connectionType, connectionURL, error)
func (s *ExtensionServer) generateWebUIURL(r *http.Request, workspaceName, namespace string) (string, string, error) {
	// TODO: Implement Web UI URL generation with JWT tokens
	return connectionv1alpha1.ConnectionTypeWebUI, "https://placeholder-webui-url.com", nil
}

// checkWorkspaceAuthorization checks if the user is authorized to access the workspace
func (s *ExtensionServer) checkWorkspaceAuthorization(r *http.Request, workspaceName, namespace string) (*WorkspaceAdmissionResult, error) {
	user := GetUserFromHeaders(r)
	if user == "" {
		return nil, fmt.Errorf("user not found in request headers")
	}

	return s.CheckWorkspaceAccess(namespace, workspaceName, user, s.logger)
}
