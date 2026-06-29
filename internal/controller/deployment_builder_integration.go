/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"fmt"
	"sort"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// applyIntegrationsToDeployment lists the workspace's WorkspaceIntegration children (by the
// workspace.jupyter.org/workspace-name label) and merges each child's frozen, resolved
// deploymentModifications into the deployment pod template.
//
// All values come from the children's own spec fields (deploymentModifications/shareProcessNamespace),
// which the WorkspaceIntegration mutating webhook resolved against the live referenced resources at the
// child's OWN admission. The controller therefore does ZERO resolution and ZERO reads of
// WorkspaceIntegrationTemplate or the referenced resources at reconcile time; it only lists the frozen
// children and stamps their values onto the pod.
//
// Both AccessStrategy and Integration are producers of the same PodModifications shape, so the
// structural merge flows through the shared applyPodModificationsToDeployment helper. Children are
// explicitly sorted by name before merging so the result is deterministic: client.List order is
// not contractually guaranteed (it is etcd key order, and the fake client returns insertion order),
// and env/volume/container ordering affects the pod hash and therefore rollout. Today
// integrationRefs is capped at MaxItems=1 so at most one child applies, but the sort keeps the
// merge order-stable if that cap is ever raised.
func (db *DeploymentBuilder) applyIntegrationsToDeployment(
	ctx context.Context,
	deployment *appsv1.Deployment,
	workspace *workspacev1alpha1.Workspace,
) error {
	if deployment == nil {
		return fmt.Errorf("cannot apply integrations to nil deployment")
	}

	// No integrations requested -> nothing to list or apply.
	if len(workspace.Spec.IntegrationRefs) == 0 {
		return nil
	}

	list := &workspacev1alpha1.WorkspaceIntegrationList{}
	if err := db.client.List(ctx, list,
		client.InNamespace(workspace.Namespace),
		client.MatchingLabels{workspaceutil.LabelWorkspaceName: workspace.Name},
	); err != nil {
		return fmt.Errorf("failed to list WorkspaceIntegrations for workspace %q: %w", workspace.Name, err)
	}

	// Sort by name for a deterministic merge order -- client.List order is not guaranteed.
	sort.Slice(list.Items, func(i, j int) bool {
		return list.Items[i].Name < list.Items[j].Name
	})

	logger := logf.FromContext(ctx)

	for i := range list.Items {
		wi := &list.Items[i]
		if !workspaceIntegrationResolved(wi) {
			// The child exists but has not been resolved yet (webhook pending / failed). Skip it: the
			// deployment is built without this integration and a later reconcile (triggered by the
			// Owns watch when the child is resolved) re-applies it. Never build half-resolved content.
			logger.V(1).Info("Skipping unresolved WorkspaceIntegration child when building deployment",
				"workspaceintegration", wi.Name, "workspace", workspace.Name)
			continue
		}

		// ShareProcessNamespace is OR-reduced across all integrations: any one that requests it
		// enables a shared PID namespace for the whole pod. We only ever set it true here -- a later
		// integration must not clear an earlier one's request. The base deployment leaves it nil and
		// NeedsUpdate rebuilds the desired deployment from scratch each reconcile, so removing the
		// flag from every integration resets the pod to nil (reversible).
		if wi.Spec.ShareProcessNamespace != nil && *wi.Spec.ShareProcessNamespace {
			enabled := true
			deployment.Spec.Template.Spec.ShareProcessNamespace = &enabled
		}

		if wi.Spec.DeploymentModifications != nil {
			pm := wi.Spec.DeploymentModifications.PodModifications
			// Structural merge (volumes, primary volume mounts, init + additional containers) via the
			// shared helper -- the same one AccessStrategy uses.
			applyPodModificationsToDeployment(deployment, pm)
			// Primary-container env: the integration's MergeEnv is already resolved to literals at
			// admission (ValueTemplate holds the literal value), so merge it directly. AccessStrategy
			// resolves its env live elsewhere; that is why env is per-source, not in the shared helper.
			if pm != nil && pm.PrimaryContainerModifications != nil &&
				len(pm.PrimaryContainerModifications.MergeEnv) > 0 &&
				len(deployment.Spec.Template.Spec.Containers) > 0 {
				mergeLiteralEnv(
					&deployment.Spec.Template.Spec.Containers[0],
					pm.PrimaryContainerModifications.MergeEnv,
				)
			}
		}
	}

	return nil
}

// mergeLiteralEnv merges already-resolved env entries (whose ValueTemplate holds a literal value)
// into a container's env list. Existing env vars with matching names are updated; new ones appended.
func mergeLiteralEnv(container *corev1.Container, env []workspacev1alpha1.AccessEnvTemplate) {
	for _, e := range env {
		found := false
		for i, existing := range container.Env {
			if existing.Name == e.Name {
				container.Env[i].Value = e.ValueTemplate
				found = true
				break
			}
		}
		if !found {
			container.Env = append(container.Env, corev1.EnvVar{Name: e.Name, Value: e.ValueTemplate})
		}
	}
}

// workspaceIntegrationResolved reports whether the WorkspaceIntegration child has been resolved by
// the mutating webhook. A child that carries any resolved output (deploymentModifications,
// statusProbe, or shareProcessNamespace) has been through the webhook; an unresolved shell carries
// none. Given the webhook's failurePolicy=Fail, a persisted child is always resolved -- this guard
// only matters for the brief in-memory window before admission.
func workspaceIntegrationResolved(wi *workspacev1alpha1.WorkspaceIntegration) bool {
	return wi.Spec.DeploymentModifications != nil ||
		wi.Spec.StatusProbe != nil ||
		wi.Spec.ShareProcessNamespace != nil
}
