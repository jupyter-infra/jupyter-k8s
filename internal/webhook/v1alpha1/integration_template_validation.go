/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"errors"
	"fmt"
	"io"
	"text/template"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/jsonpath"
)

// Static validation for a WorkspaceIntegrationTemplate at admission time -- the checks that need no live
// referenced resource, so authoring mistakes are rejected at the template author's write instead of
// surfacing later as degraded workspaces. validateIntegrationTemplate checks a template's own fields:
// syntax, every {{ resource }} names a declared resourceRef with a valid JSONPath, and every
// {{ .Parameters.X }} names a parameter declared in spec.parameters (a typo'd reference is rejected).
//
// The complementary workspace-side check (a Workspace supplies every parameter its template declares)
// is validateWorkspaceIntegrationParameters, invoked per-ref by IntegrationTemplateRefValidator.

// templateValidationData is the render context static validation renders each templated field against.
// Its fields ARE the allowlist of top-level references a template may use: .Parameters.<declared> and
// .Workspace.Name/.Namespace. Rendered under missingkey=error (see validateTemplateExpression), so an
// undeclared .Parameters.X or an unknown .Workspace field errors. It mirrors the field shape of the
// controller resolver's live render context; if that context gains a field a template may reference, add
// it here too or a valid template using it would fail validation.
//
// Workspace MUST stay a typed struct (not a map): a struct-field reference like {{ .Workspace.Nme }} is
// a hard text/template error (can't evaluate field Nme), which is what catches a .Workspace typo at
// admission. missingkey=error only covers map keys (that is why .Parameters is a map -- its declared
// keys are seeded), so demoting Workspace to a map would silently render an unknown field to empty and
// let the typo through. TestValidateIntegrationTemplate_WorkspaceFieldTypoRejected locks this in.
type templateValidationData struct {
	Workspace  templateValidationWorkspace
	Parameters map[string]string
}

type templateValidationWorkspace struct {
	Name      string
	Namespace string
}

// validateIntegrationTemplate statically validates a template's fields without any read. Returns a
// non-nil error describing the first problem.
func validateIntegrationTemplate(tmpl *workspacev1alpha1.WorkspaceIntegrationTemplate) error {
	if tmpl == nil {
		return errors.New("template is nil")
	}

	// 1. Build the allowlists every templated field is validated against: the names a template expression
	//    is permitted to reference. allowedParamNames holds the parameter names declared in spec.parameters
	//    ({{ .Parameters.X }} may only use these); allowedResourceRefIDs holds the resourceRef handles
	//    ({{ resource "id" ... }} may only name these). allowedParamNames is a map[string]string (not a
	//    set) because it doubles as the render data below -- seeding the declared names with empty values
	//    lets one template Execute reject an undeclared {{ .Parameters.typo }} via missingkey=error.
	allowedParamNames := make(map[string]string, len(tmpl.Spec.Parameters))
	for i := range tmpl.Spec.Parameters {
		allowedParamNames[tmpl.Spec.Parameters[i].Name] = ""
	}
	allowedResourceRefIDs := make(map[string]bool, len(tmpl.Spec.ResourceRefs))
	for i := range tmpl.Spec.ResourceRefs {
		allowedResourceRefIDs[tmpl.Spec.ResourceRefs[i].Name] = true
	}

	// 2. A resourceRef's own metadata.name/namespace may interpolate .Workspace/.Parameters but NEVER
	//    {{ resource }} (they identify the resource to fetch, so can't depend on a fetched one).
	for i := range tmpl.Spec.ResourceRefs {
		ref := &tmpl.Spec.ResourceRefs[i]
		if err := validateMetadataExpression(ref.Metadata.Name, allowedParamNames); err != nil {
			return fmt.Errorf("resourceRef %q metadata.name: %w", ref.Name, err)
		}
		if ref.Metadata.Namespace != "" {
			if err := validateMetadataExpression(ref.Metadata.Namespace, allowedParamNames); err != nil {
				return fmt.Errorf("resourceRef %q metadata.namespace: %w", ref.Name, err)
			}
		}
	}

	// 3. The statusProbe exec command is resolver-rendered too (the operator resolves its {{ resource }} /
	//    {{ .Parameters }} from the frozen values before exec'ing it -- see controller.resolveStatusProbeCommand),
	//    so its args must be validated with the same rules as pod-mod fields. Checked before the pod-mods
	//    early return, since a template may declare a statusProbe without any deploymentModifications.
	if tmpl.Spec.StatusProbe != nil && tmpl.Spec.StatusProbe.Exec != nil {
		for j, arg := range tmpl.Spec.StatusProbe.Exec.Command {
			if err := validatePodFieldExpression(arg, allowedParamNames, allowedResourceRefIDs); err != nil {
				return fmt.Errorf("statusProbe.exec.command[%d]: %w", j, err)
			}
		}
	}

	// 4. Pod-mod fields may use {{ resource }} against a declared resourceRef.
	if tmpl.Spec.DeploymentModifications == nil || tmpl.Spec.DeploymentModifications.PodModifications == nil {
		return nil
	}
	pm := tmpl.Spec.DeploymentModifications.PodModifications
	for i := range pm.AdditionalContainers {
		if err := validateContainerTemplates(&pm.AdditionalContainers[i], allowedParamNames, allowedResourceRefIDs); err != nil {
			return fmt.Errorf("additionalContainers[%d] (%q): %w", i, pm.AdditionalContainers[i].Name, err)
		}
	}
	for i := range pm.InitContainers {
		if err := validateContainerTemplates(&pm.InitContainers[i], allowedParamNames, allowedResourceRefIDs); err != nil {
			return fmt.Errorf("initContainers[%d] (%q): %w", i, pm.InitContainers[i].Name, err)
		}
	}
	if pm.PrimaryContainerModifications != nil {
		for i := range pm.PrimaryContainerModifications.MergeEnv {
			env := &pm.PrimaryContainerModifications.MergeEnv[i]
			if err := validatePodFieldExpression(env.ValueTemplate, allowedParamNames, allowedResourceRefIDs); err != nil {
				return fmt.Errorf("primaryContainerModifications.mergeEnv[%q]: %w", env.Name, err)
			}
		}
		if err := validateVolumeMountTemplates(pm.PrimaryContainerModifications.VolumeMounts, allowedParamNames, allowedResourceRefIDs); err != nil {
			return fmt.Errorf("primaryContainerModifications.%w", err)
		}
	}
	return nil
}

