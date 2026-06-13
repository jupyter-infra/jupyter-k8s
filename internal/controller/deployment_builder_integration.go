/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"fmt"
	"strings"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// ApplyIntegrationStrategyToDeployment resolves the workspace's integration strategy and
// merges its resolved pod modifications into the deployment. This is the public entry point
// matching the ApplyAccessStrategyToDeployment pattern.
//
// Steps:
//  1. Build template data from workspace metadata + parameters
//  2. If resourceLookup is defined: resolve its name/namespace and fetch the resource
//  3. Resolve all template expressions in podModifications
//  4. Merge resolved modifications into the deployment
func (db *DeploymentBuilder) ApplyIntegrationStrategyToDeployment(
	ctx context.Context,
	deployment *appsv1.Deployment,
	workspace *workspacev1alpha1.Workspace,
	strategy *workspacev1alpha1.WorkspaceIntegrationStrategy,
) error {
	if deployment == nil {
		return fmt.Errorf("cannot apply integration strategy %q to nil deployment", strategy.Name)
	}
	if strategy == nil {
		return nil // Nothing to do
	}

	logger := logf.FromContext(ctx)
	ref := workspace.Spec.IntegrationStrategy

	logger.V(1).Info("Applying integration strategy",
		"strategy", strategy.Name,
		"namespace", strategy.Namespace)

	// Nothing to apply if there are no deployment modifications
	if strategy.Spec.DeploymentModifications == nil ||
		strategy.Spec.DeploymentModifications.PodModifications == nil {
		return nil
	}

	// Build template data context. The CRD stores parameters as an extensible
	// name/value list; flatten it to a map for {{ .Parameters.X }} template access.
	tmplData := IntegrationTemplateData{
		Workspace: IntegrationWorkspaceData{
			Name:      workspace.Name,
			Namespace: workspace.Namespace,
		},
		Parameters: ref.ParametersMap(),
	}

	resolver := NewIntegrationResolver()

	// Resolve and fetch the looked-up resource, if defined
	var resource *unstructured.Unstructured
	var err error
	if strategy.Spec.ResourceLookup != nil {
		resource, err = db.resolveAndFetchResource(ctx, strategy, tmplData, resolver)
		if err != nil {
			return fmt.Errorf("integration strategy %q: %w", strategy.Name, err)
		}
	}

	// Resolve all template expressions in the pod modifications
	resolvedMods, err := resolver.ResolvePodModifications(
		strategy.Spec.DeploymentModifications.PodModifications,
		tmplData,
		resource,
	)
	if err != nil {
		return fmt.Errorf("integration strategy %q: failed to resolve pod modifications: %w", strategy.Name, err)
	}

	// Merge resolved modifications into the deployment
	if err := db.mergeIntegrationModifications(deployment, resolvedMods); err != nil {
		return fmt.Errorf("integration strategy %q: failed to apply modifications: %w", strategy.Name, err)
	}

	return nil
}

// resolveAndFetchResource resolves the resourceLookup name/namespace templates and
// fetches the target resource via the unstructured client.
func (db *DeploymentBuilder) resolveAndFetchResource(
	ctx context.Context,
	strategy *workspacev1alpha1.WorkspaceIntegrationStrategy,
	tmplData IntegrationTemplateData,
	resolver *IntegrationResolver,
) (*unstructured.Unstructured, error) {
	lookup := strategy.Spec.ResourceLookup

	// Default the namespace template to the workspace namespace when unset
	effectiveLookup := *lookup
	if effectiveLookup.Namespace == "" {
		effectiveLookup.Namespace = tmplData.Workspace.Namespace
	}

	resolvedName, resolvedNamespace, err := resolver.ResolveResourceLookup(&effectiveLookup, tmplData)
	if err != nil {
		return nil, err
	}

	gvk := parseGroupVersionKind(lookup.APIVersion, lookup.Kind)
	resource := &unstructured.Unstructured{}
	resource.SetGroupVersionKind(gvk)

	err = db.client.Get(ctx, client.ObjectKey{Name: resolvedName, Namespace: resolvedNamespace}, resource)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("looked-up resource %s %q not found in namespace %q",
				lookup.Kind, resolvedName, resolvedNamespace)
		}
		if apierrors.IsForbidden(err) {
			return nil, fmt.Errorf("insufficient permissions to get %s %q in namespace %q: %w",
				lookup.Kind, resolvedName, resolvedNamespace, err)
		}
		return nil, fmt.Errorf("failed to get %s %q in namespace %q: %w",
			lookup.Kind, resolvedName, resolvedNamespace, err)
	}

	return resource, nil
}

