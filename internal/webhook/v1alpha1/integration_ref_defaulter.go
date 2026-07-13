/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// IntegrationRefDefaulter resolves and STAMPS the namespace of each integrationTemplateRef that omits one,
// in the Workspace mutating webhook -- so the ref is fully-qualified before it is stored, and every later
// reader (the validating webhook, the controller resolver) sees an explicit namespace instead of
// re-inferring it. Resolution mirrors the controller's order: the workspace's own namespace first, then
// the configured shared namespace.
//
// Confused-deputy safety: this only resolves refs whose namespace is EMPTY, and an empty namespace can
// only mean the workspace's own namespace or the shared namespace -- both allowed targets. It never reads
// an arbitrary user-named namespace (an explicit ref.Namespace is left untouched here and scope-checked by
// the validating webhook). So the two reads it may issue are always to namespaces the user may reference.
//
// Best-effort: if the template is found in neither namespace, the ref is left unstamped (namespace stays
// empty) and the validating webhook -- reading the ref's now-defaulted (own) namespace -- produces the
// authoritative "not found" rejection, keeping error messaging in one place. A transient/API read error is
// returned so the (fail-closed) mutating webhook surfaces it rather than silently storing an unresolved ref.
type IntegrationRefDefaulter struct {
	client          client.Client
	sharedNamespace string
}

// NewIntegrationRefDefaulter creates the defaulter. sharedNamespace is the cluster-wide shared namespace
// (the operator's --default-template-namespace flag) used as the fallback target, exactly as the
// IntegrationTemplateRefValidator and the WorkspaceTemplate resolver use it.
func NewIntegrationRefDefaulter(c client.Client, sharedNamespace string) *IntegrationRefDefaulter {
	return &IntegrationRefDefaulter{client: c, sharedNamespace: sharedNamespace}
}

// ApplyIntegrationRefDefaults stamps the resolved namespace onto every integrationTemplateRef that omits
// one. Refs that already carry a namespace are left untouched (the validating webhook scope-checks those).
// Idempotent: on a re-admission (e.g. a controller metadata update) every ref is already stamped, so this
// is a no-op and never re-resolves -- which is what keeps a since-deleted template from wedging reconciles.
func (d *IntegrationRefDefaulter) ApplyIntegrationRefDefaults(ctx context.Context, workspace *workspacev1alpha1.Workspace) error {
	log := logf.FromContext(ctx).WithName("integration-ref-defaulter").
		WithValues("workspace", workspace.GetName(), "namespace", workspace.GetNamespace())

	for i := range workspace.Spec.IntegrationTemplateRefs {
		ref := &workspace.Spec.IntegrationTemplateRefs[i]
		if ref.Namespace != "" {
			// Explicit namespace: leave as-is; the validating webhook enforces scope on it.
			continue
		}

		resolved, err := d.resolveNamespace(ctx, ref.Name, workspace.Namespace)
		if err != nil {
			return err
		}
		if resolved == "" {
			// Not found in own or shared namespace: leave unstamped and let the validating webhook reject
			// with the authoritative message naming both namespaces.
			log.V(1).Info("Could not resolve integrationTemplateRef namespace; leaving unstamped for the validator to reject",
				"integrationTemplateRef", ref.Name)
			continue
		}

		ref.Namespace = resolved
		log.Info("Stamped integrationTemplateRef namespace",
			"integrationTemplateRef", ref.Name, "resolvedNamespace", resolved)
	}
	return nil
}

// resolveNamespace returns the namespace a by-name ref resolves to: the workspace's own namespace if the
// template exists there, else the shared namespace if it exists there, else "" (not found in either). A
// non-NotFound (transient/API) error is returned to the caller.
func (d *IntegrationRefDefaulter) resolveNamespace(ctx context.Context, name, workspaceNamespace string) (string, error) {
	tmpl := &workspacev1alpha1.WorkspaceIntegrationTemplate{}

	err := d.client.Get(ctx, client.ObjectKey{Name: name, Namespace: workspaceNamespace}, tmpl)
	if err == nil {
		return workspaceNamespace, nil
	}
	if !errors.IsNotFound(err) {
		return "", fmt.Errorf("failed to resolve integration template %q in namespace %q: %w", name, workspaceNamespace, err)
	}

	// Not in the workspace namespace: try the shared namespace if one is configured and different.
	if d.sharedNamespace == "" || d.sharedNamespace == workspaceNamespace {
		return "", nil
	}
	err = d.client.Get(ctx, client.ObjectKey{Name: name, Namespace: d.sharedNamespace}, tmpl)
	if err == nil {
		return d.sharedNamespace, nil
	}
	if !errors.IsNotFound(err) {
		return "", fmt.Errorf("failed to resolve integration template %q in shared namespace %q: %w", name, d.sharedNamespace, err)
	}
	return "", nil
}
