package controller

import (
	"bytes"
	"fmt"
	"text/template"

	workspacesv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

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
	Workspace      *workspacesv1alpha1.Workspace
	AccessStrategy *workspacesv1alpha1.WorkspaceAccessStrategy
	Service        *corev1.Service
}

// BuildUnstructuredResource builds an unstructured resource from a template
func (b *AccessResourcesBuilder) BuildUnstructuredResource(
	accessResourceTemplate workspacesv1alpha1.AccessResourceTemplate,
	workspace *workspacesv1alpha1.Workspace,
	accessStrategy *workspacesv1alpha1.WorkspaceAccessStrategy,
	service *corev1.Service,
) (*unstructured.Unstructured, error) {
	// Generate resource name using NamePrefix and workspace name
	name := fmt.Sprintf("%s-%s", accessResourceTemplate.NamePrefix, workspace.Name)

	// Process resource template
	resourceTmpl, err := template.New("resource").Parse(accessResourceTemplate.Template)
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

	// Convert YAML to unstructured.Unstructured
	obj := &unstructured.Unstructured{}
	if err := yaml.Unmarshal([]byte(resourceYAML), obj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal resource YAML: %w", err)
	}

	// Set basic metadata
	obj.SetName(name)

	// Determine the target namespace
	targetNamespace := accessResourceData.Workspace.Namespace
	if accessStrategy.Spec.AccessResourcesNamespace != "" {
		targetNamespace = accessStrategy.Spec.AccessResourcesNamespace
	}
	obj.SetNamespace(targetNamespace)

	// Set GVK
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "", // Will be parsed from ApiVersion
		Version: accessResourceTemplate.ApiVersion,
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
	workspace *workspacesv1alpha1.Workspace,
	accessStrategy *workspacesv1alpha1.WorkspaceAccessStrategy,
	service *corev1.Service,
) (string, error) {
	accessUrlTemplate := accessStrategy.Spec.AccessURLTemplate

	// no URL Template in the accessStrategy -> no AccessUrl
	if accessUrlTemplate == "" {
		return "", nil
	}

	// Resolve the AccessURLTemplate using the template engine
	tmpl, err := template.New("accessURL").Parse(accessUrlTemplate)
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
