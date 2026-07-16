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

// ResourceValueProvider backs the {{ resource "<id>" "{jsonpath}" }} template function. It is the
// swappable seam that lets the controller resolve an integration two ways from the SAME template:
//
//   - liveResourceValueProvider (capture): reads the referenced resource live via JSONPath AND
//     records every (id, jsonPath) -> value it serves, so the controller can freeze the capture
//     into status.resolvedIntegrations. Used only when a re-resolve trigger changes (the integration's
//     parametersHash or observedIntegrationTemplateVersion).
//   - frozenResourceValueProvider (replay): serves values from a previously captured map and never
//     touches the referenced resource. Used on reconciles where neither trigger changed, so external
//     drift in the referenced resource cannot change the rendered pod template (no roll).
//
// Both implementations key on (id, jsonPath) via CaptureKey so a captured value replays under the
// exact expression that produced it.
type ResourceValueProvider interface {
	// Value returns the resolved string for resource <id> at <jsonPath>. Returning an error aborts
	// resolution (fail-closed): the caller must not apply a partially resolved overlay.
	Value(id, jsonPath string) (string, error)
}

// CaptureKey is the stable key under which a single {{ resource id "jsonPath" }} resolution is
// recorded and later replayed. Using both the id and the jsonPath makes each captured value
// self-describing and collision-free across resourceRefs.
func CaptureKey(id, jsonPath string) string {
	return id + "|" + jsonPath
}

// IntegrationTemplateResolver resolves template expressions in WorkspaceIntegrationTemplate fields.
// A resolver carries the ResourceValueProvider that backs {{ resource ... }} — capture or replay.
type IntegrationTemplateResolver struct {
	provider ResourceValueProvider
}

// NewIntegrationTemplateResolver creates a resolver whose {{ resource ... }} function reads from the
// given provider. Pass a live (capturing) provider on token change, or a frozen (replay) provider on
// an unchanged token.
func NewIntegrationTemplateResolver(provider ResourceValueProvider) *IntegrationTemplateResolver {
	return &IntegrationTemplateResolver{provider: provider}
}

// liveResourceValueProvider reads values from fetched resources via JSONPath and records each one it
// serves, so the full set can be frozen into status after a successful resolve.
type liveResourceValueProvider struct {
	resources map[string]*unstructured.Unstructured
	captured  map[string]string
}

// NewLiveResourceValueProvider builds a capturing provider over the fetched resources.
func NewLiveResourceValueProvider(resources map[string]*unstructured.Unstructured) *liveResourceValueProvider {
	return &liveResourceValueProvider{resources: resources, captured: map[string]string{}}
}

// Value reads resource <id> at <jsonPath> and records the result under CaptureKey(id, jsonPath).
func (p *liveResourceValueProvider) Value(id, jsonPath string) (string, error) {
	res, ok := p.resources[id]
	if !ok || res == nil {
		return "", fmt.Errorf("resource template references id %q but no such resource is available (id not declared in resourceRefs or not fetched)", id)
	}
	val, err := evaluateJSONPathExpression(res, jsonPath)
	if err != nil {
		return "", err
	}
	p.captured[CaptureKey(id, jsonPath)] = val
	return val, nil
}

// Captured returns the recorded (id|jsonPath) -> value map to freeze into status. The returned map
// is the provider's live map; copy it if the caller needs to retain it past the provider's lifetime.
func (p *liveResourceValueProvider) Captured() map[string]string {
	return p.captured
}

// frozenResourceValueProvider replays values recorded during an earlier live capture. It never reads
// the referenced resource, which is exactly what makes an unchanged-token reconcile drift-proof.
type frozenResourceValueProvider struct {
	values map[string]string
}

// NewFrozenResourceValueProvider builds a replay provider over a previously captured value map.
func NewFrozenResourceValueProvider(values map[string]string) *frozenResourceValueProvider {
	return &frozenResourceValueProvider{values: values}
}

