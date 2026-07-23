/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"fmt"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Helpers shared by the capture (resource_manager_integration.go) and build (deployment_builder_integration.go)
// sides, so neither owns the other's fundamentals.
//
// Fail-closed invariant -- a broken integration never strips a running sidecar nor blocks the reconcile.
// Three sites, in reconcile order:
//  1. reconcileIntegrations (capture): a capture error keeps the existing record, logs, continues.
//  2. applyIntegrationsToDeployment (build): a render error aborts the build, leaving the running Deployment.
//  3. ensureDeploymentUpToDate (NeedsUpdate): a desired-build error is non-fatal for integration workspaces.

// findResolvedIntegration returns the frozen record for the named integration, or nil if none has been
// frozen yet.
func findResolvedIntegration(status *workspacev1alpha1.WorkspaceStatus, name string) *workspacev1alpha1.ResolvedIntegration {
	if status == nil {
		return nil
	}
	for i := range status.ResolvedIntegrations {
		if status.ResolvedIntegrations[i].Name == name {
			return &status.ResolvedIntegrations[i]
		}
	}
	return nil
}

// buildIntegrationTemplateData builds the render context (workspace identity + user parameters).
func buildIntegrationTemplateData(
	workspace *workspacev1alpha1.Workspace,
	ref *workspacev1alpha1.IntegrationTemplateRef,
) IntegrationTemplateData {
	return IntegrationTemplateData{
		Workspace:  IntegrationWorkspaceData{Name: workspace.Name, Namespace: workspace.Namespace},
		Parameters: ref.ParametersMap(),
	}
}

// resolveStatusProbeCommand renders the statusProbe's exec command against the given resolver, returning
// a copy of the probe with the command resolved (the input probe is not mutated). The probe command can
// carry {{ resource }} / {{ .Parameters }} expressions exactly like the sidecar fields, so it must go
// through the resolver too -- both the capture side (live provider, to harvest the keys) and the probe
// side (frozen provider, to replay them) call this so the SAME expression set is rendered on each. A nil
// probe or one without an Exec command is returned unchanged.
func resolveStatusProbeCommand(
	resolver *IntegrationTemplateResolver,
	probe *workspacev1alpha1.IntegrationStatusProbe,
	data IntegrationTemplateData,
) (*workspacev1alpha1.IntegrationStatusProbe, error) {
	if probe == nil || probe.Exec == nil || len(probe.Exec.Command) == 0 {
		return probe, nil
	}
	resolved := probe.DeepCopy()
	for i, arg := range resolved.Exec.Command {
		rendered, err := resolver.ResolveTemplateExpression(arg, data)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve statusProbe command: %w", err)
		}
		resolved.Exec.Command[i] = rendered
	}
	return resolved, nil
}

// getIntegrationTemplate loads the referenced template from the ref's namespace, defaulting to the
// workspace's own namespace when the ref omits one. Admission validates that the ref targets the
// workspace's own namespace or the shared namespace before it is stored. Missing is a fail-closed error.
func getIntegrationTemplate(
	ctx context.Context,
	c client.Client,
	workspace *workspacev1alpha1.Workspace,
	ref *workspacev1alpha1.IntegrationTemplateRef,
) (*workspacev1alpha1.WorkspaceIntegrationTemplate, error) {
	ns := ref.Namespace
	if ns == "" {
		ns = workspace.Namespace
	}
	template := &workspacev1alpha1.WorkspaceIntegrationTemplate{}
	if err := c.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ns}, template); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("integration template %q not found in namespace %q", ref.Name, ns)
		}
		return nil, fmt.Errorf("failed to get integration template %q: %w", ref.Name, err)
	}
	return template, nil
}
