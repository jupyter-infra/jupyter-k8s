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

// IntegrationTemplateResolver resolves template expressions in WorkspaceIntegrationTemplate fields.
type IntegrationTemplateResolver struct{}

// NewIntegrationTemplateResolver creates a new resolver.
func NewIntegrationTemplateResolver() *IntegrationTemplateResolver {
	return &IntegrationTemplateResolver{}
}

// ResolveField resolves all template expressions in a single string field in one pass.
//
// Two kinds of template expression are supported, both as ordinary Go text/template
// constructs:
//   - {{ .Workspace.Name }}, {{ .Workspace.Namespace }}, {{ .Parameters.X }} — workspace
//     metadata and user-supplied parameters
//   - {{ resource "<id>" "{jsonpath}" }} — a value read from the fetched resource named
//     <id> in resourceRefs, via JSONPath. This is registered as a template function (like
//     b32encode in the access strategy renderer), so the JSONPath string — including [N]
//     indexing — is passed through untouched as a quoted function argument.
//
// resources maps each resourceRef id to its fetched object. Returns an error if: the
// template syntax is invalid, a referenced parameter key is missing, a {{ resource }}
// expression names an id with no fetched resource, the JSONPath is invalid, or the
// JSONPath evaluates to an empty string.
func (r *IntegrationTemplateResolver) ResolveField(field string, data IntegrationTemplateData, resources map[string]*unstructured.Unstructured) (string, error) {
	tmpl, err := template.New("field").
		Option("missingkey=error").
		Funcs(template.FuncMap{
			"resource": func(id, jsonPath string) (string, error) {
				res, ok := resources[id]
				if !ok || res == nil {
					return "", fmt.Errorf("resource template references id %q but no such resource is available (id not declared in resourceRefs or not fetched)", id)
				}
				return evaluateJSONPathExpression(res, jsonPath)
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

// ResolveResourceRef resolves template expressions in a resourceRef's name/namespace fields
// and validates they don't resolve to empty strings. The ref's name/namespace templates may
// only reference workspace metadata and parameters — not other resources — so resources is
// not threaded in here.
//
// Namespace is +optional on the CRD ("defaults to the workspace's namespace if omitted"), but
// this function does NOT apply that default: it requires both name and namespace to resolve to
// non-empty strings. The caller is responsible for pre-filling the namespace default (typically
// the workspace namespace) before calling — see the baker, which sets effectiveRef.Namespace
// when the field is empty. The empty checks here are a defense-in-depth guard against an
// un-defaulted ref or a template that evaluates to "".
func (r *IntegrationTemplateResolver) ResolveResourceRef(ref *workspacev1alpha1.ResourceRef, data IntegrationTemplateData) (resolvedName, resolvedNamespace string, err error) {
	if ref == nil {
		return "", "", fmt.Errorf("resourceRef is nil")
	}

	resolvedName, err = r.ResolveField(ref.Name, data, nil)
	if err != nil {
		return "", "", fmt.Errorf("resourceRef %q name template: %w", ref.ID, err)
	}
	if resolvedName == "" {
		return "", "", fmt.Errorf("resourceRef %q name resolved to empty string (template: %q)", ref.ID, ref.Name)
	}

	resolvedNamespace, err = r.ResolveField(ref.Namespace, data, nil)
	if err != nil {
		return "", "", fmt.Errorf("resourceRef %q namespace template: %w", ref.ID, err)
	}
	if resolvedNamespace == "" {
		return "", "", fmt.Errorf("resourceRef %q namespace resolved to empty string (template: %q)", ref.ID, ref.Namespace)
	}

	return resolvedName, resolvedNamespace, nil
}

// ResolvePodModifications resolves all template expressions across the templatable string
// fields in the pod modifications. The resolved-field contract is:
//   - each container in additionalContainers and initContainers: image, command[], args[],
//     workingDir, env[].value, and volumeMounts[].{mountPath,subPath,subPathExpr}
//   - primaryContainerModifications: mergeEnv[].valueTemplate and
//     volumeMounts[].{mountPath,subPath,subPathExpr}
//
// Pod-level volumes (mods.Volumes) are intentionally NOT resolved: a Volume source is a
// structured union (configMap names, hostPath, etc.), not a free-form string the integration
// author is expected to template. If that changes, extend this contract explicitly.
// Deep-copies the input before mutating so the original CR data is not modified.
func (r *IntegrationTemplateResolver) ResolvePodModifications(
	mods *workspacev1alpha1.PodModifications,
	data IntegrationTemplateData,
	resources map[string]*unstructured.Unstructured,
) (*workspacev1alpha1.PodModifications, error) {
	if mods == nil {
		return nil, nil
	}

	// Deep copy to avoid mutating the original CR data
	resolved := mods.DeepCopy()

	// Resolve additional containers
	for i := range resolved.AdditionalContainers {
		if err := r.resolveContainerFields(&resolved.AdditionalContainers[i], data, resources); err != nil {
			return nil, fmt.Errorf("additionalContainers[%d] (container %q): %w",
				i, resolved.AdditionalContainers[i].Name, err)
		}
	}

	// Resolve init containers
	for i := range resolved.InitContainers {
		if err := r.resolveContainerFields(&resolved.InitContainers[i], data, resources); err != nil {
			return nil, fmt.Errorf("initContainers[%d] (container %q): %w",
				i, resolved.InitContainers[i].Name, err)
		}
	}

	// Resolve primaryContainerModifications
	if resolved.PrimaryContainerModifications != nil {
		for i := range resolved.PrimaryContainerModifications.MergeEnv {
			env := &resolved.PrimaryContainerModifications.MergeEnv[i]
			val, err := r.ResolveField(env.ValueTemplate, data, resources)
			if err != nil {
				return nil, fmt.Errorf("primaryContainerModifications mergeEnv %q valueTemplate: %w", env.Name, err)
			}
			env.ValueTemplate = val
		}
		if err := r.resolveVolumeMounts(resolved.PrimaryContainerModifications.VolumeMounts, data, resources); err != nil {
			return nil, fmt.Errorf("primaryContainerModifications %w", err)
		}
	}

	return resolved, nil
}

// resolveContainerFields resolves template expressions in a container's string fields:
// image, command[], args[], workingDir, env[].value, and volumeMounts[].{mountPath,subPath,subPathExpr}.
func (r *IntegrationTemplateResolver) resolveContainerFields(
	container *corev1.Container,
	data IntegrationTemplateData,
	resources map[string]*unstructured.Unstructured,
) error {
	// Resolve image
	if container.Image != "" {
		val, err := r.ResolveField(container.Image, data, resources)
		if err != nil {
			return fmt.Errorf("image field: %w", err)
		}
		container.Image = val
	}

	// Resolve command
	for i, cmd := range container.Command {
		val, err := r.ResolveField(cmd, data, resources)
		if err != nil {
			return fmt.Errorf("command[%d]: %w", i, err)
		}
		container.Command[i] = val
	}

	// Resolve args
	for i, arg := range container.Args {
		val, err := r.ResolveField(arg, data, resources)
		if err != nil {
			return fmt.Errorf("args[%d]: %w", i, err)
		}
		container.Args[i] = val
	}

	// Resolve workingDir
	if container.WorkingDir != "" {
		val, err := r.ResolveField(container.WorkingDir, data, resources)
		if err != nil {
			return fmt.Errorf("workingDir: %w", err)
		}
		container.WorkingDir = val
	}

	// Resolve env values
	for i := range container.Env {
		if container.Env[i].Value != "" {
			val, err := r.ResolveField(container.Env[i].Value, data, resources)
			if err != nil {
				return fmt.Errorf("env %q value: %w", container.Env[i].Name, err)
			}
			container.Env[i].Value = val
		}
	}

	// Resolve volume mount paths
	if err := r.resolveVolumeMounts(container.VolumeMounts, data, resources); err != nil {
		return err
	}

	return nil
}

// resolveVolumeMounts resolves template expressions in each volume mount's path fields:
// mountPath, subPath, subPathExpr. Mutates the mounts in place. The Name field is a reference
// to a declared volume and is left untouched.
func (r *IntegrationTemplateResolver) resolveVolumeMounts(
	mounts []corev1.VolumeMount,
	data IntegrationTemplateData,
	resources map[string]*unstructured.Unstructured,
) error {
	for i := range mounts {
		m := &mounts[i]
		if m.MountPath != "" {
			val, err := r.ResolveField(m.MountPath, data, resources)
			if err != nil {
				return fmt.Errorf("volumeMounts[%d] (%q) mountPath: %w", i, m.Name, err)
			}
			m.MountPath = val
		}
		if m.SubPath != "" {
			val, err := r.ResolveField(m.SubPath, data, resources)
			if err != nil {
				return fmt.Errorf("volumeMounts[%d] (%q) subPath: %w", i, m.Name, err)
			}
			m.SubPath = val
		}
		if m.SubPathExpr != "" {
			val, err := r.ResolveField(m.SubPathExpr, data, resources)
			if err != nil {
				return fmt.Errorf("volumeMounts[%d] (%q) subPathExpr: %w", i, m.Name, err)
			}
			m.SubPathExpr = val
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
