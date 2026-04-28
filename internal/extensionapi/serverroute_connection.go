/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

// Package extensionapi provides extension API server functionality.
package extensionapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"text/template"

	"github.com/go-logr/logr"
	pluginapi "github.com/jupyter-infra/jupyter-k8s-plugin/api"
	"github.com/jupyter-infra/jupyter-k8s-plugin/plugin"
	connectionv1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/connection/v1alpha1"
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/jwt"
	"github.com/jupyter-infra/jupyter-k8s/internal/workspace"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// handlerK8sNative is the reserved handler name for core k8s-native functionality.
	handlerK8sNative = "k8s-native"
)

// isWorkspaceAvailable checks if the workspace has the Available condition set to True.
func isWorkspaceAvailable(ws *workspacev1alpha1.Workspace) bool {
	for _, condition := range ws.Status.Conditions {
		if condition.Type == "Available" && condition.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

// resolveConnectionHandler looks up the handler for a given connection type.
// First checks CreateConnectionHandlerMap, then falls back to CreateConnectionHandler.
func resolveConnectionHandler(accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy, connectionType string) (pluginName, action string, found bool) {
	if accessStrategy == nil {
		return "", "", false
	}

	// Check the handler map first
	if ref, ok := accessStrategy.Spec.CreateConnectionHandlerMap[connectionType]; ok {
		pluginName, action = plugin.ParseHandlerRef(ref)
		return pluginName, action, true
	}

	// Fall back to default handler
	if accessStrategy.Spec.CreateConnectionHandler != "" {
		pluginName, action = plugin.ParseHandlerRef(accessStrategy.Spec.CreateConnectionHandler)
		return pluginName, action, true
	}

	return "", "", false
}

// validateConnection validates that a workspace is ready for connection and resolves context.
// Returns the access strategy, resolved connection context, or an HTTP error.
func (s *ExtensionServer) validateConnection(ws *workspacev1alpha1.Workspace, logger logr.Logger) (*workspacev1alpha1.WorkspaceAccessStrategy, map[string]string, int, error) {
	// AccessStrategy should exist because the controller prevents deletion while workspaces reference it
	accessStrategy, err := s.getAccessStrategy(ws)
	if err != nil {
		logger.Error(err, "Failed to get access strategy", "workspaceName", ws.Name)

		if errors.IsNotFound(err) {
			return nil, nil, http.StatusNotFound, fmt.Errorf("access strategy not found")
		}
		return nil, nil, http.StatusInternalServerError, fmt.Errorf("internal server error")
	}

	// Check workspace is available
	if !isWorkspaceAvailable(ws) {
		logger.Info("Connection rejected: workspace not available",
			"workspaceName", ws.Name,
			"namespace", ws.Namespace)
		return nil, nil, http.StatusBadRequest, fmt.Errorf("workspace is not available. Check workspace status for details")
	}

	// Resolve dynamic values in connection context (e.g. extensionapi::PodUid())
	resolvedContext, err := ResolveConnectionContext(accessStrategy.Spec.CreateConnectionContext, s.k8sClient, ws.Name)
	if err != nil {
		logger.Error(err, "Failed to resolve connection context", "workspaceName", ws.Name)
		return nil, nil, http.StatusInternalServerError, fmt.Errorf("failed to resolve connection context: %w", err)
	}

	return accessStrategy, resolvedContext, 0, nil
}

// generateBearerTokenURL generates a connection URL with JWT bearer token.
// Used for k8s-native connections (web-ui).
func (s *ExtensionServer) generateBearerTokenURL(r *http.Request, ws *workspacev1alpha1.Workspace, accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) (string, error) {
	user := GetUser(r)
	if user == "" {
		return "", fmt.Errorf("user information not found in request headers")
	}
	groups := GetGroups(r)
	extra := GetExtra(r)

	if accessStrategy == nil {
		return "", fmt.Errorf("no AccessStrategy configured for workspace")
	}
	if accessStrategy.Spec.BearerAuthURLTemplate == "" {
		return "", fmt.Errorf("BearerAuthURLTemplate not configured in AccessStrategy")
	}

	// Create signer based on access strategy
	signer, err := s.signerFactory.CreateSigner(accessStrategy)
	if err != nil {
		return "", fmt.Errorf("failed to create signer: %w", err)
	}

	// Generate URL from template
	bearerURL, err := s.renderBearerAuthURL(accessStrategy.Spec.BearerAuthURLTemplate, ws, accessStrategy)
	if err != nil {
		return "", fmt.Errorf("failed to render bearer auth URL: %w", err)
	}

	// Parse URL to extract domain and path for JWT claims
	parsedURL, err := url.Parse(bearerURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse generated URL: %w", err)
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

	// Generate JWT token with domain and path using access strategy-specific signer.
	// skipRefresh=true: bootstrap tokens are exchanged immediately for session tokens
	// via /bearer-auth, so refresh is not applicable.
	token, err := signer.GenerateToken(user, groups, user, extra, path, domain, jwt.TokenTypeBootstrap, true)
	if err != nil {
		return "", fmt.Errorf("failed to generate JWT token: %w", err)
	}

	return fmt.Sprintf("%s?token=%s", bearerURL, token), nil
}

// generatePluginConnectionURL delegates connection URL generation to a plugin.
func (s *ExtensionServer) generatePluginConnectionURL(r *http.Request, ws *workspacev1alpha1.Workspace, pluginName, action, connectionType string, resolvedContext map[string]string, namespace string) (string, error) {
	pc, ok := s.pluginClients[pluginName]
	if !ok {
		return "", fmt.Errorf("no plugin endpoint configured for handler %q", pluginName)
	}

	// Propagate a request ID so plugin logs can be correlated with this API call
	ctx := plugin.ContextWithOriginRequestID(r.Context(), plugin.GenerateRequestID())

	switch action {
	case "createSession", "":
		req := &pluginapi.CreateSessionRequest{
			PodUID:            resolvedContext["podUid"],
			WorkspaceName:     ws.Name,
			Namespace:         namespace,
			ConnectionType:    connectionType,
			ConnectionContext: resolvedContext,
		}
		resp, err := pc.CreateSession(ctx, req)
		if err != nil {
			return "", err
		}
		return resp.ConnectionURL, nil
	default:
		return "", fmt.Errorf("unsupported plugin action %q for handler %q", action, pluginName)
	}
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

	connectionType := req.Spec.WorkspaceConnectionType

	logger.Info("Creating workspace connection",
		"namespace", namespace,
		"workspaceName", req.Spec.WorkspaceName,
		"connectionType", connectionType)

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

	// Validate connection readiness and resolve context
	accessStrategy, resolvedContext, statusCode, err := s.validateConnection(ws, logger)
	if err != nil {
		WriteKubernetesError(w, statusCode, err.Error())
		return
	}

	// Generate connection URL based on connection type
	var responseType, responseURL string

	switch connectionType {
	case connectionv1alpha1.ConnectionTypeWebUI:
		if !hasWebUIEnabled(accessStrategy) {
			WriteKubernetesError(w, http.StatusBadRequest, "web browser access is not enabled for this workspace")
			return
		}
		responseURL, err = s.generateBearerTokenURL(r, ws, accessStrategy)
		responseType = connectionv1alpha1.ConnectionTypeWebUI

	default:
		// All *-remote types go through the plugin handler map
		pluginName, action, found := resolveConnectionHandler(accessStrategy, connectionType)
		if !found {
			WriteKubernetesError(w, http.StatusBadRequest, fmt.Sprintf("connection type %q is not configured for this workspace", connectionType))
			return
		}
		if pluginName == handlerK8sNative {
			WriteKubernetesError(w, http.StatusBadRequest, fmt.Sprintf("k8s-native handler is not supported for remote connections (requested %q)", connectionType))
			return
		}
		if _, ok := s.pluginClients[pluginName]; !ok {
			WriteKubernetesError(w, http.StatusBadRequest, "plugin endpoints not configured. Please configure controller.plugins in helm values")
			return
		}
		responseURL, err = s.generatePluginConnectionURL(r, ws, pluginName, action, connectionType, resolvedContext, namespace)
		responseType = connectionType
	}

	if err != nil {
		logger.Error(err, "Failed to generate connection URL", "connectionType", connectionType)
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

// remoteConnectionTypeRegex validates the {ide}-remote pattern: alphanumeric segments separated by single hyphens.
var remoteConnectionTypeRegex = regexp.MustCompile(`^[a-zA-Z0-9]+(?:-[a-zA-Z0-9]+)*-remote$`)

// isRemoteConnectionType checks if the connection type matches the {ide}-remote pattern.
func isRemoteConnectionType(connectionType string) bool {
	return remoteConnectionTypeRegex.MatchString(connectionType)
}

// validateWorkspaceConnectionRequest validates the workspace connection request
func validateWorkspaceConnectionRequest(req *connectionv1alpha1.WorkspaceConnectionRequest) error {
	if req.Spec.WorkspaceName == "" {
		return fmt.Errorf("workspaceName is required")
	}

	if req.Spec.WorkspaceConnectionType == "" {
		return fmt.Errorf("workspaceConnectionType is required")
	}

	connectionType := req.Spec.WorkspaceConnectionType

	// Accept web-ui, known remote types, and any *-remote pattern
	switch {
	case connectionType == connectionv1alpha1.ConnectionTypeWebUI:
		// valid
	case isRemoteConnectionType(connectionType):
		// valid — known or unknown remote types are accepted
	default:
		return fmt.Errorf("invalid workspaceConnectionType: '%s'. Must be 'web-ui' or follow the '{ide}-remote' pattern (e.g. 'vscode-remote', 'kiro-remote', 'cursor-remote')", connectionType)
	}

	return nil
}

// checkWorkspaceAuthorization checks if the user is authorized to access the workspace
func (s *ExtensionServer) checkWorkspaceAuthorization(r *http.Request, workspaceName, namespace string) (*workspacev1alpha1.Workspace, *WorkspaceAdmissionResult, error) {
	user := GetUser(r)
	if user == "" {
		return nil, nil, fmt.Errorf("user not found in request headers")
	}

	return s.CheckWorkspaceAccess(namespace, workspaceName, user, s.logger)
}

// hasWebUIEnabled checks if BearerAuthURLTemplate is defined in the access strategy.
func hasWebUIEnabled(accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) bool {
	if accessStrategy == nil {
		return false
	}
	return accessStrategy.Spec.BearerAuthURLTemplate != ""
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
