/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package extensionapi

import (
	"net/http/httptest"
	"strings"
	"testing"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// --- generateBearerTokenURL tests ---

func TestGenerateBearerTokenURL(t *testing.T) {
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: testWorkspace, Namespace: namespaceDefault},
		Spec: workspacev1alpha1.WorkspaceSpec{
			AccessStrategy: &workspacev1alpha1.AccessStrategyRef{Name: testStrategy},
		},
	}

	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{Name: testStrategy, Namespace: namespaceDefault},
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			BearerAuthURLTemplate: "https://test.com/workspaces/{{.Workspace.Namespace}}/{{.Workspace.Name}}/bearer-auth",
		},
	}

	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)
	fakeClient := ctrlclient.NewClientBuilder().WithScheme(scheme).WithObjects(workspace, accessStrategy).Build()

	server := &ExtensionServer{
		config:        &ExtensionConfig{},
		signerFactory: &mockSignerFactory{signer: &mockSigner{token: testToken}},
		k8sClient:     fakeClient,
	}

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Remote-User", testUser)

	url, err := server.generateBearerTokenURL(req, workspace, accessStrategy)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	expected := "https://test.com/workspaces/default/test-workspace/bearer-auth?token=test-token"
	if url != expected {
		t.Errorf("expected %s, got %s", expected, url)
	}
}

func TestGenerateBearerTokenURL_SubdomainRouting(t *testing.T) {
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceMyWorkspace, Namespace: namespaceDefault},
		Spec: workspacev1alpha1.WorkspaceSpec{
			AccessStrategy: &workspacev1alpha1.AccessStrategyRef{Name: "subdomain-strategy"},
		},
	}

	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{Name: "subdomain-strategy", Namespace: namespaceDefault},
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			BearerAuthURLTemplate: "https://{{.Workspace.Name}}-{{b32encode .Workspace.Namespace}}.example.com/bearer-auth",
		},
	}

	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)
	fakeClient := ctrlclient.NewClientBuilder().WithScheme(scheme).WithObjects(workspace, accessStrategy).Build()

	server := &ExtensionServer{
		config:        &ExtensionConfig{},
		signerFactory: &mockSignerFactory{signer: &mockSigner{token: testToken}},
		k8sClient:     fakeClient,
	}

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Remote-User", testUser)

	url, err := server.generateBearerTokenURL(req, workspace, accessStrategy)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	expected := "https://myworkspace-mrswmylvnr2a.example.com/bearer-auth?token=test-token"
	if url != expected {
		t.Errorf("expected %s, got %s", expected, url)
	}
}

func TestGenerateBearerTokenURL_PassesGroupsAndExtra(t *testing.T) {
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: testWorkspace, Namespace: namespaceDefault},
		Spec: workspacev1alpha1.WorkspaceSpec{
			AccessStrategy: &workspacev1alpha1.AccessStrategyRef{Name: testStrategy},
		},
	}

	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{Name: testStrategy, Namespace: namespaceDefault},
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			BearerAuthURLTemplate: bearerAuthTestURL,
		},
	}

	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)
	fakeClient := ctrlclient.NewClientBuilder().WithScheme(scheme).WithObjects(workspace, accessStrategy).Build()

	signer := &mockSigner{token: testToken}
	server := &ExtensionServer{
		config:        &ExtensionConfig{},
		signerFactory: &mockSignerFactory{signer: signer},
		k8sClient:     fakeClient,
	}

	req := httptest.NewRequest("POST", "/test", nil)
	userInfo := &user.DefaultInfo{
		Name:   testUser,
		Groups: []string{"cluster-workspace-admin", "system:authenticated"},
		Extra: map[string][]string{
			"arn":          {"arn:aws:sts::123456:assumed-role/Admin/session"},
			"canonicalarn": {"arn:aws:iam::123456:role/Admin"},
		},
	}
	ctx := request.WithUser(req.Context(), userInfo)
	req = req.WithContext(ctx)

	_, err := server.generateBearerTokenURL(req, workspace, accessStrategy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify groups were passed to the signer
	if len(signer.lastGroups) != 2 {
		t.Fatalf("expected 2 groups, got %d: %v", len(signer.lastGroups), signer.lastGroups)
	}
	if signer.lastGroups[0] != "cluster-workspace-admin" || signer.lastGroups[1] != "system:authenticated" {
		t.Errorf("unexpected groups: %v", signer.lastGroups)
	}

	// Verify extra was passed to the signer
	if len(signer.lastExtra) != 2 {
		t.Fatalf("expected 2 extra keys, got %d: %v", len(signer.lastExtra), signer.lastExtra)
	}
	if signer.lastExtra["arn"][0] != "arn:aws:sts::123456:assumed-role/Admin/session" {
		t.Errorf("unexpected extra arn: %v", signer.lastExtra["arn"])
	}
	if signer.lastExtra["canonicalarn"][0] != "arn:aws:iam::123456:role/Admin" {
		t.Errorf("unexpected extra canonicalarn: %v", signer.lastExtra["canonicalarn"])
	}
}

