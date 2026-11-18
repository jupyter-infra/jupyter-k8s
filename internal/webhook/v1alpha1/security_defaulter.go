/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// applySecurityDefaults applies security-related defaults from template to workspace. A template whose
// security contexts hold identity templates is left to the IdentityDefaulter, which resolves them.
func applySecurityDefaults(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) {
	// Skipping both contexts loses nothing: the workspace defaulter runs the IdentityDefaulter later in
	// the same request, and it applies the pod and container defaults in whole, resolved for the owner
	// and namespace, exactly as below for a workspace that sets none. If the IdentityDefaulter is not
	// configured, or cannot resolve them, the admission request fails (pinned by the webhook spec "fails
	// closed when the identity defaulter or resolver is not configured"), so a workspace never goes
	// through without its defaults.
	if template.Spec.HasIdentityTemplates() {
		return
	}

	// Apply pod security context defaults. Every identity field holds an integer here, so StaticID cannot
	// fail; the error check only guards against a template this function was not meant for.
	if workspace.Spec.PodSecurityContext == nil && template.Spec.DefaultPodSecurityContext != nil {
		if sc, err := template.Spec.DefaultPodSecurityContext.Resolve(workspacev1alpha1.StaticID); err == nil {
			workspace.Spec.PodSecurityContext = sc
		}
	}

	// Apply container security context defaults
	if workspace.Spec.ContainerSecurityContext == nil && template.Spec.DefaultContainerSecurityContext != nil {
		if sc, err := template.Spec.DefaultContainerSecurityContext.Resolve(workspacev1alpha1.StaticID); err == nil {
			workspace.Spec.ContainerSecurityContext = sc
		}
	}
}
