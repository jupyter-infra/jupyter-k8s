/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/template"

	"github.com/jupyter-infra/jupyter-k8s-plugin/plugin"
	connectionv1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/connection/v1alpha1"
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

// AccessResourcesBuilder builds resources for WorkspaceAccessStrategy
type AccessResourcesBuilder struct {
	// plugins holds the names of the plugin sidecars the controller is configured with, the same list the
	// Extension API serves connection requests from.
	plugins map[string]bool
}

// NewAccessResourcesBuilder creates a new AccessResourcesBuilder
func NewAccessResourcesBuilder(plugins ...string) *AccessResourcesBuilder {
	b := &AccessResourcesBuilder{plugins: map[string]bool{}}
	for _, name := range plugins {
		b.plugins[name] = true
	}
	return b
}

// fullAccessResourceData provides values for template substitutions
type fullAccessResourceData struct {
	Workspace      *workspacev1alpha1.Workspace
	AccessStrategy *workspacev1alpha1.WorkspaceAccessStrategy
	Service        *corev1.Service
}

// BuildUnstructuredResource builds an unstructured resource from a template
func (b *AccessResourcesBuilder) BuildUnstructuredResource(
	accessResourceTemplate workspacev1alpha1.AccessResourceTemplate,
	workspace *workspacev1alpha1.Workspace,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
	service *corev1.Service,
) (*unstructured.Unstructured, error) {
	// Generate resource name using NamePrefix and workspace name
	name := fmt.Sprintf("%s-%s", accessResourceTemplate.NamePrefix, workspace.Name)

	// Process resource template
	resourceTmpl, err := template.New("resource").Funcs(template.FuncMap{
		"b32encode": workspaceutil.EncodeNamespaceB32,
	}).Parse(accessResourceTemplate.Template)
	if err != nil {
		return nil, fmt.Errorf("failed to parse resource template: %w", err)
	}

	accessResourceData := &fullAccessResourceData{
		Workspace:      workspace,
		AccessStrategy: accessStrategy,
		Service:        service,
	}

	var resourceBuffer bytes.Buffer
	if err := resourceTmpl.Execute(&resourceBuffer, accessResourceData); err != nil {
		return nil, fmt.Errorf("failed to execute resource template: %w", err)
	}
	resourceYAML := resourceBuffer.String()

	// First add apiVersion and kind to the YAML
	yamlWithMeta := fmt.Sprintf("apiVersion: %s\nkind: %s\n%s",
		accessResourceTemplate.ApiVersion,
		accessResourceTemplate.Kind,
		resourceYAML)

	// Convert YAML to unstructured.Unstructured
	obj := &unstructured.Unstructured{}
	if err := yaml.Unmarshal([]byte(yamlWithMeta), obj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal resource YAML: %w", err)
	}

	// Set basic metadata
	obj.SetName(name)

	// The AccessResource MUST be in the Workspace namespace
	// in order for the Workspace is the owner of the AccessResource
	targetNamespace := accessResourceData.Workspace.Namespace
	obj.SetNamespace(targetNamespace)

	// Set Group-Version-Kind properly by parsing the API version
	apiVersionParts := strings.Split(accessResourceTemplate.ApiVersion, "/")
	var group, version string
	if len(apiVersionParts) == 2 {
		group = apiVersionParts[0]
		version = apiVersionParts[1]
	} else {
		// Core API group uses version without group prefix
		version = accessResourceTemplate.ApiVersion
	}

	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   group,
		Version: version,
		Kind:    accessResourceTemplate.Kind,
	})

	// Add labels to the access resource referring to:
	// - the Workspace
	// - AccessStrategy
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	accessStrategyNamespace := accessStrategy.Namespace
	if accessStrategyNamespace == "" {
		accessStrategyNamespace = workspace.Namespace
	}
	labels[workspaceutil.LabelWorkspaceName] = workspace.Name
	labels[LabelWorkspaceNamespace] = workspace.Namespace
	labels[LabelAccessStrategyName] = accessStrategy.Name
	labels[LabelAccessStrategyNamespace] = accessStrategyNamespace
	obj.SetLabels(labels)

	return obj, nil
}

