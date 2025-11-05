package controller

import (
	corev1 "k8s.io/api/core/v1"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

// addStandardWorkspaceEnvVars adds standard environment variables that all workspaces should have
func (db *DeploymentBuilder) addStandardWorkspaceEnvVars(
	container *corev1.Container,
	workspace *workspacev1alpha1.Workspace,
) {
	standardEnvVars := []corev1.EnvVar{
		{
			Name:  EnvWorkspaceNamespace,
			Value: workspace.Namespace,
		},
		{
			Name:  EnvWorkspaceName,
			Value: workspace.Name,
		},
	}

	// Add ACCESS_TYPE
	if accessType := db.getAccessType(workspace); accessType != "" {
		standardEnvVars = append(standardEnvVars, corev1.EnvVar{
			Name:  EnvAccessType,
			Value: accessType,
		})
	}

	// Add APP_TYPE
	if appType := db.getAppType(workspace); appType != "" {
		standardEnvVars = append(standardEnvVars, corev1.EnvVar{
			Name:  EnvAppType,
			Value: appType,
		})
	}

	container.Env = append(standardEnvVars, container.Env...)
}

// getAccessType determines the access type from workspace spec
func (db *DeploymentBuilder) getAccessType(workspace *workspacev1alpha1.Workspace) string {
	return workspace.Spec.AccessType
}

// getAppType determines the app type from workspace spec
func (db *DeploymentBuilder) getAppType(workspace *workspacev1alpha1.Workspace) string {
	return workspace.Spec.AppType
}
