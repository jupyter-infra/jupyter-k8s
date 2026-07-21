/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"errors"
	"fmt"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// errIntegrationReplayFailed marks a build failure that originates specifically in the integration
// frozen-replay path: the referenced template can't be loaded (deleted/unreadable), a frozen value can
// no longer be rendered (template edited to drop a value), or an integration container/volume name
// collides. The resource manager swallows ONLY this error -- preserving the running Deployment and its
// already-injected sidecar (health still surfaces via the status probe) -- and keeps every non-replay
// build error (base pod spec, image resolver, access strategy) fatal. Wrap replay failures with it;
// check with errors.Is at the swallow site.
var errIntegrationReplayFailed = errors.New("could not apply integration to the workspace")

// This file is the build side of integrations: it renders each attached integration onto the pod template
// from the frozen values (status.resolvedIntegrations), using the pre-fetched template (supplied by the
// resource manager) for pod SHAPE and never reading the referenced resource (that is the capture side's
// job). Mirrors deployment_builder_access.go. Functions are ordered entry points first, then the helpers
// they call.

// applyIntegrationsToDeployment merges every attached integration's frozen values into the pod template,
// using the caller-supplied templates keyed by integration name -- same values yield the same template, so
// an unchanged integration never rolls the pod. Fail-closed: no frozen record yet -> skip (base-only); no
// template supplied for the ref, or a render error -> abort the build with a sentinel-tagged error so the
// resource manager leaves the running pod untouched.
func (db *DeploymentBuilder) applyIntegrationsToDeployment(
	ctx context.Context,
	deployment *appsv1.Deployment,
	workspace *workspacev1alpha1.Workspace,
	integrationTemplates map[string]*workspacev1alpha1.WorkspaceIntegrationTemplate,
) error {
	logger := logf.FromContext(ctx)

	for i := range workspace.Spec.IntegrationTemplateRefs {
		ref := &workspace.Spec.IntegrationTemplateRefs[i]
		frozen := findResolvedIntegration(&workspace.Status, ref.Name)
		if frozen == nil {
			logger.V(1).Info("integration has no frozen values yet; building base pod without its overlay",
				"integration", ref.Name)
			continue
		}
		template := integrationTemplates[ref.Name]
		if template == nil {
			// The resource manager could not supply the template (deleted or transiently unreadable). An
			// existing workspace must PRESERVE its running sidecar, not wedge or roll to a base-only pod:
			// tag with the sentinel so the resource manager swallows it and keeps the running Deployment.
			return fmt.Errorf("integration %q overlay failed: could not load the template: %w",
				ref.Name, errIntegrationReplayFailed)
		}
		mods, shareProcessNamespace, err := db.buildIntegrationPodModifications(workspace, ref, frozen, template)
		if err != nil {
			return fmt.Errorf("integration %q overlay failed: %w", ref.Name, err)
		}
		if err := applyPodModifications(&deployment.Spec.Template.Spec, mods); err != nil {
			return fmt.Errorf("integration %q overlay failed: %w", ref.Name, err)
		}
		// OR-reduce: any integration requesting a shared PID namespace enables it; none can disable it.
		if shareProcessNamespace != nil && *shareProcessNamespace {
			deployment.Spec.Template.Spec.ShareProcessNamespace = shareProcessNamespace
		}
	}
	return nil
}

// buildIntegrationPodModifications resolves {{ resource }} values from the frozen set (a missing frozen
// value is a hard error) against the pre-fetched template's pod shape. shareProcessNamespace is carried through.
func (db *DeploymentBuilder) buildIntegrationPodModifications(
	workspace *workspacev1alpha1.Workspace,
	ref *workspacev1alpha1.IntegrationTemplateRef,
	frozen *workspacev1alpha1.ResolvedIntegration,
	template *workspacev1alpha1.WorkspaceIntegrationTemplate,
) (*workspacev1alpha1.PodModifications, *bool, error) {
	shareProcessNamespace := template.Spec.ShareProcessNamespace

	if template.Spec.DeploymentModifications == nil || template.Spec.DeploymentModifications.PodModifications == nil {
		return &workspacev1alpha1.PodModifications{}, shareProcessNamespace, nil
	}

	data := buildIntegrationTemplateData(workspace, ref)
	resolver := NewIntegrationTemplateResolver(NewFrozenResourceValueProvider(frozen.Values))
	mods, err := resolver.ResolvePodModifications(template.Spec.DeploymentModifications.PodModifications, data)
	if err != nil {
		// A frozen value can no longer be rendered (template edited to reference a value not in the
		// frozen set). Tag with the sentinel so the resource manager preserves the running pod.
		return nil, nil, fmt.Errorf("could not apply the pod changes for integration %q: %w",
			ref.Name, errors.Join(errIntegrationReplayFailed, err))
	}
	if mods == nil {
		mods = &workspacev1alpha1.PodModifications{}
	}
	return mods, shareProcessNamespace, nil
}

