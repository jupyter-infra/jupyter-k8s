/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package extensionapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	pluginapi "github.com/jupyter-infra/jupyter-k8s-plugin/api"
	"github.com/jupyter-infra/jupyter-k8s-plugin/pluginclient"
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// --- generatePluginConnectionURL tests ---

func TestGeneratePluginConnectionURL_Success(t *testing.T) {
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: testWorkspace, Namespace: namespaceDefault},
	}

	// Create httptest server simulating the plugin
	pluginSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req pluginapi.CreateSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.PodUID != "test-uid-123" {
			t.Errorf("expected podUID test-uid-123, got %s", req.PodUID)
		}
		if req.ConnectionContext["ssmDocumentName"] != "test-document" {
			t.Errorf("expected ssmDocumentName test-document, got %s", req.ConnectionContext["ssmDocumentName"])
		}
		if req.ConnectionType != connectionTypeVSCodeRemote {
			t.Errorf("expected connectionType vscode-remote, got %s", req.ConnectionType)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pluginapi.CreateSessionResponse{
			ConnectionURL: "vscode://vscode-remote/ssh-remote+test-workspace/home/user",
		})
	}))
	defer pluginSrv.Close()

	server := &ExtensionServer{
		config: &ExtensionConfig{},
		pluginClients: map[string]*pluginclient.PluginClient{
			pluginNameAWS: pluginclient.NewPluginClient(pluginSrv.URL, logr.Discard()),
		},
	}

	resolvedContext := map[string]string{
		"podUid":          "test-uid-123",
		"ssmDocumentName": "test-document",
	}

	req := httptest.NewRequest("POST", "/test", nil)
	url, err := server.generatePluginConnectionURL(req, workspace, pluginNameAWS, "createSession", connectionTypeVSCodeRemote, resolvedContext, namespaceDefault)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if url != "vscode://vscode-remote/ssh-remote+test-workspace/home/user" {
		t.Errorf("unexpected URL: %s", url)
	}
}

func TestGeneratePluginConnectionURL_PluginError(t *testing.T) {
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: testWorkspace, Namespace: namespaceDefault},
	}

	pluginSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "SSM creation failed"})
	}))
	defer pluginSrv.Close()

	server := &ExtensionServer{
		config: &ExtensionConfig{},
		pluginClients: map[string]*pluginclient.PluginClient{
			pluginNameAWS: pluginclient.NewPluginClient(pluginSrv.URL, logr.Discard()),
		},
	}

	req := httptest.NewRequest("POST", "/test", nil)
	_, err := server.generatePluginConnectionURL(req, workspace, pluginNameAWS, "createSession", connectionTypeVSCodeRemote, map[string]string{}, namespaceDefault)

	if err == nil {
		t.Error("expected error from plugin client")
	}
}

func TestGeneratePluginConnectionURL_NoPlugin(t *testing.T) {
	server := &ExtensionServer{
		config:        &ExtensionConfig{},
		pluginClients: map[string]*pluginclient.PluginClient{},
	}

	req := httptest.NewRequest("POST", "/test", nil)
	_, err := server.generatePluginConnectionURL(req, &workspacev1alpha1.Workspace{}, pluginNameAWS, "createSession", connectionTypeVSCodeRemote, map[string]string{}, namespaceDefault)

	if err == nil {
		t.Error("expected error for missing plugin")
	}
	if !strings.Contains(err.Error(), "no plugin endpoint configured") {
		t.Errorf("expected plugin error, got: %v", err)
	}
}

func TestGeneratePluginConnectionURL_UnsupportedAction(t *testing.T) {
	server := &ExtensionServer{
		config:        &ExtensionConfig{},
		pluginClients: map[string]*pluginclient.PluginClient{pluginNameAWS: pluginclient.NewPluginClient("http://localhost:8080", logr.Discard())},
	}

	req := httptest.NewRequest("POST", "/test", nil)
	_, err := server.generatePluginConnectionURL(req, &workspacev1alpha1.Workspace{}, pluginNameAWS, "unknownAction", connectionTypeVSCodeRemote, map[string]string{}, namespaceDefault)

	if err == nil {
		t.Error("expected error for unsupported action")
	}
	if !strings.Contains(err.Error(), "unsupported plugin action") {
		t.Errorf("expected unsupported action error, got: %v", err)
	}
}

// --- resolveConnectionHandler tests ---

func TestResolveConnectionHandler(t *testing.T) {
	tests := []struct {
		name           string
		accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy
		connectionType string
		expectedPlugin string
		expectedAction string
		expectedFound  bool
	}{
		{
			name:           "nil access strategy",
			accessStrategy: nil,
			connectionType: connectionTypeWebUI,
			expectedFound:  false,
		},
		{
			name: "found in handler map",
			accessStrategy: &workspacev1alpha1.WorkspaceAccessStrategy{
				Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
					CreateConnectionHandlerMap: map[string]string{
						connectionTypeVSCodeRemote: "aws:createSession",
					},
					CreateConnectionHandler: handlerK8sNative,
				},
			},
			connectionType: connectionTypeVSCodeRemote,
			expectedPlugin: pluginNameAWS,
			expectedAction: "createSession",
			expectedFound:  true,
		},
		{
			name: "falls back to default handler",
			accessStrategy: &workspacev1alpha1.WorkspaceAccessStrategy{
				Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
					CreateConnectionHandlerMap: map[string]string{
						connectionTypeVSCodeRemote: "aws:createSession",
					},
					CreateConnectionHandler: handlerK8sNative,
				},
			},
			connectionType: connectionTypeWebUI,
			expectedPlugin: handlerK8sNative,
			expectedAction: "",
			expectedFound:  true,
		},
		{
			name: "no handler configured",
			accessStrategy: &workspacev1alpha1.WorkspaceAccessStrategy{
				Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{},
			},
			connectionType: connectionTypeWebUI,
			expectedFound:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin, action, found := resolveConnectionHandler(tt.accessStrategy, tt.connectionType)
			if found != tt.expectedFound {
				t.Errorf("expected found=%v, got %v", tt.expectedFound, found)
			}
			if plugin != tt.expectedPlugin {
				t.Errorf("expected plugin=%q, got %q", tt.expectedPlugin, plugin)
			}
			if action != tt.expectedAction {
				t.Errorf("expected action=%q, got %q", tt.expectedAction, action)
			}
		})
	}
}
