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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// BuildWorkspaceIntegration resolves a WorkspaceIntegration against the live referenced resources
// and its templateRef parameters, freezing the resolved output directly onto the WorkspaceIntegration's
// own spec fields (DeploymentModifications, ShareProcessNamespace, StatusProbe) with all template
// expressions substituted to literal values. This mirrors how a Workspace carries its resolved
// WorkspaceTemplate values on its own spec fields -- no bespoke "resolved" envelope.
//
// It is the admission-time entry point for the WorkspaceIntegration mutating webhook: the webhook
// calls it once at the WorkspaceIntegration's own admission (CREATE/UPDATE). The workspace controller
// never calls this at reconcile time -- it only reads the already-resolved child.
//
// Resolution is fail-closed: any resourceRef that cannot be resolved or fetched, or any template
// expression that resolves to empty (e.g. a not-yet-ready RayCluster whose .status.head.serviceName
// is blank), returns an error so admission rejects the request and the prior good state is
// preserved. On error the wi spec fields are left unchanged.
//
// The template's deploymentModifications keeps its native PodModifications shape (MergeEnv stays
// []AccessEnvTemplate, its ValueTemplate now holding a literal); the statusProbe is frozen as-is
// (it carries no template expressions today).
func BuildWorkspaceIntegration(
	ctx context.Context,
	c client.Client,
	wi *workspacev1alpha1.WorkspaceIntegration,
	template *workspacev1alpha1.WorkspaceIntegrationTemplate,
) error {
	if wi == nil {
		return fmt.Errorf("cannot build nil WorkspaceIntegration")
	}
	if template == nil {
		return fmt.Errorf("cannot build WorkspaceIntegration %q from nil template", wi.Name)
	}

	// A WorkspaceIntegration is always co-located with its Workspace (cross-namespace references
	// are unsupported). Reject a workspaceRef.namespace that names a different namespace rather
	// than silently resolving against the wrong (or a nonexistent) workspace. Empty is allowed and
	// defaults to the WI's own namespace below.
	if ns := wi.Spec.WorkspaceRef.Namespace; ns != "" && ns != wi.Namespace {
		return fmt.Errorf(
			"WorkspaceIntegration %q: workspaceRef.namespace %q must match the object's own namespace %q "+
				"(cross-namespace references are not supported)", wi.Name, ns, wi.Namespace)
	}

	// shareProcessNamespace is a static pod-level toggle on the template (no resolution needed);
	// carry it through. A template value of false (or unset) is intentionally frozen as nil, not
	// *false: the controller OR-reduces this flag across all of a workspace's integrations when
	// applying, so nil and *false are semantically equivalent and an integration never needs to
	// force the pod-level flag back off on its own.
	var shareProcessNamespace *bool
	if template.Spec.ShareProcessNamespace != nil && *template.Spec.ShareProcessNamespace {
		enabled := true
		shareProcessNamespace = &enabled
	}

	// Resolve the deploymentModifications (if any) before mutating wi, so a failure leaves the
	// prior good spec untouched.
	var deploymentMods *workspacev1alpha1.DeploymentModifications
	if template.Spec.DeploymentModifications != nil &&
		template.Spec.DeploymentModifications.PodModifications != nil {

		// Build template data: workspace identity (from the WI's workspaceRef) + the templateRef's
		// flattened parameters. The workspace namespace defaults to the WI's own namespace.
		workspaceNamespace := wi.Spec.WorkspaceRef.Namespace
		if workspaceNamespace == "" {
			workspaceNamespace = wi.Namespace
		}
		tmplData := IntegrationTemplateData{
			Workspace: IntegrationWorkspaceData{
				Name:      wi.Spec.WorkspaceRef.Name,
				Namespace: workspaceNamespace,
			},
			Parameters: wi.Spec.TemplateRef.ParametersMap(),
		}

		resolver := NewIntegrationTemplateResolver()

		// Resolve+fetch each referenced resource into a map keyed by ref id so
		// {{ resource "<id>" "<jsonpath>" }} expressions can address them.
		resources, err := fetchReferencedResources(ctx, c, template, tmplData, resolver)
		if err != nil {
			return fmt.Errorf("integration template %q: %w", template.Name, err)
		}

		// Resolve all template expressions in the pod modifications into literals, keeping the
		// native PodModifications shape (including MergeEnv []AccessEnvTemplate).
		mods, err := resolver.ResolvePodModifications(
			template.Spec.DeploymentModifications.PodModifications,
			tmplData,
			resources,
		)
		if err != nil {
			return fmt.Errorf("integration template %q: failed to resolve pod modifications: %w", template.Name, err)
		}
		if mods != nil {
			deploymentMods = &workspacev1alpha1.DeploymentModifications{PodModifications: mods}
		}
	}

	// Commit the resolved output onto the wi spec. Assign every output field unconditionally so a
	// re-build (e.g. switching clusters) never leaves a stale value from a prior resolution.
	wi.Spec.ShareProcessNamespace = shareProcessNamespace
	wi.Spec.StatusProbe = template.Spec.StatusProbe.DeepCopy()
	wi.Spec.DeploymentModifications = deploymentMods

	return nil
}

// fetchReferencedResources resolves every resourceRef's name/namespace templates and fetches each
// target resource via the unstructured client, returning a map keyed by the ref id. Fail-closed:
// any ref that cannot be resolved or fetched aborts the whole build (so admission rejects).
func fetchReferencedResources(
	ctx context.Context,
	c client.Client,
	template *workspacev1alpha1.WorkspaceIntegrationTemplate,
	tmplData IntegrationTemplateData,
	resolver *IntegrationTemplateResolver,
) (map[string]*unstructured.Unstructured, error) {
	resources := make(map[string]*unstructured.Unstructured, len(template.Spec.ResourceRefs))

	for i := range template.Spec.ResourceRefs {
		ref := &template.Spec.ResourceRefs[i]

		// Default the namespace template to the workspace namespace when unset.
		effectiveRef := *ref
		if effectiveRef.Namespace == "" {
			effectiveRef.Namespace = tmplData.Workspace.Namespace
		}

		resolvedName, resolvedNamespace, err := resolver.ResolveResourceRef(&effectiveRef, tmplData)
		if err != nil {
			return nil, err
		}

		gvk := schema.FromAPIVersionAndKind(ref.APIVersion, ref.Kind)
		resource := &unstructured.Unstructured{}
		resource.SetGroupVersionKind(gvk)

		err = c.Get(ctx, client.ObjectKey{Name: resolvedName, Namespace: resolvedNamespace}, resource)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("resourceRef %q: %s %q not found in namespace %q",
					ref.ID, ref.Kind, resolvedName, resolvedNamespace)
			}
			if apierrors.IsForbidden(err) {
				return nil, fmt.Errorf("resourceRef %q: insufficient permissions to get %s %q in namespace %q: %w",
					ref.ID, ref.Kind, resolvedName, resolvedNamespace, err)
			}
			return nil, fmt.Errorf("resourceRef %q: failed to get %s %q in namespace %q: %w",
				ref.ID, ref.Kind, resolvedName, resolvedNamespace, err)
		}

		resources[ref.ID] = resource
	}

	return resources, nil
}