// applyPodModifications appends the integration's containers/volumes and merges its primary-container env
// (integration-owned key wins). Mirrors the AccessStrategy merge. It rejects a modification whose
// container or volume name collides with an existing one (including the primary container and the
// workspace-storage volume) -- an appended duplicate produces an invalid Deployment the API server
// rejects, so catch it here (tagged errIntegrationReplayFailed) rather than let the update fail opaquely.
func applyPodModifications(ps *corev1.PodSpec, pm *workspacev1alpha1.PodModifications) error {
	if pm == nil {
		return nil
	}

	existingContainers := make(map[string]struct{}, len(ps.Containers)+len(ps.InitContainers))
	for i := range ps.Containers {
		existingContainers[ps.Containers[i].Name] = struct{}{}
	}
	for i := range ps.InitContainers {
		existingContainers[ps.InitContainers[i].Name] = struct{}{}
	}
	for _, c := range append(append([]corev1.Container{}, pm.AdditionalContainers...), pm.InitContainers...) {
		if _, clash := existingContainers[c.Name]; clash {
			return fmt.Errorf("%w: integration container name %q collides with an existing container",
				errIntegrationReplayFailed, c.Name)
		}
		existingContainers[c.Name] = struct{}{}
	}

	existingVolumes := make(map[string]struct{}, len(ps.Volumes))
	for i := range ps.Volumes {
		existingVolumes[ps.Volumes[i].Name] = struct{}{}
	}
	for _, v := range pm.Volumes {
		if _, clash := existingVolumes[v.Name]; clash {
			return fmt.Errorf("%w: integration volume name %q collides with an existing volume",
				errIntegrationReplayFailed, v.Name)
		}
		existingVolumes[v.Name] = struct{}{}
	}

	ps.Containers = append(ps.Containers, pm.AdditionalContainers...)
	ps.InitContainers = append(ps.InitContainers, pm.InitContainers...)
	ps.Volumes = append(ps.Volumes, pm.Volumes...)

	if pm.PrimaryContainerModifications == nil {
		return nil
	}
	primary := findPrimaryContainer(ps)
	if primary == nil {
		// Should never happen -- buildPodSpec always creates the primary container. Log rather than
		// silently drop the primary-container env/volumeMounts if the pod template is ever malformed.
		logf.Log.Error(nil, "primary container not found in pod spec; dropping integration primary-container modifications",
			"expectedContainerName", PrimaryContainerName)
		return nil
	}
	for _, env := range pm.PrimaryContainerModifications.MergeEnv {
		// MergeEnv[].ValueTemplate has already been resolved to a literal by the resolver.
		setEnv(primary, env.Name, env.ValueTemplate)
	}
	primary.VolumeMounts = append(primary.VolumeMounts, pm.PrimaryContainerModifications.VolumeMounts...)
	return nil
}

// findPrimaryContainer returns the workspace's primary container by its well-known name.
func findPrimaryContainer(ps *corev1.PodSpec) *corev1.Container {
	for i := range ps.Containers {
		if ps.Containers[i].Name == PrimaryContainerName {
			return &ps.Containers[i]
		}
	}
	return nil
}

// setEnv sets (overwriting an existing same-named entry) a literal-valued env var on a container.
// It assigns a FRESH EnvVar so that if the prior entry carried a ValueFrom, that source is cleared --
// leaving both Value and ValueFrom set is rejected by the API server.
func setEnv(c *corev1.Container, name, value string) {
	for i := range c.Env {
		if c.Env[i].Name == name {
			c.Env[i] = corev1.EnvVar{Name: name, Value: value}
			return
		}
	}
	c.Env = append(c.Env, corev1.EnvVar{Name: name, Value: value})
}
