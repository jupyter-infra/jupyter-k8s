package controller

import (
	"bytes"
	"encoding/base32"
	"fmt"
	"strings"
	"text/template"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

// TODO: Remove this duplicate function once import cycle is resolved
// This is a copy of workspace.EncodeNamespaceB32 to avoid import cycle
func encodeNamespaceB32(namespace string) string {
	encoded := base32.StdEncoding.EncodeToString([]byte(namespace))
	return strings.ToLower(strings.TrimRight(encoded, "="))
}

// AccessResourcesBuilder builds resources for WorkspaceAccessStrategy
type AccessResourcesBuilder struct {
	// No fields needed currently, may add in future
}

// NewAccessResourcesBuilder creates a new AccessResourcesBuilder
func NewAccessResourcesBuilder() *AccessResourcesBuilder {
	return &AccessResourcesBuilder{}
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
		"b32encode": encodeNamespaceB32,
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
	labels[LabelWorkspaceName] = workspace.Name
	labels[LabelWorkspaceNamespace] = workspace.Namespace
	labels[LabelAccessStrategyName] = accessStrategy.Name
	labels[LabelAccessStrategyNamespace] = accessStrategyNamespace
	obj.SetLabels(labels)

	return obj, nil
}

// ResolveAccessURL processes the AccessURLTemplate
func (b *AccessResourcesBuilder) ResolveAccessURL(
	workspace *workspacev1alpha1.Workspace,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
	service *corev1.Service,
) (string, error) {
	accessUrlTemplate := accessStrategy.Spec.AccessURLTemplate

	// no URL Template in the accessStrategy -> no AccessUrl
	if accessUrlTemplate == "" {
		return "", nil
	}

	// Resolve the AccessURLTemplate using the template engine
	tmpl, err := template.New("accessURL").Funcs(template.FuncMap{
		"b32encode": encodeNamespaceB32,
	}).Parse(accessUrlTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse AccessURLTemplate: %w", err)
	}

	// Create template data
	accessResourceData := &fullAccessResourceData{
		Workspace:      workspace,
		AccessStrategy: accessStrategy,
		Service:        service,
	}

	// Execute template
	var accessURLBuffer bytes.Buffer
	if err := tmpl.Execute(&accessURLBuffer, accessResourceData); err != nil {
		return "", fmt.Errorf("failed to execute AccessURLTemplate: %w", err)
	}

	return accessURLBuffer.String(), nil
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
	return fmt.Sprintf("%s=%s", LabelWorkspaceName, workspace.Name)
}
