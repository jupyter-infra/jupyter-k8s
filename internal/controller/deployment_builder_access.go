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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// partialAccessResourceData provides values for template substitutions
type partialAccessResourceData struct {
	Workspace      *workspacev1alpha1.Workspace
	AccessStrategy *workspacev1alpha1.WorkspaceAccessStrategy
}

func (b *DeploymentBuilder) getPrimaryContainerMergeEnv(
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
) *[]workspacev1alpha1.AccessEnvTemplate {
	if accessStrategy == nil {
		return nil
	}

	if accessStrategy.Spec.DeploymentModifications != nil &&
		accessStrategy.Spec.DeploymentModifications.PodModifications != nil &&
		accessStrategy.Spec.DeploymentModifications.PodModifications.PrimaryContainerModifications != nil &&
		len(accessStrategy.Spec.DeploymentModifications.PodModifications.PrimaryContainerModifications.MergeEnv) > 0 {
		return &accessStrategy.Spec.DeploymentModifications.PodModifications.PrimaryContainerModifications.MergeEnv
	}

	return nil
}

// resolveAccessStrategyPrimaryContainerEnv interpolates the env defined in the AccessStrategy
// for a particular Workspace.
func (b *DeploymentBuilder) resolveAccessStrategyPrimaryContainerEnv(
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
	workspace *workspacev1alpha1.Workspace,
) ([]map[string]string, error) {
	data := &partialAccessResourceData{
		Workspace:      workspace,
		AccessStrategy: accessStrategy,
	}

	var envVars = []map[string]string{}
	mergeEnv := b.getPrimaryContainerMergeEnv(accessStrategy)
	if mergeEnv != nil {
		for _, envTemplate := range *mergeEnv {
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
	}

	return envVars, nil
}

// AddAccessStrategyEnvToContainer adds environment variables from an access strategy to a container.
// The env vars defined in the AccessStrategy take precedence over those requested
// by the Workspace create / update API.
func (db *DeploymentBuilder) addAccessStrategyEnvToContainer(
	container *corev1.Container,
	workspace *workspacev1alpha1.Workspace,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
) error {
	if container == nil {
		return fmt.Errorf("container is nil, cannot apply env vars of AccessStrategy: %s", accessStrategy.Name)
	}

	resolvedAccessStrategyEnv, err := db.resolveAccessStrategyPrimaryContainerEnv(accessStrategy, workspace)
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
	workspace *workspacev1alpha1.Workspace,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
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

	// Apply deployment spec modifications if defined
	if err := db.applyDeploymentSpecModifications(deployment, accessStrategy); err != nil {
		return fmt.Errorf("failed to apply deployment spec modifications: %w", err)
	}

	return nil
}

// applyDeploymentSpecModifications applies deployment modifications from an access strategy.
// The structural merge (volumes, primary-container volume mounts, init + additional containers) is
// delegated to applyPodModificationsToDeployment, which both AccessStrategy and Integration share.
// AccessStrategy env is applied separately (live-resolved) via addAccessStrategyEnvToContainer.
func (db *DeploymentBuilder) applyDeploymentSpecModifications(
	deployment *appsv1.Deployment,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
) error {
	if deployment == nil {
		return fmt.Errorf("deployment cannot be nil")
	}
	if accessStrategy.Spec.DeploymentModifications == nil {
		return nil // Nothing to do
	}

	pm := accessStrategy.Spec.DeploymentModifications.PodModifications
	if pm != nil &&
		pm.PrimaryContainerModifications != nil &&
		len(pm.PrimaryContainerModifications.VolumeMounts) > 0 &&
		len(deployment.Spec.Template.Spec.Containers) == 0 {
		return fmt.Errorf("no containers found in deployment to add volume mounts")
	}

	applyPodModificationsToDeployment(deployment, pm)

	logf.Log.V(1).Info("Successfully applied deployment modifications",
		"accessStrategy", accessStrategy.Name)

	return nil
}

// applyPodModificationsToDeployment appends the structural pod modifications (volumes,
// primary-container volume mounts, init containers, additional containers) onto the deployment's pod
// template. It is the shared merge used by both AccessStrategy and WorkspaceIntegration -- both are
// producers of the same PodModifications shape. Primary-container ENV is intentionally NOT handled
// here: AccessStrategy resolves env live (template execution per workspace) while Integration env is
// already resolved to literals at admission, so each caller merges env in its own way.
//
// Volume mounts are applied to the primary (first) container only when one exists; callers that
// require a primary container should validate that before calling.
func applyPodModificationsToDeployment(
	deployment *appsv1.Deployment,
	pm *workspacev1alpha1.PodModifications,
) {
	if pm == nil {
		return
	}

	if len(pm.Volumes) > 0 {
		deployment.Spec.Template.Spec.Volumes = append(
			deployment.Spec.Template.Spec.Volumes, pm.Volumes...)
	}

	if pm.PrimaryContainerModifications != nil &&
		len(pm.PrimaryContainerModifications.VolumeMounts) > 0 &&
		len(deployment.Spec.Template.Spec.Containers) > 0 {
		primaryContainer := &deployment.Spec.Template.Spec.Containers[0]
		primaryContainer.VolumeMounts = append(
			primaryContainer.VolumeMounts, pm.PrimaryContainerModifications.VolumeMounts...)
	}

	if len(pm.InitContainers) > 0 {
		deployment.Spec.Template.Spec.InitContainers = append(
			deployment.Spec.Template.Spec.InitContainers, pm.InitContainers...)
	}

	if len(pm.AdditionalContainers) > 0 {
		deployment.Spec.Template.Spec.Containers = append(
			deployment.Spec.Template.Spec.Containers, pm.AdditionalContainers...)
	}
}
