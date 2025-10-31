// Package extensionapi provides extension API server functionality.
package extensionapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	connectionv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/connection/v1alpha1"
	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/aws"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/workspace"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
)

// noOpPodExec is a no-op implementation of PodExecInterface for connection URL generation
type noOpPodExec struct{}

func (n *noOpPodExec) ExecInPod(ctx context.Context, pod *corev1.Pod, containerName string, cmd []string, stdin string) (string, error) {
	return "", fmt.Errorf("pod exec not supported in connection URL generation")
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
	if err := s.checkWorkspaceAuthorization(r, req.Spec.WorkspaceName, namespace); err != nil {
		logger.Error(err, "Authorization failed", "workspaceName", req.Spec.WorkspaceName)
		WriteKubernetesError(w, http.StatusForbidden, err.Error())
		return
	}

	// Generate response based on connection type
	var responseType, responseURL string
	switch req.Spec.WorkspaceConnectionType {
	case connectionv1alpha1.ConnectionTypeVSCodeRemote:
		responseType, responseURL, err = s.generateVSCodeURL(r, req.Spec.WorkspaceName, namespace)
	case connectionv1alpha1.ConnectionTypeWebUI:
		responseType, responseURL, err = s.generateWebUIURL(r, req.Spec.WorkspaceName, namespace)
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
		return fmt.Errorf("invalid workspaceConnectionType: %s", req.Spec.WorkspaceConnectionType)
	}

	return nil
}

// generateVSCodeURL generates a VSCode connection URL using SSM remote access strategy
// Returns (connectionType, connectionURL, error)
func (s *ExtensionServer) generateVSCodeURL(r *http.Request, workspaceName, namespace string) (string, string, error) {
	logger := ctrl.Log.WithName("vscode-handler")

	// Get cluster ID from config (already validated earlier)
	clusterId := s.config.ClusterId

	// Get pod UID from workspace name
	config := ctrl.GetConfigOrDie()
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "", "", err
	}

	podUID, err := workspace.GetPodUIDFromWorkspaceName(clientset, workspaceName)
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
func (s *ExtensionServer) checkWorkspaceAuthorization(r *http.Request, workspaceName, namespace string) error {
	var ws workspacev1alpha1.Workspace
	err := s.k8sClient.Get(context.TODO(), types.NamespacedName{
		Name:      workspaceName,
		Namespace: namespace,
	}, &ws)
	if err != nil {
		return fmt.Errorf("failed to get workspace %s: %w", workspaceName, err)
	}

	// Check ownership type
	ownershipType := ws.Spec.OwnershipType

	// If workspace is public or ownership type not set, allow access
	if ownershipType == "" || ownershipType == "Public" {
		return nil
	}

	// For private workspaces (OwnerOnly), check if user matches owner
	if ownershipType == "OwnerOnly" {
		annotations := ws.GetAnnotations()
		createdBy, exists := annotations["workspace.jupyter.org/created-by"]
		if !exists {
			return fmt.Errorf("workspace owner information not found")
		}

		requestUser := getUserFromHeaders(r)

		if createdBy != requestUser {
			return fmt.Errorf("access denied: user %s is not authorized to access workspace owned by %s", requestUser, createdBy)
		}
	}

	return nil
}

// getUserFromHeaders extracts the user from request headers
func getUserFromHeaders(r *http.Request) string {
	if user := r.Header.Get("X-User"); user != "" {
		return user
	}
	if user := r.Header.Get("X-Remote-User"); user != "" {
		return user
	}
	return "unknown"
}
