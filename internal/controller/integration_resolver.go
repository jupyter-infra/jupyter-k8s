/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"bytes"
	"fmt"
	"text/template"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/util/jsonpath"
)

// IntegrationTemplateData provides the context for Go text/template resolution.
type IntegrationTemplateData struct {
	Workspace  IntegrationWorkspaceData
	Parameters map[string]string
}

// IntegrationWorkspaceData holds workspace identity fields available in templates.
type IntegrationWorkspaceData struct {
	Name      string
	Namespace string
}

// IntegrationResolver resolves template expressions in WorkspaceIntegrationStrategy fields.
type IntegrationResolver struct{}

// NewIntegrationResolver creates a new resolver.
func NewIntegrationResolver() *IntegrationResolver {
	return &IntegrationResolver{}
}

// ResolveField resolves all template expressions in a single string field in one pass.
//
// Two kinds of template expression are supported, both as ordinary Go text/template
// constructs:
//   - {{ .Workspace.Name }}, {{ .Workspace.Namespace }}, {{ .Parameters.X }} — workspace
//     metadata and user-supplied parameters
//   - {{ resource "{jsonpath}" }} — a value read from the fetched lookup resource via
//     JSONPath. This is registered as a template function (like b32encode in the access
//     strategy renderer), so the JSONPath string — including [N] indexing — is passed
//     through untouched as a quoted function argument.
//
// Returns an error if: the template syntax is invalid, a referenced parameter key is
// missing, a {{ resource }} expression is used without a fetched resource, the JSONPath
// is invalid, or the JSONPath evaluates to an empty string.
func (r *IntegrationResolver) ResolveField(field string, data IntegrationTemplateData, resource *unstructured.Unstructured) (string, error) {
	tmpl, err := template.New("field").
		Option("missingkey=error").
		Funcs(template.FuncMap{
			"resource": func(jsonPath string) (string, error) {
				if resource == nil {
					return "", fmt.Errorf("resource template %q used but no resource available (resourceLookup not defined or resource not fetched)", jsonPath)
				}
				return evaluateJSONPathExpression(resource, jsonPath)
			},
		}).
		Parse(field)
	if err != nil {
		return "", fmt.Errorf("invalid template syntax %q: %w", field, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("template execution failed %q: %w", field, err)
	}

	return buf.String(), nil
}

// ResolveResourceLookup resolves template expressions in resourceLookup name/namespace fields
// and validates they don't resolve to empty strings.
func (r *IntegrationResolver) ResolveResourceLookup(lookup *workspacev1alpha1.ResourceLookup, data IntegrationTemplateData) (resolvedName, resolvedNamespace string, err error) {
	if lookup == nil {
		return "", "", fmt.Errorf("resourceLookup is nil")
	}

	resolvedName, err = r.ResolveField(lookup.Name, data, nil)
	if err != nil {
		return "", "", fmt.Errorf("resourceLookup name template: %w", err)
	}
	if resolvedName == "" {
		return "", "", fmt.Errorf("resourceLookup name resolved to empty string (template: %q)", lookup.Name)
	}

	resolvedNamespace, err = r.ResolveField(lookup.Namespace, data, nil)
	if err != nil {
		return "", "", fmt.Errorf("resourceLookup namespace template: %w", err)
	}
	if resolvedNamespace == "" {
		return "", "", fmt.Errorf("resourceLookup namespace resolved to empty string (template: %q)", lookup.Namespace)
	}

	return resolvedName, resolvedNamespace, nil
}

// ResolvePodModifications resolves all template expressions across all string fields
// in the pod modifications. Fields to resolve in each container: image, command[], args[], env[].value.
// Also resolves mergeEnv[].valueTemplate in primaryContainerModifications.
// Deep-copies the input before mutating so the original CR data is not modified.
func (r *IntegrationResolver) ResolvePodModifications(
	mods *workspacev1alpha1.PodModifications,
	data IntegrationTemplateData,
	resource *unstructured.Unstructured,
) (*workspacev1alpha1.PodModifications, error) {
	if mods == nil {
		return nil, nil
	}

	// Deep copy to avoid mutating the original CR data
	resolved := mods.DeepCopy()

	// Resolve additional containers
	for i := range resolved.AdditionalContainers {
		if err := r.resolveContainerFields(&resolved.AdditionalContainers[i], data, resource); err != nil {
			return nil, fmt.Errorf("additionalContainers[%d] (container %q): %w",
				i, resolved.AdditionalContainers[i].Name, err)
		}
	}

	// Resolve init containers
	for i := range resolved.InitContainers {
		if err := r.resolveContainerFields(&resolved.InitContainers[i], data, resource); err != nil {
			return nil, fmt.Errorf("initContainers[%d] (container %q): %w",
				i, resolved.InitContainers[i].Name, err)
		}
	}

	// Resolve primaryContainerModifications
	if resolved.PrimaryContainerModifications != nil {
		for i := range resolved.PrimaryContainerModifications.MergeEnv {
			env := &resolved.PrimaryContainerModifications.MergeEnv[i]
			val, err := r.ResolveField(env.ValueTemplate, data, resource)
			if err != nil {
				return nil, fmt.Errorf("primaryContainerModifications mergeEnv %q valueTemplate: %w", env.Name, err)
			}
			env.ValueTemplate = val
		}
	}

	return resolved, nil
}

// resolveContainerFields resolves template expressions in a container's string fields:
// image, command[], args[], env[].value.
func (r *IntegrationResolver) resolveContainerFields(
	container *corev1.Container,
	data IntegrationTemplateData,
	resource *unstructured.Unstructured,
) error {
	// Resolve image
	if container.Image != "" {
		val, err := r.ResolveField(container.Image, data, resource)
		if err != nil {
			return fmt.Errorf("image field: %w", err)
		}
		container.Image = val
	}

	// Resolve command
	for i, cmd := range container.Command {
		val, err := r.ResolveField(cmd, data, resource)
		if err != nil {
			return fmt.Errorf("command[%d]: %w", i, err)
		}
		container.Command[i] = val
	}

	// Resolve args
	for i, arg := range container.Args {
		val, err := r.ResolveField(arg, data, resource)
		if err != nil {
			return fmt.Errorf("args[%d]: %w", i, err)
		}
		container.Args[i] = val
	}

	// Resolve env values
	for i := range container.Env {
		if container.Env[i].Value != "" {
			val, err := r.ResolveField(container.Env[i].Value, data, resource)
			if err != nil {
				return fmt.Errorf("env %q value: %w", container.Env[i].Name, err)
			}
			container.Env[i].Value = val
		}
	}

	return nil
}

// evaluateJSONPathExpression evaluates a JSONPath expression against an unstructured resource.
func evaluateJSONPathExpression(obj *unstructured.Unstructured, expression string) (string, error) {
	jp := jsonpath.New("fieldPath")
	if err := jp.Parse(expression); err != nil {
		return "", fmt.Errorf("invalid JSONPath %q: %w", expression, err)
	}

	var buf bytes.Buffer
	if err := jp.Execute(&buf, obj.Object); err != nil {
		return "", fmt.Errorf("JSONPath %q evaluation failed: %w", expression, err)
	}

	result := buf.String()
	if result == "" {
		return "", fmt.Errorf("JSONPath %q returned empty result", expression)
	}

	return result, nil
}