func TestGenerateBearerTokenURL_NoContextFallsBackToEmptyGroupsAndExtra(t *testing.T) {
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: testWorkspace, Namespace: namespaceDefault},
		Spec: workspacev1alpha1.WorkspaceSpec{
			AccessStrategy: &workspacev1alpha1.AccessStrategyRef{Name: testStrategy},
		},
	}

	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{Name: testStrategy, Namespace: namespaceDefault},
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			BearerAuthURLTemplate: bearerAuthTestURL,
		},
	}

	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)
	fakeClient := ctrlclient.NewClientBuilder().WithScheme(scheme).WithObjects(workspace, accessStrategy).Build()

	signer := &mockSigner{token: testToken}
	server := &ExtensionServer{
		config:        &ExtensionConfig{},
		signerFactory: &mockSignerFactory{signer: signer},
		k8sClient:     fakeClient,
	}

	// Request with header-based user, no Kubernetes context
	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Remote-User", testUser)

	_, err := server.generateBearerTokenURL(req, workspace, accessStrategy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Groups and extra should be nil when no k8s context
	if signer.lastGroups != nil {
		t.Errorf("expected nil groups, got %v", signer.lastGroups)
	}
	if signer.lastExtra != nil {
		t.Errorf("expected nil extra, got %v", signer.lastExtra)
	}
}

func TestGenerateBearerTokenURL_NoAccessStrategy(t *testing.T) {
	server := &ExtensionServer{
		config:        &ExtensionConfig{},
		signerFactory: &mockSignerFactory{signer: &mockSigner{token: testToken}},
	}

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Remote-User", testUser)

	_, err := server.generateBearerTokenURL(req, &workspacev1alpha1.Workspace{}, nil)

	if err == nil {
		t.Error("expected error for missing AccessStrategy, got nil")
	}
	if !strings.Contains(err.Error(), "no AccessStrategy configured") {
		t.Errorf("expected AccessStrategy error, got: %v", err)
	}
}

func TestGenerateBearerTokenURL_MissingTemplate(t *testing.T) {
	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			BearerAuthURLTemplate: "",
		},
	}

	server := &ExtensionServer{
		config:        &ExtensionConfig{},
		signerFactory: &mockSignerFactory{signer: &mockSigner{token: testToken}},
	}

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Remote-User", testUser)

	_, err := server.generateBearerTokenURL(req, &workspacev1alpha1.Workspace{}, accessStrategy)

	if err == nil {
		t.Error("expected error for missing BearerAuthURLTemplate, got nil")
	}
	if !strings.Contains(err.Error(), "BearerAuthURLTemplate not configured") {
		t.Errorf("expected template error, got: %v", err)
	}
}

// --- generateWebSocketConnectionURL tests ---

