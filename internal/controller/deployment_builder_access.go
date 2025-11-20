/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
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

// applyDeploymentSpecModifications applies deployment modifications from access strategy
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

	mods := accessStrategy.Spec.DeploymentModifications

	// Add volumes
	if mods.PodModifications != nil && len(mods.PodModifications.Volumes) > 0 {
		logf.Log.V(1).Info("Adding volumes from deployment modifications",
			"accessStrategy", accessStrategy.Name,
			"volumeCount", len(mods.PodModifications.Volumes))

		deployment.Spec.Template.Spec.Volumes = append(
			deployment.Spec.Template.Spec.Volumes,
			mods.PodModifications.Volumes...,
		)

		for _, volume := range mods.PodModifications.Volumes {
			logf.Log.V(1).Info("Added volume",
				"accessStrategy", accessStrategy.Name,
				"volumeName", volume.Name)
		}
	}

	// Add volume mounts to primary container
	if mods.PodModifications != nil &&
		mods.PodModifications.PrimaryContainerModifications != nil &&
		len(mods.PodModifications.PrimaryContainerModifications.VolumeMounts) > 0 {

		if len(deployment.Spec.Template.Spec.Containers) == 0 {
			return fmt.Errorf("no containers found in deployment to add volume mounts")
		}

		logf.Log.V(1).Info("Adding volume mounts to primary container",
			"accessStrategy", accessStrategy.Name,
			"mountCount", len(mods.PodModifications.PrimaryContainerModifications.VolumeMounts))

		primaryContainer := &deployment.Spec.Template.Spec.Containers[0]
		primaryContainer.VolumeMounts = append(
			primaryContainer.VolumeMounts,
			mods.PodModifications.PrimaryContainerModifications.VolumeMounts...,
		)

		for _, mount := range mods.PodModifications.PrimaryContainerModifications.VolumeMounts {
			logf.Log.V(1).Info("Added volume mount to primary container",
				"accessStrategy", accessStrategy.Name,
				"volumeName", mount.Name,
				"mountPath", mount.MountPath)
		}
	}

	// Add init containers
	if mods.PodModifications != nil && len(mods.PodModifications.InitContainers) > 0 {
		logf.Log.V(1).Info("Adding init containers",
			"accessStrategy", accessStrategy.Name,
			"containerCount", len(mods.PodModifications.InitContainers))

		deployment.Spec.Template.Spec.InitContainers = append(
			deployment.Spec.Template.Spec.InitContainers,
			mods.PodModifications.InitContainers...,
		)

		for _, container := range mods.PodModifications.InitContainers {
			logf.Log.V(1).Info("Added init container",
				"accessStrategy", accessStrategy.Name,
				"containerName", container.Name,
				"image", container.Image)
		}
	}

	// Add additional containers
	if mods.PodModifications != nil && len(mods.PodModifications.AdditionalContainers) > 0 {
		logf.Log.V(1).Info("Adding additional containers",
			"accessStrategy", accessStrategy.Name,
			"containerCount", len(mods.PodModifications.AdditionalContainers))

		deployment.Spec.Template.Spec.Containers = append(
			deployment.Spec.Template.Spec.Containers,
			mods.PodModifications.AdditionalContainers...,
		)

		for _, container := range mods.PodModifications.AdditionalContainers {
			logf.Log.V(1).Info("Added additional container",
				"accessStrategy", accessStrategy.Name,
				"containerName", container.Name,
				"image", container.Image)
		}
	}

	logf.Log.V(1).Info("Successfully applied deployment modifications",
		"accessStrategy", accessStrategy.Name)

	return nil
}
