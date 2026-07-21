/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// This file is the capture side of integrations: it reads each integration's referenced resource and
// freezes the resolved substitutions into status.resolvedIntegrations -- the ONLY place that reads a
// referenced resource. The build side renders the pod from those frozen values.
// Functions are ordered entry points first, then the helpers they call.
//
// Recapture policy (hasIntegrationChanged): recapture on a parametersHash or templateVersion drift.
// The template-version trigger is load-bearing beyond rolling the pod -- the build renders the pod SHAPE
// from the live template every reconcile, so a shape edit rolls the pod regardless; recapture additionally
// refreezes the {{ resource }} VALUES to match, so an edit that adds a new {{ resource }} doesn't leave the
// frozen set missing a key (which would hard-error the build's frozen-replay).

// RBAC for the resources capture reads: the operator gets the referenced resource as an Unstructured
// (get-only is sufficient -- controller-runtime v0.24.1 serves an Unstructured Get directly from the API,
// bypassing the informer cache, so no list/watch backing a cache is required).
//
// There is deliberately NO +kubebuilder:rbac marker for these reads and the operator chart grants no
// cross-API read by default: an integration's referenced kinds (e.g. ray.io rayclusters) are specific to a
// deployment, not the generic operator, so baking them into the default ClusterRole would grant every
// install read on APIs it never uses. A deployment that configures integrations supplies the read grant
// out-of-band -- the same model as watched resources (--watch-resources-gvk / accessResources.additionalGvk),
// which the generic chart also does not auto-grant.

// hasIntegrationTemplateRefs reports whether the workspace attaches any integrations.
func (rm *ResourceManager) hasIntegrationTemplateRefs(workspace *workspacev1alpha1.Workspace) bool {
	return len(workspace.Spec.IntegrationTemplateRefs) > 0
}

