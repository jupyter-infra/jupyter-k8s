/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"fmt"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// WorkspaceIntegrationManager reconciles the set of WorkspaceIntegration child objects for a
// workspace against workspace.spec.integrationRefs.
//
// It is the "operator pattern" half of the Option 2 model: the workspace controller creates one
// WorkspaceIntegration SHELL per integrationRefs entry (templateRef + workspaceRef + ownerRef +
// identification label), but performs NO resolution itself -- it never reads the
// WorkspaceIntegrationTemplate or the referenced resources. The frozen spec output fields
// (deploymentModifications/statusProbe/shareProcessNamespace) are produced by the
// WorkspaceIntegration mutating webhook at the child's own admission. This keeps the workspace
// reconcile loop free of reconcile-time RayCluster reads.
//
// ownerReferences point each child at the workspace so Kubernetes garbage-collects the children
// when the workspace is deleted. Identification at deployment-build time is by the
// LabelWorkspaceName label (option B): the deployment builder lists children with a label selector
// rather than relying on the deterministic name.
type WorkspaceIntegrationManager struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewWorkspaceIntegrationManager creates a new WorkspaceIntegrationManager.
func NewWorkspaceIntegrationManager(c client.Client, scheme *runtime.Scheme) *WorkspaceIntegrationManager {
	return &WorkspaceIntegrationManager{client: c, scheme: scheme}
}

// ListForWorkspace returns the WorkspaceIntegration children owned by the given workspace, found by
// the LabelWorkspaceName label in the workspace's namespace. This is the canonical identification
// path (option B) used both for reconciling the set and for building the deployment.
func (m *WorkspaceIntegrationManager) ListForWorkspace(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
) ([]workspacev1alpha1.WorkspaceIntegration, error) {
	list := &workspacev1alpha1.WorkspaceIntegrationList{}
	if err := m.client.List(ctx, list,
		client.InNamespace(workspace.Namespace),
		client.MatchingLabels{workspaceutil.LabelWorkspaceName: workspace.Name},
	); err != nil {
		return nil, fmt.Errorf("failed to list WorkspaceIntegrations for workspace %q: %w", workspace.Name, err)
	}
	return list.Items, nil
}