// ResolveTemplateURL resolves a Go text/template URL string using workspace, access strategy,
// and service data. Shared by ResolveAccessURL and the access startup prober.
func (b *AccessResourcesBuilder) ResolveTemplateURL(
	urlTemplate string,
	workspace *workspacev1alpha1.Workspace,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
	service *corev1.Service,
) (string, error) {
	tmpl, err := template.New("url").Funcs(template.FuncMap{
		"b32encode": workspaceutil.EncodeNamespaceB32,
	}).Parse(urlTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL template: %w", err)
	}

	data := &fullAccessResourceData{
		Workspace:      workspace,
		AccessStrategy: accessStrategy,
		Service:        service,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute URL template: %w", err)
	}

	return buf.String(), nil
}

// ResolveAccessURL processes the AccessURLTemplate
func (b *AccessResourcesBuilder) ResolveAccessURL(
	workspace *workspacev1alpha1.Workspace,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
	service *corev1.Service,
) (string, error) {
	if accessStrategy.Spec.AccessURLTemplate == "" {
		return "", nil
	}
	return b.ResolveTemplateURL(accessStrategy.Spec.AccessURLTemplate, workspace, accessStrategy, service)
}

// ResolveApplicationBasePath resolves the access strategy's applicationBasePathTemplate.
// Returns "" when the field is absent or on template error (resolveIdlePath treats "" as no prefix).
func (b *AccessResourcesBuilder) ResolveApplicationBasePath(
	workspace *workspacev1alpha1.Workspace,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
	service *corev1.Service,
) (string, error) {
	if accessStrategy.Spec.ApplicationBasePathTemplate == "" {
		return "", nil
	}
	resolved, err := b.ResolveTemplateURL(accessStrategy.Spec.ApplicationBasePathTemplate, workspace, accessStrategy, service)
	if err != nil {
		return "", err
	}
	return resolved, nil
}

// ResolveConnectionTypes lists the WorkspaceConnection types the Extension API accepts for an access strategy,
// in the order it resolves a request: web-ui, ssh-over-websocket, then the {ide}-remote keys of
// createConnectionHandlerMap sorted, each kept only when its handler is served (servesHandler). A key outside
// the {ide}-remote pattern is refused before the strategy is read, and the default createConnectionHandler
// serves an open set, so neither is listed. Returns nil when nothing is configured: an absent status field
// decodes to nil, so the status manager sees no change on the next reconcile.
func (b *AccessResourcesBuilder) ResolveConnectionTypes(
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
) []string {
	var connectionTypes []string
	if accessStrategy.Spec.BearerAuthURLTemplate != "" {
		connectionTypes = append(connectionTypes, connectionv1alpha1.ConnectionTypeWebUI)
	}
	if accessStrategy.Spec.WebSocketURLTemplate != "" {
		connectionTypes = append(connectionTypes, connectionv1alpha1.ConnectionTypeWebSocket)
	}
	var remoteTypes []string
	for connectionType, handlerRef := range accessStrategy.Spec.CreateConnectionHandlerMap {
		if connectionv1alpha1.IsRemoteConnectionType(connectionType) && b.servesHandler(accessStrategy, handlerRef) {
			remoteTypes = append(remoteTypes, connectionType)
		}
	}
	sort.Strings(remoteTypes)
	return append(connectionTypes, remoteTypes...)
}

// servesHandler reports whether a connection request resolved to the handler reference gets a URL from the
// Extension API, which refuses k8s-native without the WebSocket route, a plugin it has no client for, and a
// plugin action other than createSession.
func (b *AccessResourcesBuilder) servesHandler(
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy, handlerRef string,
) bool {
	pluginName, action := plugin.ParseHandlerRef(handlerRef)
	if pluginName == workspacev1alpha1.CreateConnectionHandlerK8sNative {
		return accessStrategy.Spec.WebSocketURLTemplate != ""
	}
	return b.plugins[pluginName] && (action == "" || action == workspacev1alpha1.CreateConnectionActionCreateSession)
}

// ResolveAccessResourceSelector creates a label selector string for finding access resources
// associated with a specific workspace and access strategy
func (b *AccessResourcesBuilder) ResolveAccessResourceSelector(
	workspace *workspacev1alpha1.Workspace,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
) string {
	hasAccessResources := len(accessStrategy.Spec.AccessResourceTemplates) > 0

	// if the AccessStrategy does not define AccessResources, do not set a selector.
	if !hasAccessResources {
		return ""
	}

	// Format: key1=value1,key2=value2
	return fmt.Sprintf("%s=%s", workspaceutil.LabelWorkspaceName, workspace.Name)
}
