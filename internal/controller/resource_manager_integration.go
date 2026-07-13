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

// RBAC for the resources capture reads. Deliberately narrow (get-only, named kind, no wildcard) per
// least-privilege -- a new referenced kind needs its own marker here, never a blanket grant.
// get-only (no list/watch) is sufficient because capture reads the referenced resource as an
// Unstructured, which controller-runtime v0.24.1 serves via a direct API get that bypasses the
// informer cache -- so no list/watch backing a cache is required. If a cached typed client is ever
// used for these reads, add list;watch.
// +kubebuilder:rbac:groups=ray.io,resources=rayclusters,verbs=get

// hasIntegrationTemplateRefs reports whether the workspace attaches any integrations.
func (rm *ResourceManager) hasIntegrationTemplateRefs(workspace *workspacev1alpha1.Workspace) bool {
	return len(workspace.Spec.IntegrationTemplateRefs) > 0
}

// reconcileIntegrations brings status.resolvedIntegrations up to date before the build: per ref, carry
// the frozen record forward or recapture (see hasIntegrationChanged), pruning removed integrations and
// persisting a changed set in one status patch. Fail-closed: a capture error preserves the existing record.
func (rm *ResourceManager) reconcileIntegrations(ctx context.Context, workspace *workspacev1alpha1.Workspace) error {
	logger := logf.FromContext(ctx)

	resolvedIntegrations := make([]workspacev1alpha1.ResolvedIntegration, 0, len(workspace.Spec.IntegrationTemplateRefs))
	changed := false
	var firstErr error

	for i := range workspace.Spec.IntegrationTemplateRefs {
		ref := &workspace.Spec.IntegrationTemplateRefs[i]
		currentParametersHash := getIntegrationParametersHash(ref)
		existing := findResolvedIntegration(&workspace.Status, ref.Name)

		// Load the template (cached read, not the referenced resource): feeds the gate and the capture.
		// Fail-closed if unreadable -- keep any existing record so the running sidecar survives.
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
		changed = true
		logger.Info("Integration input changed; recorded new resolved values",
			"integration", ref.Name, "parametersHash", currentParametersHash,
			"observedIntegrationTemplateVersion", currentTemplateVersion, "valueCount", len(values))
	}

	// Detect pruned records (integration removed from spec) as a change too.
	if len(resolvedIntegrations) != len(workspace.Status.ResolvedIntegrations) {
		changed = true
	}

	if changed {
		if err := rm.persistResolvedIntegrationsToStatus(ctx, workspace, resolvedIntegrations); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
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

// persistResolvedIntegrationsToStatus writes status.resolvedIntegrations via a status merge patch (won't clobber
// concurrent condition/probe writes) and updates the in-memory workspace so the same reconcile's build
// renders from the just-frozen values.
func (rm *ResourceManager) persistResolvedIntegrationsToStatus(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
	resolved []workspacev1alpha1.ResolvedIntegration,
) error {
	base := workspace.DeepCopy()
	workspace.Status.ResolvedIntegrations = resolved
	if err := rm.client.Status().Patch(ctx, workspace, client.MergeFrom(base)); err != nil {
		return fmt.Errorf("failed to persist status.resolvedIntegrations: %w", err)
	}
	return nil
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