// EnsureForWorkspace reconciles the WorkspaceIntegration children for the workspace: it creates a
// shell for each integrationRefs entry that has none, updates a shell whose templateRef/workspaceRef
// drifted from the workspace's intent (re-triggering the webhook bake), and deletes any child no
// longer backed by an integrationRefs entry. It returns the reconciled set of children, assembled
// locally as it goes (no trailing re-list).
//
// Creating/updating a shell deliberately leaves the resolved output fields untouched here -- the
// mutating webhook fills them at admission. The manager only owns the shell's identity and references.
func (m *WorkspaceIntegrationManager) EnsureForWorkspace(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
) ([]workspacev1alpha1.WorkspaceIntegration, error) {
	logger := logf.FromContext(ctx)

	existing, err := m.ListForWorkspace(ctx, workspace)
	if err != nil {
		return nil, err
	}

	// Index existing children by name for quick lookup and stale detection.
	existingByName := make(map[string]*workspacev1alpha1.WorkspaceIntegration, len(existing))
	for i := range existing {
		existingByName[existing[i].Name] = &existing[i]
	}

	// desiredNames tracks which child names the current integrationRefs require, so anything else
	// owned by this workspace is stale and gets deleted.
	desiredNames := make(map[string]struct{}, len(workspace.Spec.IntegrationRefs))

	// Assemble the reconciled set as we go rather than re-listing at the end: the re-list was an
	// extra API call the caller discards, and an informer-backed client may not yet reflect a
	// just-created object. Each desired ref contributes exactly one child to the result.
	result := make([]workspacev1alpha1.WorkspaceIntegration, 0, len(workspace.Spec.IntegrationRefs))

	for i := range workspace.Spec.IntegrationRefs {
		ref := &workspace.Spec.IntegrationRefs[i]
		name := GenerateWorkspaceIntegrationName(workspace.Name, ref.Name)
		desiredNames[name] = struct{}{}

		desiredSpec := m.buildShellSpec(workspace, ref)

		if current, ok := existingByName[name]; ok {
			// Update only if the intent (templateRef/workspaceRef) drifted. We compare only the
			// shell input fields, never the resolved output -- that is webhook-owned and updating it
			// would fight the webhook. An update re-runs the webhook, re-resolving the output.
			if !equality.Semantic.DeepEqual(current.Spec.TemplateRef, desiredSpec.TemplateRef) ||
				!equality.Semantic.DeepEqual(current.Spec.WorkspaceRef, desiredSpec.WorkspaceRef) {
				current.Spec.TemplateRef = desiredSpec.TemplateRef
				current.Spec.WorkspaceRef = desiredSpec.WorkspaceRef
				if err := m.client.Update(ctx, current); err != nil {
					return nil, fmt.Errorf("failed to update WorkspaceIntegration %q: %w", name, err)
				}
				logger.Info("Updated WorkspaceIntegration shell", "workspaceintegration", name)
			}
			result = append(result, *current)
			continue
		}

		// Create a new shell. ownerRef -> workspace (controller=true) for GC and re-enqueue.
		wi := &workspacev1alpha1.WorkspaceIntegration{}
		wi.Name = name
		wi.Namespace = workspace.Namespace
		wi.Labels = map[string]string{workspaceutil.LabelWorkspaceName: workspace.Name}
		wi.Spec = desiredSpec
		if err := controllerutil.SetControllerReference(workspace, wi, m.scheme); err != nil {
			return nil, fmt.Errorf("failed to set owner reference on WorkspaceIntegration %q: %w", name, err)
		}
		if err := m.client.Create(ctx, wi); err != nil {
			if apierrors.IsAlreadyExists(err) {
				// Raced with another reconcile that created it. Fetch the winner so the returned set
				// stays complete (one Get on the rare race path, not a blanket re-list every time).
				logger.V(1).Info("WorkspaceIntegration already exists, fetching the existing child", "workspaceintegration", name)
				raced := &workspacev1alpha1.WorkspaceIntegration{}
				if getErr := m.client.Get(ctx, client.ObjectKey{Name: name, Namespace: workspace.Namespace}, raced); getErr != nil {
					return nil, fmt.Errorf("failed to get raced WorkspaceIntegration %q: %w", name, getErr)
				}
				result = append(result, *raced)
				continue
			}
			return nil, fmt.Errorf("failed to create WorkspaceIntegration %q: %w", name, err)
		}
		logger.Info("Created WorkspaceIntegration shell", "workspaceintegration", name, "template", ref.Name)
		result = append(result, *wi)
	}

	// Delete stale children no longer backed by an integrationRefs entry.
	for i := range existing {
		child := &existing[i]
		if _, wanted := desiredNames[child.Name]; wanted {
			continue
		}
		if !child.DeletionTimestamp.IsZero() {
			continue // already being deleted
		}
		if err := m.client.Delete(ctx, child); err != nil && !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to delete stale WorkspaceIntegration %q: %w", child.Name, err)
		}
		logger.Info("Deleted stale WorkspaceIntegration shell", "workspaceintegration", child.Name)
	}

	return result, nil
}

// buildShellSpec builds the desired shell spec (templateRef + workspaceRef) for an integrationRef.
// It deliberately omits the resolved output fields -- the webhook owns those.
func (m *WorkspaceIntegrationManager) buildShellSpec(
	workspace *workspacev1alpha1.Workspace,
	ref *workspacev1alpha1.IntegrationTemplateRef,
) workspacev1alpha1.WorkspaceIntegrationSpec {
	return workspacev1alpha1.WorkspaceIntegrationSpec{
		TemplateRef: *ref.DeepCopy(),
		WorkspaceRef: workspacev1alpha1.WorkspaceRef{
			Name:      workspace.Name,
			Namespace: workspace.Namespace,
		},
	}
}