// validateContainerTemplates checks every templated string field of a container (pod-mod context, so
// {{ resource }} is allowed against a declared resourceRef): image, command, args, workingDir, env value,
// and volume-mount paths. This set must stay in step with the fields the controller's resolver renders --
// an unvalidated resolver-rendered field is one whose author typo escapes admission and surfaces later as
// a degraded workspace.
func validateContainerTemplates(c *corev1.Container, allowedParamNames map[string]string, allowedResourceRefIDs map[string]bool) error {
	if err := validatePodFieldExpression(c.Image, allowedParamNames, allowedResourceRefIDs); err != nil {
		return fmt.Errorf("image: %w", err)
	}
	for j, cmd := range c.Command {
		if err := validatePodFieldExpression(cmd, allowedParamNames, allowedResourceRefIDs); err != nil {
			return fmt.Errorf("command[%d]: %w", j, err)
		}
	}
	for j, arg := range c.Args {
		if err := validatePodFieldExpression(arg, allowedParamNames, allowedResourceRefIDs); err != nil {
			return fmt.Errorf("args[%d]: %w", j, err)
		}
	}
	if err := validatePodFieldExpression(c.WorkingDir, allowedParamNames, allowedResourceRefIDs); err != nil {
		return fmt.Errorf("workingDir: %w", err)
	}
	for j := range c.Env {
		if err := validatePodFieldExpression(c.Env[j].Value, allowedParamNames, allowedResourceRefIDs); err != nil {
			return fmt.Errorf("env[%q]: %w", c.Env[j].Name, err)
		}
	}
	return validateVolumeMountTemplates(c.VolumeMounts, allowedParamNames, allowedResourceRefIDs)
}

