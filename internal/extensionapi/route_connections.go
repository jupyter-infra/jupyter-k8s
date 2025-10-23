package extensionapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/aws"
)

func handleConnectionCreate(config *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setupLog := ctrl.Log.WithName("connection-handler")

		// Extract namespace from URL path
		pathParts := strings.Split(r.URL.Path, "/")
		namespace := pathParts[5] // /apis/connection.workspaces.jupyter.org/v1alpha1/namespaces/{namespace}/connections

		// Parse request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		var req map[string]interface{}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Extract spec
		spec, ok := req["spec"].(map[string]interface{})
		if !ok {
			http.Error(w, "Missing spec", http.StatusBadRequest)
			return
		}

		spaceName, _ := spec["spaceName"].(string)
		ide, _ := spec["ide"].(string)

		if spaceName == "" {
			http.Error(w, "spaceName is required", http.StatusBadRequest)
			return
		}

		setupLog.Info("Creating connection", "namespace", namespace, "spaceName", spaceName, "ide", ide)

		// Get pod UID from space name
		podUID, err := getPodUIDFromSpaceName(spaceName)
		if err != nil {
			setupLog.Error(err, "Failed to get pod UID", "spaceName", spaceName)
		} else {
			setupLog.Info("Found pod UID for workspace", "spaceName", spaceName, "podUID", podUID)
		}

		// Generate response based on IDE type
		var responseType, responseURL string
		if ide == "vscode" {
			responseType, responseURL, err = generateVSCodeURL(r, config, spaceName, namespace, podUID)
		} else {
			responseType, responseURL, err = generateWebUIURL(r, config, spaceName, namespace)
		}

		if err != nil {
			setupLog.Error(err, "Failed to generate connection URL", "ide", ide)
			http.Error(w, "Failed to generate connection URL", http.StatusInternalServerError)
			return
		}

		// Create response
		response := map[string]interface{}{
			"apiVersion": "connection.workspaces.jupyter.org/v1alpha1",
			"kind":       "WorkspaceConnection",
			"metadata":   req["metadata"],
			"spec":       spec,
			"status": map[string]interface{}{
				"type": responseType,
				"url":  responseURL,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
	}
}

func generateVSCodeURL(r *http.Request, config *Config, spaceName, namespace, podUID string) (string, string, error) {
	setupLog := ctrl.Log.WithName("vscode-handler")

	// Create SSM client
	ssmClient, err := aws.NewSSMClient(r.Context())
	if err != nil {
		return "", "", err
	}

	// Find managed instance by pod UID
	instanceID, err := ssmClient.FindInstanceByPodUID(r.Context(), podUID)
	if err != nil {
		return "", "", err
	}

	setupLog.Info("Found managed instance for pod", "podUID", podUID, "instanceID", instanceID)

	// Get SSM document name
	documentName, err := aws.GetSSMDocumentName()
	if err != nil {
		return "", "", err
	}

	// Start SSM session
	sessionInfo, err := ssmClient.StartSession(r.Context(), instanceID, documentName)
	if err != nil {
		return "", "", err
	}

	setupLog.Info("SSM session started successfully", "instanceID", instanceID, "sessionID", sessionInfo.SessionID)

	if config.ClusterARN == "" {
		setupLog.Error(nil, "CLUSTER_ARN environment variable not set")
		return "", "", fmt.Errorf("CLUSTER_ARN not configured")
	}

	url := "vscode://amazonwebservices.aws-toolkit-vscode/connect/sagemaker?sessionId=" + sessionInfo.SessionID + "&sessionToken=" + sessionInfo.TokenValue + "&streamUrl=" + sessionInfo.StreamURL + "&workspaceName=" + spaceName + "&namespace=" + namespace + "&clusterArn=" + config.ClusterARN

	return "vscode", url, nil
}

func generateWebUIURL(r *http.Request, config *Config, spaceName, namespace string) (string, string, error) {
	setupLog := ctrl.Log.WithName("webui-handler")

	if config.KMSKeyID == "" {
		setupLog.Error(nil, "KMS_KEY_ID not configured")
		return "", "", fmt.Errorf("KMS_KEY_ID not configured")
	}

	kmsClient, err := aws.NewKMSClient(r.Context(), config.KMSKeyID)
	if err != nil {
		return "", "", err
	}

	// Generate JWT token with space info
	jwtToken, err := kmsClient.GenerateJWTToken(r.Context(), spaceName, namespace, "test-user")
	if err != nil {
		return "", "", err
	}

	setupLog.Info("Generated JWT token for Web UI", "spaceName", spaceName, "namespace", namespace)

	url := "https://test-presigned-url.com/" + namespace + "/" + spaceName + "?token=" + jwtToken

	return "webui", url, nil
}
