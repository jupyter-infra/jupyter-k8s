/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// applyEnvDefaults merges template's BaseEnv into workspace's Env.
// Workspace env vars take precedence by name (same pattern as baseLabels).
func applyEnvDefaults(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) {
	if len(template.Spec.BaseEnv) == 0 {
		return
	}

	// Build lookup of existing workspace env var names
	existing := make(map[string]struct{}, len(workspace.Spec.Env))
	for _, e := range workspace.Spec.Env {
		existing[e.Name] = struct{}{}
	}

	// Add template env vars if name doesn't already exist
	for _, e := range template.Spec.BaseEnv {
		if _, exists := existing[e.Name]; !exists {
			workspace.Spec.Env = append(workspace.Spec.Env, *e.DeepCopy())
		}
	}
}