// validateVolumeMountTemplates checks the templated path fields of every volume mount (mountPath,
// subPath, subPathExpr) -- the same fields the controller's resolver renders. Shared by container mounts
// and the primary-container modifications so the two cannot drift.
func validateVolumeMountTemplates(mounts []corev1.VolumeMount, allowedParamNames map[string]string, allowedResourceRefIDs map[string]bool) error {
	for j := range mounts {
		m := &mounts[j]
		if err := validatePodFieldExpression(m.MountPath, allowedParamNames, allowedResourceRefIDs); err != nil {
			return fmt.Errorf("volumeMounts[%q].mountPath: %w", m.Name, err)
		}
		if err := validatePodFieldExpression(m.SubPath, allowedParamNames, allowedResourceRefIDs); err != nil {
			return fmt.Errorf("volumeMounts[%q].subPath: %w", m.Name, err)
		}
		if err := validatePodFieldExpression(m.SubPathExpr, allowedParamNames, allowedResourceRefIDs); err != nil {
			return fmt.Errorf("volumeMounts[%q].subPathExpr: %w", m.Name, err)
		}
	}
	return nil
}

// validateMetadataExpression validates a resourceRef metadata.name/namespace expression: .Workspace and
// .Parameters are allowed, but {{ resource }} is not (metadata identifies the resource to fetch, so it
// cannot depend on a fetched one). No resourceRef allowlist is needed since {{ resource }} is rejected.
func validateMetadataExpression(field string, allowedParamNames map[string]string) error {
	return validateTemplateExpression(field, allowedParamNames, nil, false)
}

// validatePodFieldExpression validates a pod-modification expression (container image/command/args/env,
// mergeEnv): .Workspace, .Parameters, and {{ resource "<id>" ... }} against a declared resourceRef.
func validatePodFieldExpression(field string, allowedParamNames map[string]string, allowedResourceRefIDs map[string]bool) error {
	return validateTemplateExpression(field, allowedParamNames, allowedResourceRefIDs, true)
}

// validateTemplateExpression parses and executes one templated field with missingkey=error, so a single
// pass catches: bad Go-template syntax; an undeclared {{ .Parameters.X }} or bad {{ .Workspace.X }}
// (missingkey=error, since allowedParamNames seeds only the declared names); and, via the resource func,
// a {{ resource }} that is disallowed here (allowResourceRefs=false), names an undeclared resourceRef,
// or carries an invalid JSONPath. The rendered output is discarded -- only errors matter. Prefer the
// validateMetadataExpression / validatePodFieldExpression wrappers over calling this directly.
//
// KNOWN LIMITATIONS of static validation, both of which let an undeclared/typo'd parameter reference
// slip past here and surface only as a degraded workspace at resolve time:
//
//  1. Dynamic-key access: missingkey=error only fires for the field-access form {{ .Parameters.X }}. The
//     dynamic-key form {{ index .Parameters "X" }} bypasses missingkey entirely -- text/template's index
//     builtin returns the map's zero value (empty string) for an unknown key rather than erroring -- so a
//     typo'd or undeclared key resolves silently to empty and passes validation.
//  2. Untaken conditional branches: validation seeds every declared parameter to the empty string (its
//     name, not a value -- see templateValidationData), so a {{ if .Parameters.X }} / {{ range ... }}
//     guard is falsy and its body is never executed during this render. Any {{ .Parameters.<undeclared> }}
//     or {{ resource ... }} expression inside that untaken branch is therefore not exercised and its error
//     does not surface here; it is only caught at resolve time when real parameter values make the branch
//     taken.
func validateTemplateExpression(field string, allowedParamNames map[string]string, allowedResourceRefIDs map[string]bool, allowResourceRefs bool) error {
	if field == "" {
		return nil
	}
	tmpl, err := template.New("validate").
		Option("missingkey=error").
		Funcs(template.FuncMap{
			"resource": func(id, jsonPath string) (string, error) {
				if !allowResourceRefs {
					return "", fmt.Errorf("may not reference resources via {{ resource }} here (only .Workspace and .Parameters are allowed)")
				}
				if !allowedResourceRefIDs[id] {
					return "", fmt.Errorf("references undeclared resourceRef %q (add it to resourceRefs)", id)
				}
				if perr := jsonpath.New("validate").Parse(jsonPath); perr != nil {
					return "", fmt.Errorf("invalid JSONPath %q: %w", jsonPath, perr)
				}
				return "", nil
			},
		}).
		Parse(field)
	if err != nil {
		return fmt.Errorf("invalid template %q: %w", field, err)
	}
	if err := tmpl.Execute(io.Discard, templateValidationData{Parameters: allowedParamNames}); err != nil {
		return fmt.Errorf("invalid template %q: %w", field, err)
	}
	return nil
}
