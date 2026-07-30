/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package extensionapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	connectionv1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/connection/v1alpha1"
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/jwt"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Shared test fixtures for the serverroute_connection_*_test.go files: mock
// signer/validator implementations, a failing io.ReadCloser, and helpers for
// building an ExtensionServer and driving HandleConnectionCreate.

const testUser = "test-user"
const testWorkspaceMyWorkspace = "myworkspace"
const testStrategyWebSocket = "ws-strategy"

// mockSignerFactory for testing
type mockSignerFactory struct {
	signer *mockSigner
	err    error
}

func (m *mockSignerFactory) CreateSigner(accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) (jwt.Signer, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.signer, nil
}

// mockSigner for testing
type mockSigner struct {
	token string
	// captured args from last GenerateToken call
	lastGroups []string
	lastExtra  map[string][]string
}

func (m *mockSigner) GenerateToken(username string, groups []string, uid string, extra map[string][]string, path string, domain string, tokenType string, skipRefresh bool) (string, error) {
	m.lastGroups = groups
	m.lastExtra = extra
	return m.token, nil
}

func (m *mockSigner) GenerateRefreshToken(claims *jwt.Claims) (string, error) {
	return m.token, nil
}

func (m *mockSigner) ValidateToken(tokenString string) (*jwt.Claims, error) {
	return nil, nil
}

// mockTokenValidator for testing
type mockTokenValidator struct {
	claims *jwt.Claims
	err    error
}

func (m *mockTokenValidator) ValidateToken(tokenString string) (*jwt.Claims, error) {
	return m.claims, m.err
}

// badReader is a helper that always returns an error when reading
type badReader struct{}

func (e *badReader) Read(p []byte) (n int, err error) {
	return 0, fmt.Errorf("read error")
}

func (e *badReader) Close() error {
	return nil
}

// availableWorkspaceWithStrategy builds a Public, Available workspace referencing
// an access strategy, plus that access strategy, for driving HandleConnectionCreate
// end to end.
func availableWorkspaceWithStrategy(strategySpec workspacev1alpha1.WorkspaceAccessStrategySpec) (*workspacev1alpha1.Workspace, *workspacev1alpha1.WorkspaceAccessStrategy) {
	strategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{Name: strategyName, Namespace: namespaceDefault},
		Spec:       strategySpec,
	}
	ws := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: testWorkspace, Namespace: namespaceDefault},
		Spec: workspacev1alpha1.WorkspaceSpec{
			AccessType:     AccessTypePublic,
			AccessStrategy: &workspacev1alpha1.AccessStrategyRef{Name: strategyName, Namespace: namespaceDefault},
		},
		Status: workspacev1alpha1.WorkspaceStatus{
			Conditions: []metav1.Condition{
				{Type: conditionTypeAvailable, Status: metav1.ConditionTrue, Reason: "Ready",
					LastTransitionTime: metav1.Now()},
			},
		},
	}
	return ws, strategy
}

func newConnectionServer(t *testing.T, objs ...client.Object) *ExtensionServer {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, workspacev1alpha1.AddToScheme(scheme))
	logger := ctrl.Log.WithName("test")
	return &ExtensionServer{
		config:        &ExtensionConfig{},
		k8sClient:     ctrlclient.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build(),
		logger:        &logger,
		signerFactory: &mockSignerFactory{signer: &mockSigner{token: testToken}},
	}
}

func postConnection(t *testing.T, server *ExtensionServer, connectionType string) *httptest.ResponseRecorder {
	t.Helper()
	reqBody := connectionv1alpha1.WorkspaceConnectionRequest{
		Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
			WorkspaceName:           testWorkspace,
			WorkspaceConnectionType: connectionType,
		},
	}
	bodyBytes, err := json.Marshal(reqBody)
	require.NoError(t, err)

	httpReq := httptest.NewRequest("POST", connectionsPath, bytes.NewReader(bodyBytes))
	httpReq.Header.Set("X-User", testUser)
	w := httptest.NewRecorder()

	server.HandleConnectionCreate(w, httpReq)
	return w
}