// Value serves the frozen value for CaptureKey(id, jsonPath). A missing key is a hard error rather
// than a silent empty: it means the template referenced an expression that was not present when the
// values were frozen (e.g. a template edit added a new {{ resource ... }}), which must force a
// re-resolve (token change) rather than render an incomplete pod spec.
func (p *frozenResourceValueProvider) Value(id, jsonPath string) (string, error) {
	key := CaptureKey(id, jsonPath)
	val, ok := p.values[key]
	if !ok {
		return "", fmt.Errorf("frozen resolution has no value for %q; the template references an expression absent from the frozen set (a re-resolve trigger must change to re-resolve)", key)
	}
	return val, nil
}

// ResolveTemplateExpression resolves all template expressions in a single template-bearing string
// (templateExpression) in one pass. The input is a raw string that may contain {{ ... }} expressions
// (e.g. a container arg, an env value, a resourceRef name); the rendered literal string is returned.
//
// Two kinds of template expression are supported, both as ordinary Go text/template
// constructs:
//   - {{ .Workspace.Name }}, {{ .Workspace.Namespace }}, {{ .Parameters.X }} — workspace
//     metadata and user-supplied parameters
//   - {{ resource "<id>" "{jsonpath}" }} — a value served by the resolver's ResourceValueProvider
//     (live-capture from the referenced resource, or frozen-replay from status). Registered as a
//     template function (like b32encode in the access strategy renderer), so the JSONPath string —
//     including [N] indexing — is passed through untouched as a quoted function argument.
//
// Returns an error if: the template syntax is invalid, a referenced parameter key is missing, or the
// provider fails to serve a {{ resource }} expression (unknown id, invalid/empty JSONPath under
// capture, or a missing key under replay). All such errors are fail-closed — the caller must not
// apply a partially resolved overlay.
func (r *IntegrationTemplateResolver) ResolveTemplateExpression(templateExpression string, data IntegrationTemplateData) (string, error) {
	tmpl, err := template.New("integrationTemplateExpression").
		Option("missingkey=error").
		Funcs(template.FuncMap{
			"resource": func(id, jsonPath string) (string, error) {
				if r.provider == nil {
					return "", fmt.Errorf("resource template references id %q but resolver has no value provider", id)
				}
				return r.provider.Value(id, jsonPath)
			},
		}).
		Parse(templateExpression)
	if err != nil {
		return "", fmt.Errorf("invalid template syntax %q: %w", templateExpression, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render template %q: %w", templateExpression, err)
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
// the workspace namespace) before calling — see the freeze capture path (fetchResources), which
// sets effectiveRef.Metadata.Namespace when the field is empty. The empty checks here are a
// defense-in-depth guard against an un-defaulted ref or a template that evaluates to "".
func (r *IntegrationTemplateResolver) ResolveResourceRef(ref *workspacev1alpha1.ResourceRef, data IntegrationTemplateData) (resolvedName, resolvedNamespace string, err error) {
	if ref == nil {
		return "", "", fmt.Errorf("resourceRef is nil")
	}

	resolvedName, err = r.ResolveTemplateExpression(ref.Metadata.Name, data)
	if err != nil {
		return "", "", fmt.Errorf("resourceRef %q metadata.name template: %w", ref.Name, err)
	}
	if resolvedName == "" {
		return "", "", fmt.Errorf("resourceRef %q metadata.name resolved to empty string (template: %q)", ref.Name, ref.Metadata.Name)
	}

	resolvedNamespace, err = r.ResolveTemplateExpression(ref.Metadata.Namespace, data)
	if err != nil {
		return "", "", fmt.Errorf("resourceRef %q metadata.namespace template: %w", ref.Name, err)
	}
	if resolvedNamespace == "" {
		return "", "", fmt.Errorf("resourceRef %q metadata.namespace resolved to empty string (template: %q)", ref.Name, ref.Metadata.Namespace)
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
) (*workspacev1alpha1.PodModifications, error) {
	if mods == nil {
		return nil, nil
	}

	// Deep copy to avoid mutating the original CR data
	resolved := mods.DeepCopy()

	// Resolve additional containers
	for i := range resolved.AdditionalContainers {
		if err := r.resolveContainerFields(&resolved.AdditionalContainers[i], data); err != nil {
			return nil, fmt.Errorf("additionalContainers[%d] (container %q): %w",
				i, resolved.AdditionalContainers[i].Name, err)
		}
	}

	// Resolve init containers
	for i := range resolved.InitContainers {
		if err := r.resolveContainerFields(&resolved.InitContainers[i], data); err != nil {
			return nil, fmt.Errorf("initContainers[%d] (container %q): %w",
				i, resolved.InitContainers[i].Name, err)
		}
	}

	// Resolve primaryContainerModifications
	if resolved.PrimaryContainerModifications != nil {
		for i := range resolved.PrimaryContainerModifications.MergeEnv {
			env := &resolved.PrimaryContainerModifications.MergeEnv[i]
			val, err := r.ResolveTemplateExpression(env.ValueTemplate, data)
			if err != nil {
				return nil, fmt.Errorf("primaryContainerModifications mergeEnv %q valueTemplate: %w", env.Name, err)
			}
			env.ValueTemplate = val
		}
		if err := r.resolveVolumeMounts(resolved.PrimaryContainerModifications.VolumeMounts, data); err != nil {
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
) error {
	// Resolve image
	if container.Image != "" {
		val, err := r.ResolveTemplateExpression(container.Image, data)
		if err != nil {
			return fmt.Errorf("image field: %w", err)
		}
		container.Image = val
	}

	// Resolve command
	for i, cmd := range container.Command {
		val, err := r.ResolveTemplateExpression(cmd, data)
		if err != nil {
			return fmt.Errorf("command[%d]: %w", i, err)
		}
		container.Command[i] = val
	}

	// Resolve args
	for i, arg := range container.Args {
		val, err := r.ResolveTemplateExpression(arg, data)
		if err != nil {
			return fmt.Errorf("args[%d]: %w", i, err)
		}
		container.Args[i] = val
	}

	// Resolve workingDir
	if container.WorkingDir != "" {
		val, err := r.ResolveTemplateExpression(container.WorkingDir, data)
		if err != nil {
			return fmt.Errorf("workingDir: %w", err)
		}
		container.WorkingDir = val
	}

	// Resolve env values
	for i := range container.Env {
		if container.Env[i].Value != "" {
			val, err := r.ResolveTemplateExpression(container.Env[i].Value, data)
			if err != nil {
				return fmt.Errorf("env %q value: %w", container.Env[i].Name, err)
			}
			container.Env[i].Value = val
		}
	}

	// Resolve volume mount paths
	return r.resolveVolumeMounts(container.VolumeMounts, data)
}

// resolveVolumeMounts resolves template expressions in each volume mount's path fields:
// mountPath, subPath, subPathExpr. Mutates the mounts in place. The Name field is a reference
// to a declared volume and is left untouched.
func (r *IntegrationTemplateResolver) resolveVolumeMounts(
	mounts []corev1.VolumeMount,
	data IntegrationTemplateData,
) error {
	for i := range mounts {
		m := &mounts[i]
		if m.MountPath != "" {
			val, err := r.ResolveTemplateExpression(m.MountPath, data)
			if err != nil {
				return fmt.Errorf("volumeMounts[%d] (%q) mountPath: %w", i, m.Name, err)
			}
			m.MountPath = val
		}
		if m.SubPath != "" {
			val, err := r.ResolveTemplateExpression(m.SubPath, data)
			if err != nil {
				return fmt.Errorf("volumeMounts[%d] (%q) subPath: %w", i, m.Name, err)
			}
			m.SubPath = val
		}
		if m.SubPathExpr != "" {
			val, err := r.ResolveTemplateExpression(m.SubPathExpr, data)
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
