/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package extensionapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/go-logr/logr"
	pluginapi "github.com/jupyter-infra/jupyter-k8s-plugin/api"
	"github.com/jupyter-infra/jupyter-k8s-plugin/pluginclient"
	connectionv1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/connection/v1alpha1"
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
	"github.com/jupyter-infra/jupyter-k8s/internal/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The controller derives status.connectionTypes from the rules HandleConnectionCreate applies here. This test
// drives both over the same strategies and server settings, so a change on either side fails until the other
// follows. Two settings only the controller sees stay in its own unit tests: a disabled extension API runs no
// server, and a missing JWT secret leaves the server without any signer factory. An unmapped {ide}-remote type
// is the documented divergence: the default handler may serve it, the list never names it.
func TestConnectionTypesMatchHandleConnectionCreate(t *testing.T) {
	const (
		webSocketTestURL = "wss://test.com/ssh-ws"
		unmappedRemote   = "unmapped-remote"
	)
	handlerMap := map[string]string{
		connectionTypeVSCodeRemote: workspacev1alpha1.CreateConnectionHandlerK8sNative,
		"kiro-remote":              awsCreateSession,
		"cursor-remote":            "azure:createSession",
		"zed-remote":               "aws:ssm-remote-access",
		"vscode_remote":            awsCreateSession,
	}
	strategies := map[string]workspacev1alpha1.WorkspaceAccessStrategySpec{
		"browser route only":   {BearerAuthURLTemplate: bearerAuthTestURL},
		"websocket route only": {WebSocketURLTemplate: webSocketTestURL},
		"browser route and a handler map": {
			BearerAuthURLTemplate:      bearerAuthTestURL,
			CreateConnectionHandlerMap: handlerMap,
		},
		"both routes and a handler map": {
			BearerAuthURLTemplate:      bearerAuthTestURL,
			WebSocketURLTemplate:       webSocketTestURL,
			CreateConnectionHandlerMap: handlerMap,
		},
		"a plugin as the default handler": {
			BearerAuthURLTemplate:      bearerAuthTestURL,
			WebSocketURLTemplate:       webSocketTestURL,
			CreateConnectionHandler:    awsCreateSession,
			CreateConnectionHandlerMap: handlerMap,
		},
		"k8s-native as the default handler": {
			BearerAuthURLTemplate:      bearerAuthTestURL,
			WebSocketURLTemplate:       webSocketTestURL,
			CreateConnectionHandler:    workspacev1alpha1.CreateConnectionHandlerK8sNative,
			CreateConnectionHandlerMap: handlerMap,
		},
	}

	for name, spec := range strategies {
		for _, awsPlugin := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s, aws plugin configured=%t", name, awsPlugin), func(t *testing.T) {
				ws, strategy := availableWorkspaceWithStrategy(spec)
				server := newConnectionServer(t, ws, strategy)
				server.signerFactory = signedK8sNativeFactory(t)
				options := controller.WorkspaceControllerOptions{ExtensionAPIEnabled: true, K8sNativeSignerEnabled: true}
				if awsPlugin {
					pluginURL := createSessionPlugin(t)
					server.pluginClients = map[string]*pluginclient.PluginClient{
						pluginNameAWS: pluginclient.NewPluginClient(pluginURL, logr.Discard()),
					}
					options.PluginEndpoints = map[string]string{pluginNameAWS: pluginURL}
				}
				listed := controller.NewAccessResourcesBuilder(options).ResolveConnectionTypes(strategy)

				candidates := make([]string, 0, 2+len(spec.CreateConnectionHandlerMap))
				candidates = append(candidates, connectionv1alpha1.ConnectionTypeWebUI, connectionv1alpha1.ConnectionTypeWebSocket)
				for key := range spec.CreateConnectionHandlerMap {
					candidates = append(candidates, key)
				}
				servedByHandler := map[string]bool{}
				for _, connectionType := range candidates {
					w := postConnection(t, server, connectionType)
					served := w.Code == http.StatusCreated
					assert.Equal(t, served, slices.Contains(listed, connectionType),
						"%s: served=%t (HTTP %d: %s) but listed=%v", connectionType, served, w.Code, w.Body.String(), listed)
					// A key outside the {ide}-remote pattern is refused before its handler is consulted.
					if handlerRef, mapped := spec.CreateConnectionHandlerMap[connectionType]; mapped &&
						connectionv1alpha1.IsRemoteConnectionType(connectionType) {
						servedByHandler[handlerRef] = served
					}
				}

				// The default handler serves an unmapped type exactly as it serves a mapped one, and the list omits it.
				assert.NotContains(t, listed, unmappedRemote)
				w := postConnection(t, server, unmappedRemote)
				assert.Equal(t, servedByHandler[spec.CreateConnectionHandler], w.Code == http.StatusCreated,
					"%s through the default handler %q: HTTP %d: %s", unmappedRemote, spec.CreateConnectionHandler, w.Code, w.Body.String())
			})
		}
	}
}

// signedK8sNativeFactory builds the factory the server builds when --jwt-secret-name is set, with a signing key
// loaded and past its cool-off so token generation succeeds.
func signedK8sNativeFactory(t *testing.T) jwt.SignerFactory {
	t.Helper()
	factory, err := createJWTCompositeSignerFactory(&ExtensionConfig{
		JwtSecretName:  "signing-keys",
		NewKeyUseDelay: time.Nanosecond,
	})
	require.NoError(t, err)
	signer, err := factory.CreateSigner(nil)
	require.NoError(t, err)
	standard, ok := signer.(*jwt.StandardSigner)
	require.True(t, ok, "the k8s-native factory should hand out the standard signer")
	require.NoError(t, standard.UpdateKeys(map[string][]byte{"k1": []byte("0123456789abcdef0123456789abcdef")}, "k1"))
	return factory
}

// createSessionPlugin serves a plugin that answers every CreateSession call with a URL.
func createSessionPlugin(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pluginapi.CreateSessionResponse{ConnectionURL: "vscode://remote"})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}