// mergeIntegrationModifications merges resolved integration strategy pod
// modifications into a deployment. The modifications must already have all template
// expressions resolved (see IntegrationResolver.ResolvePodModifications).
//
// The merge logic mirrors applyDeploymentSpecModifications used by AccessStrategy:
// volumes, init containers, and additional containers are appended; volume mounts
// and env vars are merged into the primary (first) container.
func (db *DeploymentBuilder) mergeIntegrationModifications(
	deployment *appsv1.Deployment,
	resolvedMods *workspacev1alpha1.PodModifications,
) error {
	if deployment == nil {
		return fmt.Errorf("cannot apply integration strategy to nil deployment")
	}
	if resolvedMods == nil {
		return nil // Nothing to do
	}

	// Append volumes
	if len(resolvedMods.Volumes) > 0 {
		deployment.Spec.Template.Spec.Volumes = append(
			deployment.Spec.Template.Spec.Volumes,
			resolvedMods.Volumes...,
		)
	}

	// Append init containers
	if len(resolvedMods.InitContainers) > 0 {
		deployment.Spec.Template.Spec.InitContainers = append(
			deployment.Spec.Template.Spec.InitContainers,
			resolvedMods.InitContainers...,
		)
	}

	// Append additional containers (sidecars)
	if len(resolvedMods.AdditionalContainers) > 0 {
		deployment.Spec.Template.Spec.Containers = append(
			deployment.Spec.Template.Spec.Containers,
			resolvedMods.AdditionalContainers...,
		)
	}

	// Apply primary container modifications
	if resolvedMods.PrimaryContainerModifications != nil {
		if len(deployment.Spec.Template.Spec.Containers) == 0 {
			return fmt.Errorf("no primary container found in deployment to apply integration strategy modifications")
		}
		primaryContainer := &deployment.Spec.Template.Spec.Containers[0]

		if len(resolvedMods.PrimaryContainerModifications.VolumeMounts) > 0 {
			primaryContainer.VolumeMounts = append(
				primaryContainer.VolumeMounts,
				resolvedMods.PrimaryContainerModifications.VolumeMounts...,
			)
		}

		mergeIntegrationEnv(primaryContainer, resolvedMods.PrimaryContainerModifications.MergeEnv)
	}

	return nil
}

// mergeIntegrationEnv merges resolved env templates into a container's env list.
// The valueTemplate field holds the already-resolved value at this point.
// Existing env vars with matching names are updated; new ones are appended.
func mergeIntegrationEnv(container *corev1.Container, mergeEnv []workspacev1alpha1.AccessEnvTemplate) {
	for _, envTemplate := range mergeEnv {
		found := false
		for i, existing := range container.Env {
			if existing.Name == envTemplate.Name {
				container.Env[i].Value = envTemplate.ValueTemplate
				found = true
				break
			}
		}
		if !found {
			container.Env = append(container.Env, corev1.EnvVar{
				Name:  envTemplate.Name,
				Value: envTemplate.ValueTemplate,
			})
		}
	}
}

// parseGroupVersionKind parses an apiVersion string (e.g. "ray.io/v1" or "v1") plus a
// kind into a schema.GroupVersionKind.
func parseGroupVersionKind(apiVersion, kind string) schema.GroupVersionKind {
	var group, version string
	parts := strings.SplitN(apiVersion, "/", 2)
	if len(parts) == 2 {
		group = parts[0]
		version = parts[1]
	} else {
		version = apiVersion
	}
	return schema.GroupVersionKind{Group: group, Version: version, Kind: kind}
}