func TestGenerateWebSocketConnectionURL_Success(t *testing.T) {
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceMyWorkspace, Namespace: namespaceDefault},
		Spec: workspacev1alpha1.WorkspaceSpec{
			AccessStrategy: &workspacev1alpha1.AccessStrategyRef{Name: testStrategyWebSocket},
		},
	}

	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{Name: testStrategyWebSocket, Namespace: namespaceDefault},
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			BearerAuthURLTemplate: "https://myworkspace-default.example.com/ssh-ws",
		},
	}

	server := &ExtensionServer{
		config:        &ExtensionConfig{},
		signerFactory: &mockSignerFactory{signer: &mockSigner{token: testToken}},
	}

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Remote-User", testUser)

	connURL, err := server.generateWebSocketConnectionURL(req, workspace, accessStrategy)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(connURL, "wss://") {
		t.Errorf("expected wss:// scheme, got: %s", connURL)
	}
	if !strings.Contains(connURL, "token="+testToken) {
		t.Errorf("expected token in URL, got: %s", connURL)
	}
	if !strings.Contains(connURL, "myworkspace-default.example.com") {
		t.Errorf("expected host in URL, got: %s", connURL)
	}
}

func TestGenerateWebSocketConnectionURL_StripsBearerAuth(t *testing.T) {
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceMyWorkspace, Namespace: namespaceDefault},
		Spec: workspacev1alpha1.WorkspaceSpec{
			AccessStrategy: &workspacev1alpha1.AccessStrategyRef{Name: testStrategyWebSocket},
		},
	}

	// Template with /bearer-auth suffix (shared with web UI)
	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{Name: testStrategyWebSocket, Namespace: namespaceDefault},
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			BearerAuthURLTemplate: "https://myworkspace-default.example.com/bearer-auth",
		},
	}

	server := &ExtensionServer{
		config:        &ExtensionConfig{},
		signerFactory: &mockSignerFactory{signer: &mockSigner{token: testToken}},
	}

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Remote-User", testUser)

	url, err := server.generateWebSocketConnectionURL(req, workspace, accessStrategy)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should strip /bearer-auth from the URL
	if strings.Contains(url, "/bearer-auth") {
		t.Errorf("expected /bearer-auth to be stripped, got: %s", url)
	}
	if !strings.HasPrefix(url, "wss://") {
		t.Errorf("expected wss:// scheme, got: %s", url)
	}
}

func TestGenerateWebSocketConnectionURL_NoAccessStrategy(t *testing.T) {
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceMyWorkspace, Namespace: namespaceDefault},
	}

	server := &ExtensionServer{
		config:        &ExtensionConfig{},
		signerFactory: &mockSignerFactory{signer: &mockSigner{token: testToken}},
	}

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Remote-User", testUser)

	_, err := server.generateWebSocketConnectionURL(req, workspace, nil)

	if err == nil {
		t.Error("expected error for nil access strategy")
	}
}

func TestGenerateWebSocketConnectionURL_MissingUser(t *testing.T) {
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceMyWorkspace, Namespace: namespaceDefault},
	}

	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{Name: testStrategyWebSocket, Namespace: namespaceDefault},
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			BearerAuthURLTemplate: "https://myworkspace-default.example.com/ssh-ws",
		},
	}

	server := &ExtensionServer{
		config:        &ExtensionConfig{},
		signerFactory: &mockSignerFactory{signer: &mockSigner{token: testToken}},
	}

	req := httptest.NewRequest("POST", "/test", nil)
	// No X-Remote-User header

	_, err := server.generateWebSocketConnectionURL(req, workspace, accessStrategy)

	if err == nil {
		t.Error("expected error for missing user")
	}
}

func TestGenerateWebSocketConnectionURL_MissingTemplate(t *testing.T) {
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceMyWorkspace, Namespace: namespaceDefault},
	}

	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{Name: testStrategyWebSocket, Namespace: namespaceDefault},
		Spec:       workspacev1alpha1.WorkspaceAccessStrategySpec{},
	}

	server := &ExtensionServer{
		config:        &ExtensionConfig{},
		signerFactory: &mockSignerFactory{signer: &mockSigner{token: testToken}},
	}

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Remote-User", testUser)

	_, err := server.generateWebSocketConnectionURL(req, workspace, accessStrategy)

	if err == nil {
		t.Error("expected error for missing BearerAuthURLTemplate")
	}
}