// reconcileIntegrations brings status.resolvedIntegrations up to date before the build and returns the
// templates it loaded, keyed by integration name, so the build renders each pod shape without re-fetching.
// Per ref it loads the template once (feeding both the recapture gate and the build), then carries the
// frozen record forward or recaptures (see hasIntegrationChanged), pruning removed integrations. It mutates
// workspace.Status.ResolvedIntegrations IN MEMORY only -- the reconcile's single status write (via
// status_manager) persists it -- so status is written at most once per reconcile. Fail-closed: a template
// that can't be loaded, or a capture that fails, preserves the existing frozen record and is reported as an
// error; a template absent from the returned map makes the build preserve the running pod for that ref.
func (rm *ResourceManager) reconcileIntegrations(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
) (map[string]*workspacev1alpha1.WorkspaceIntegrationTemplate, error) {
	logger := logf.FromContext(ctx)

	templates := make(map[string]*workspacev1alpha1.WorkspaceIntegrationTemplate, len(workspace.Spec.IntegrationTemplateRefs))
	resolvedIntegrations := make([]workspacev1alpha1.ResolvedIntegration, 0, len(workspace.Spec.IntegrationTemplateRefs))
	var firstErr error

	for i := range workspace.Spec.IntegrationTemplateRefs {
		ref := &workspace.Spec.IntegrationTemplateRefs[i]
		currentParametersHash := getIntegrationParametersHash(ref)
		existing := findResolvedIntegration(&workspace.Status, ref.Name)

		// Load the template (cached read, not the referenced resource): feeds the gate, the capture, and
		// the build. Fail-closed if unreadable -- keep any existing record so the running sidecar survives.
		template, err := getIntegrationTemplate(ctx, rm.client, workspace, ref)
		if err != nil {
			if existing != nil {
				resolvedIntegrations = append(resolvedIntegrations, *existing)
				logger.Error(err, "integration template unreadable; preserving previously frozen values",
					"integration", ref.Name)
			} else {
				logger.Error(err, "integration template unreadable on first attach; no frozen values yet",
					"integration", ref.Name)
			}
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		templates[ref.Name] = template
		currentTemplateVersion := getIntegrationTemplateVersion(template)

		if !hasIntegrationChanged(existing, currentParametersHash, currentTemplateVersion) {
			// Freeze holds: neither version moved. Carry the frozen record forward unchanged -- no
			// resource read, and the build renders from these same values, so no pod roll.
			resolvedIntegrations = append(resolvedIntegrations, *existing)
			continue
		}

		// Recapture: first attach / switch / param edit / template edit. Reads the referenced resource.
		values, err := rm.captureIntegrationValues(ctx, workspace, ref, template)
		if err != nil {
			// Fail-closed: preserve the prior record (if any) so the running sidecar survives, and do NOT
			// advance the versions, so a later reconcile retries the capture.
			if existing != nil {
				resolvedIntegrations = append(resolvedIntegrations, *existing)
				logger.Error(err, "integration recapture failed; preserving previously frozen values",
					"integration", ref.Name)
			} else {
				logger.Error(err, "integration capture failed on first attach; no frozen values yet",
					"integration", ref.Name)
			}
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		resolvedIntegrations = append(resolvedIntegrations, workspacev1alpha1.ResolvedIntegration{
			Name:                               ref.Name,
			ParametersHash:                     currentParametersHash,
			ObservedIntegrationTemplateVersion: currentTemplateVersion,
			Values:                             values,
		})
		logger.Info("Integration input changed; recorded new resolved values",
			"integration", ref.Name, "parametersHash", currentParametersHash,
			"observedIntegrationTemplateVersion", currentTemplateVersion, "valueCount", len(values))
	}

	// Record the reconciled set on the in-memory workspace so the build renders from it and the reconcile's
	// single status write persists it. Pruned records (integration removed from spec) fall out naturally.
	workspace.Status.ResolvedIntegrations = resolvedIntegrations
	return templates, firstErr
}

// captureIntegrationValues renders the pod modifications with a recording provider and returns the
// recorded value map to freeze (the rendered output itself is discarded; the build re-renders from these
// values). Fail-closed.
func (rm *ResourceManager) captureIntegrationValues(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
	ref *workspacev1alpha1.IntegrationTemplateRef,
	template *workspacev1alpha1.WorkspaceIntegrationTemplate,
) (map[string]string, error) {
	pods := template.Spec.DeploymentModifications != nil && template.Spec.DeploymentModifications.PodModifications != nil
	// Nothing that references a resource: neither pod modifications nor a probe command to freeze.
	if !pods && template.Spec.StatusProbe == nil {
		return map[string]string{}, nil
	}

	data := buildIntegrationTemplateData(workspace, ref)

	// Read every referenced resource (resolving its templated name/namespace first) -- fail-closed on a miss.
	refResolver := NewIntegrationTemplateResolver(nil)
	resources, err := rm.fetchReferencedResources(ctx, template, data, refResolver)
	if err != nil {
		return nil, err
	}

	// Render once with a capturing provider; discard the output, keep the captured map. Both the pod
	// modifications AND the statusProbe command are rendered so every {{ resource }} they reference is
	// frozen -- the probe replays from this same map, so a probe-only expression must be captured here too.
	live := NewLiveResourceValueProvider(resources)
	resolver := NewIntegrationTemplateResolver(live)
	if pods {
		if _, err := resolver.ResolvePodModifications(template.Spec.DeploymentModifications.PodModifications, data); err != nil {
			return nil, fmt.Errorf("failed to render pod modifications for integration %q: %w", ref.Name, err)
		}
	}
	if _, err := resolveStatusProbeCommand(resolver, template.Spec.StatusProbe, data); err != nil {
		return nil, fmt.Errorf("integration %q: %w", ref.Name, err)
	}
	return live.Captured(), nil
}

// fetchReferencedResources reads each resourceRef's live object (unstructured). Fail-closed: a
// missing/forbidden ref errors so no partial overlay is applied.
func (rm *ResourceManager) fetchReferencedResources(
	ctx context.Context,
	template *workspacev1alpha1.WorkspaceIntegrationTemplate,
	data IntegrationTemplateData,
	refResolver *IntegrationTemplateResolver,
) (map[string]*unstructured.Unstructured, error) {
	resources := make(map[string]*unstructured.Unstructured, len(template.Spec.ResourceRefs))
	for i := range template.Spec.ResourceRefs {
		ref := &template.Spec.ResourceRefs[i]
		effectiveRef := *ref
		if effectiveRef.Metadata.Namespace == "" {
			effectiveRef.Metadata.Namespace = data.Workspace.Namespace
		}
		resolvedName, resolvedNamespace, err := refResolver.ResolveResourceRef(&effectiveRef, data)
		if err != nil {
			return nil, err
		}
		gvk := schema.FromAPIVersionAndKind(ref.APIVersion, ref.Kind)
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(gvk)
		if err := rm.client.Get(ctx, client.ObjectKey{Name: resolvedName, Namespace: resolvedNamespace}, obj); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("resourceRef %q: %s %q not found in namespace %q", ref.Name, ref.Kind, resolvedName, resolvedNamespace)
			}
			return nil, fmt.Errorf("resourceRef %q: failed to get %s %q in namespace %q: %w", ref.Name, ref.Kind, resolvedName, resolvedNamespace, err)
		}
		resources[ref.Name] = obj
	}
	return resources, nil
}

// hasIntegrationChanged reports whether an integration's parametersHash or templateVersion drifts from
// the frozen record (or it was never captured). It is the recapture gate -- see the file header for why
// the template-version trigger is load-bearing.
func hasIntegrationChanged(existing *workspacev1alpha1.ResolvedIntegration, currentParametersHash, currentTemplateVersion string) bool {
	if existing == nil {
		return true // never captured yet
	}
	return existing.ParametersHash != currentParametersHash ||
		existing.ObservedIntegrationTemplateVersion != currentTemplateVersion
}

// getIntegrationParametersHash hashes only the user-controlled input (templateRef namespace+name +
// sorted params), never anything from the referenced resource -- so external drift can't change it.
// Nil ref -> "".
func getIntegrationParametersHash(ref *workspacev1alpha1.IntegrationTemplateRef) string {
	if ref == nil {
		return ""
	}
	h := sha256.New()
	// sha256's Write never returns an error; ignore it explicitly to satisfy errcheck.
	_, _ = fmt.Fprintf(h, "ref\x00%s\x00%s\x00", ref.Namespace, ref.Name)
	params := ref.ParametersMap()
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		_, _ = fmt.Fprintf(h, "p\x00%s\x00%s\x00", k, params[k])
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// getIntegrationTemplateVersion is "<UID>.<Generation>" -- changes on a template edit (Generation) or
// replace (UID).
func getIntegrationTemplateVersion(template *workspacev1alpha1.WorkspaceIntegrationTemplate) string {
	return fmt.Sprintf("%s.%d", template.UID, template.Generation)
}
