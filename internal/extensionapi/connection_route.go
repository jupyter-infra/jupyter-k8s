// Package extensionapi provides extension API server functionality.
package extensionapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/aws"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/workspace"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// noOpPodExec is a no-op implementation of PodExecInterface for connection URL generation
type noOpPodExec struct{}

func (n *noOpPodExec) ExecInPod(ctx context.Context, pod *corev1.Pod, containerName string, cmd []string, stdin string) (string, error) {
	return "", fmt.Errorf("pod exec not supported in connection URL generation")
}

// WorkspaceConnectionRequest represents the request body for creating a workspace connection
type WorkspaceConnectionRequest struct {
	APIVersion string                         `json:"apiVersion"`
	Kind       string                         `json:"kind"`
	Metadata   map[string]interface{}         `json:"metadata"`
	Spec       WorkspaceConnectionRequestSpec `json:"spec"`
}

// WorkspaceConnectionRequestSpec represents the spec of a workspace connection request
type WorkspaceConnectionRequestSpec struct {
	WorkspaceName           string `json:"workspaceName"`
	WorkspaceConnectionType string `json:"workspaceConnectionType"`
}

// WorkspaceConnectionResponse represents the response for a workspace connection
type WorkspaceConnectionResponse struct {
	APIVersion string                            `json:"apiVersion"`
	Kind       string                            `json:"kind"`
	Metadata   map[string]interface{}            `json:"metadata"`
	Spec       WorkspaceConnectionRequestSpec    `json:"spec"`
	Status     WorkspaceConnectionResponseStatus `json:"status"`
}

// WorkspaceConnectionResponseStatus represents the status of a workspace connection response
type WorkspaceConnectionResponseStatus struct {
	WorkspaceConnectionType string `json:"workspaceConnectionType"`
	WorkspaceConnectionURL  string `json:"workspaceConnectionUrl"`
}

// HandleConnectionCreate handles POST requests to create a connection
func (s *ExtensionServer) HandleConnectionCreate(w http.ResponseWriter, r *http.Request) {
	logger := GetLoggerFromContext(r.Context())

	if r.Method != "POST" {
		WriteError(w, http.StatusBadRequest, "Connection must use POST method")
		return
	}

	// Extract namespace from URL path
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 6 {
		WriteError(w, http.StatusBadRequest, "Invalid URL path")
		return
	}
	namespace := pathParts[5] // /apis/connection.workspaces.jupyter.org/v1alpha1/namespaces/{namespace}/connections

	// Parse request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	var req WorkspaceConnectionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Validate request
	if err := validateWorkspaceConnectionRequest(&req); err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Check if CLUSTER_ARN is configured early for VSCode connections
	if req.Spec.WorkspaceConnectionType == ConnectionTypeVSCodeRemote {
		eksClusterARN := os.Getenv(aws.EKSClusterARNEnv)
		if eksClusterARN == "" {
			WriteError(w, http.StatusInternalServerError, "EKS_CLUSTER_ARN not configured. Please upgrade helm chart with eksClusterArn parameter")
			return
		}
	}

	logger.Info("Creating workspace connection",
		"namespace", namespace,
		"workspaceName", req.Spec.WorkspaceName,
		"connectionType", req.Spec.WorkspaceConnectionType)

	// TODO: Implement authorization check for private workspaces

	// Generate response based on connection type
	var responseType, responseURL string
	switch req.Spec.WorkspaceConnectionType {
	case ConnectionTypeVSCodeRemote:
		responseType, responseURL, err = s.generateVSCodeURL(r, req.Spec.WorkspaceName, namespace)
	case ConnectionTypeWebUI:
		responseType, responseURL, err = s.generateWebUIURL(r, req.Spec.WorkspaceName, namespace)
	default:
		WriteError(w, http.StatusBadRequest, "Invalid workspace connection type")
		return
	}

	if err != nil {
		logger.Error(err, "Failed to generate connection URL", "connectionType", req.Spec.WorkspaceConnectionType)
		WriteError(w, http.StatusInternalServerError, "Failed to generate connection URL")
		return
	}

	// Create response
	response := WorkspaceConnectionResponse{
		APIVersion: WorkspaceConnectionAPIVersion,
		Kind:       WorkspaceConnectionKind,
		Metadata:   req.Metadata,
		Spec:       req.Spec,
		Status: WorkspaceConnectionResponseStatus{
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
func validateWorkspaceConnectionRequest(req *WorkspaceConnectionRequest) error {
	if req.Spec.WorkspaceName == "" {
		return fmt.Errorf("workspaceName is required")
	}

	if req.Spec.WorkspaceConnectionType == "" {
		return fmt.Errorf("workspaceConnectionType is required")
	}

	// Validate connection type against enum
	switch req.Spec.WorkspaceConnectionType {
	case ConnectionTypeVSCodeRemote, ConnectionTypeWebUI:
		// Valid types
	default:
		return fmt.Errorf("invalid workspaceConnectionType: %s", req.Spec.WorkspaceConnectionType)
	}

	return nil
}

// generateVSCodeURL generates a VSCode connection URL using SSM remote access strategy
func (s *ExtensionServer) generateVSCodeURL(r *http.Request, workspaceName, namespace string) (string, string, error) {
	logger := ctrl.Log.WithName("vscode-handler")

	// Get cluster ARN (already validated earlier)
	eksClusterARN := os.Getenv(aws.EKSClusterARNEnv)

	// Get pod UID from workspace name
	podUID, err := workspace.GetPodUIDFromWorkspaceName(workspaceName)
	if err != nil {
		logger.Error(err, "Failed to get pod UID", "workspaceName", workspaceName)
		return "", "", err
	}

	logger.Info("Found pod UID for workspace", "workspaceName", workspaceName, "podUID", podUID)

	// Create SSM remote access strategy
	ssmStrategy, err := aws.NewSSMRemoteAccessStrategy(nil, &noOpPodExec{})
	if err != nil {
		return "", "", err
	}

	// Generate VSCode connection URL using SSM strategy
	url, err := ssmStrategy.GenerateVSCodeConnectionURL(r.Context(), workspaceName, namespace, podUID, eksClusterARN)
	if err != nil {
		return "", "", err
	}

	return ConnectionTypeVSCodeRemote, url, nil
}

// generateWebUIURL generates a Web UI connection URL
func (s *ExtensionServer) generateWebUIURL(r *http.Request, workspaceName, namespace string) (string, string, error) {
	// TODO: Implement Web UI URL generation with JWT tokens
	return ConnectionTypeWebUI, "https://placeholder-webui-url.com", nil
}
