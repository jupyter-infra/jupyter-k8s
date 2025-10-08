package controller

import (
	"bytes"
	"fmt"
	"text/template"

	workspacesv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// partialAccessResourceData provides values for template substitutions
type partialAccessResourceData struct {
	Workspace      *workspacesv1alpha1.Workspace
	AccessStrategy *workspacesv1alpha1.WorkspaceAccessStrategy
}

// resolveAccessStrategyEnv interpolates the env defined in the AccessStrategy
// for a particular Workspace.
func (b *DeploymentBuilder) resolveAccessStrategyEnv(
	accessStrategy *workspacesv1alpha1.WorkspaceAccessStrategy,
	workspace *workspacesv1alpha1.Workspace,
) ([]map[string]string, error) {
	data := &partialAccessResourceData{
		Workspace:      workspace,
		AccessStrategy: accessStrategy,
	}

	var envVars = []map[string]string{}

	for _, envTemplate := range accessStrategy.Spec.MergeEnv {
		tmpl, err := template.New("env").Parse(envTemplate.ValueTemplate)
		if err != nil {
			return nil, fmt.Errorf("failed to parse env template for %s: %w", envTemplate.Name, err)
		}

		var value bytes.Buffer
		if err := tmpl.Execute(&value, data); err != nil {
			return nil, fmt.Errorf("failed to execute env template for %s: %w", envTemplate.Name, err)
		}

		envVars = append(envVars, map[string]string{
			"name":  envTemplate.Name,
			"value": value.String(),
		})
	}

	return envVars, nil
}

// AddAccessStrategyEnvToContainer adds environment variables from an access strategy to a container.
// The env vars defined in the AccessStrategy take precedence over those requested
// by the Workspace create / update API.
func (db *DeploymentBuilder) addAccessStrategyEnvToContainer(
	container *corev1.Container,
	workspace *workspacesv1alpha1.Workspace,
	accessStrategy *workspacesv1alpha1.WorkspaceAccessStrategy,
) error {
	if container == nil {
		return fmt.Errorf("container is nil, cannot apply env vars of AccessStrategy: %s", accessStrategy.Name)
	}

	resolvedAccessStrategyEnv, err := db.resolveAccessStrategyEnv(accessStrategy, workspace)
	if err != nil {
		return err
	}

	// Add each environment variable from the access strategy
	for _, resolvedEnvVar := range resolvedAccessStrategyEnv {
		name, ok := resolvedEnvVar["name"]
		if !ok {
			return fmt.Errorf("environment variable missing name: %s", name)
		}

		value, ok := resolvedEnvVar["value"]
		if !ok {
			return fmt.Errorf("environment variable %s missing value", name)
		}

		// Check if the env var already exists
		found := false
		for i, existing := range container.Env {
			if existing.Name == name {
				// Update the existing env var
				container.Env[i].Value = value
				found = true
				break
			}
		}

		// Add the env var if it doesn't exist
		if !found {
			container.Env = append(container.Env, corev1.EnvVar{
				Name:  name,
				Value: value,
			})
		}
	}

	return nil
}

// ApplyAccessStrategyToDeployment applies access strategy settings to a container
// Currently only adds environment variables, but could be extended in the future
func (db *DeploymentBuilder) ApplyAccessStrategyToDeployment(
	deployment *appsv1.Deployment,
	workspace *workspacesv1alpha1.Workspace,
	accessStrategy *workspacesv1alpha1.WorkspaceAccessStrategy,
) error {
	if deployment == nil {
		return fmt.Errorf("cannot apply AccessStrategy '%s' to nil deployment", accessStrategy.Name)
	}
	if accessStrategy == nil {
		return nil // Nothing to do
	}
	primaryContainer := &deployment.Spec.Template.Spec.Containers[0]

	// Apply environment variables to primary container
	if err := db.addAccessStrategyEnvToContainer(primaryContainer, workspace, accessStrategy); err != nil {
		return fmt.Errorf("failed to add environment variables to container: %w", err)
	}

	// Future extensions could be added here:
	// - sidecar containers

	return nil
}
