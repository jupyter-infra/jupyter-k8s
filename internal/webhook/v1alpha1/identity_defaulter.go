/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"context"
	"fmt"
	"slices"
	"strconv"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/posixidentity"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
)

// IdentityDefaulter resolves a template's identity templates for a workspace at admission, allocating
// the owner's uid on first use, and writes the integers into the workspace's security contexts.
type IdentityDefaulter struct {
	templates *workspaceutil.TemplateResolver
	identity  *posixidentity.Resolver
}

// NewIdentityDefaulter returns an identity defaulter.
func NewIdentityDefaulter(k8sClient client.Client, defaultTemplateNamespace string, identity *posixidentity.Resolver) *IdentityDefaulter {
	return &IdentityDefaulter{
		templates: workspaceutil.NewTemplateResolver(k8sClient, defaultTemplateNamespace),
		identity:  identity,
	}
}

// ApplyIdentityDefaults applies the security context defaults of a template whose identity fields
// hold templates, resolved for the workspace's owner and namespace, and records the owner's uid in the
// posix-uid label. The label is written by this defaulter only: a user-supplied one is rejected on
// create, and the validator guards it on update.
func (d *IdentityDefaulter) ApplyIdentityDefaults(ctx context.Context, workspace *workspacev1alpha1.Workspace) error {
	// The admission request tells create from update, and dry-run from a real request. The webhook
	// server always provides it; a caller without one gets no identity resolution, which is only safe
	// for a template without identity templates (checked below, fail closed).
	req, reqErr := admission.RequestFromContext(ctx)

	// The posix-uid label is a claim: allocation and lazy recovery read it as "this owner's uid is N",
	// and recovery recreates a missing PosixIdentity for the workspace's owner from it. On create the
	// label can only come from the requester, since the operator never creates workspaces, so it is
	// untrusted: accepted, a user could claim another user's uid on their own new workspace (and, if
	// that uid's PosixIdentity were missing, have recovery hand it, and its files, to them).
	//
	// Rejected here, in the defaulter, rather than in the validator: by validation time the label set
	// below is indistinguishable from a user-supplied one. Rejected rather than silently dropped, so the
	// requester learns why; re-applying an exported manifest only needs the label removed, since the uid
	// comes from the PosixIdentity ledger anyway.
	//
	// Only on create: on update, the label already on the object was written by this defaulter. The
	// reserved-key policy (set-on-create-only) rejects changing or removing it, and
	// ValidatePosixUIDAdded rejects one added by an update unless it is the owner's resolved uid.
	if reqErr == nil && req.Operation == "CREATE" {
		if _, ok := workspace.Labels[workspaceutil.LabelPosixUID]; ok {
			return fmt.Errorf("label %s is set by the operator from the owner's PosixIdentity: remove it from "+
				"the workspace (e.g. when re-applying an exported manifest)", workspaceutil.LabelPosixUID)
		}
	}

	if workspace.Spec.TemplateRef == nil || workspace.Spec.TemplateRef.Name == "" {
		return nil
	}
	template, err := d.templates.ResolveTemplate(ctx, workspace.Spec.TemplateRef, workspace.Namespace)
	if err != nil {
		return err
	}
	// A template without identity templates is fully handled by applySecurityDefaults.
	if !template.Spec.HasIdentityTemplates() {
		return nil
	}
	// applySecurityDefaults skipped this template's security contexts for this step, so skipping it too
	// would admit the workspace with none.
	if reqErr != nil {
		return fmt.Errorf("template %q uses identity templates, which need the admission request to resolve: %w",
			template.Name, reqErr)
	}

	// A dry run must have no side effects (sideEffects=NoneOnDryRun): it peeks at the uid the owner
	// would get instead of allocating it.
	dryRun := req.DryRun != nil && *req.DryRun
	// The owner is the created-by annotation, not the requester: the controller or an admin updating
	// the workspace must not resolve it for themselves.
	identity, err := d.identity.Resolve(ctx, &template.Spec, workspace.Annotations[workspaceutil.AnnotationCreatedBy], workspace.Namespace, dryRun)
	if err != nil {
		return fmt.Errorf("failed to resolve the identity of template %q: %w", template.Name, err)
	}
	applyIdentity(workspace, identity)

	// Record the claim only for a template that references .User (UID is 0 otherwise): a shared
	// template running as the namespace gid claims no user uid.
	if identity.UID != 0 {
		if workspace.Labels == nil {
			workspace.Labels = map[string]string{}
		}
		// An existing label (an update) is never overwritten: it is immutable, and if it differs from the
		// resolved uid the validator rejects the request rather than this defaulter silently moving the
		// workspace to another uid, which would orphan its files.
		if _, ok := workspace.Labels[workspaceutil.LabelPosixUID]; !ok {
			workspace.Labels[workspaceutil.LabelPosixUID] = strconv.FormatInt(identity.UID, 10)
		}
	}
	return nil
}

// applyIdentity applies the resolved security context defaults like every template default: in whole
// to a workspace that sets none, otherwise only to the templated fields it leaves unset. The validator
// holds set fields to the resolved values.
func applyIdentity(workspace *workspacev1alpha1.Workspace, identity *posixidentity.Identity) {
	if workspace.Spec.PodSecurityContext == nil {
		workspace.Spec.PodSecurityContext = identity.Pod.DeepCopy()
	} else if resolved := identity.Pod; resolved != nil {
		pod := workspace.Spec.PodSecurityContext
		if identity.PodTemplated["runAsUser"] {
			fill(&pod.RunAsUser, resolved.RunAsUser)
		}
		if identity.PodTemplated["runAsGroup"] {
			fill(&pod.RunAsGroup, resolved.RunAsGroup)
		}
		if identity.PodTemplated["fsGroup"] {
			fill(&pod.FSGroup, resolved.FSGroup)
		}
		if identity.PodTemplated["supplementalGroups"] && len(pod.SupplementalGroups) == 0 {
			pod.SupplementalGroups = slices.Clone(resolved.SupplementalGroups)
		}
		// The template sets both, as identity templates require; the validator holds the workspace to them.
		fill(&pod.RunAsNonRoot, resolved.RunAsNonRoot)
		fill(&pod.SupplementalGroupsPolicy, resolved.SupplementalGroupsPolicy)
	}

	if workspace.Spec.ContainerSecurityContext == nil {
		workspace.Spec.ContainerSecurityContext = identity.Container.DeepCopy()
	} else if resolved := identity.Container; resolved != nil {
		container := workspace.Spec.ContainerSecurityContext
		if identity.ContainerTemplated["runAsUser"] {
			fill(&container.RunAsUser, resolved.RunAsUser)
		}
		if identity.ContainerTemplated["runAsGroup"] {
			fill(&container.RunAsGroup, resolved.RunAsGroup)
		}
	}
}

func fill[T any](field **T, value *T) {
	if *field == nil && value != nil {
		v := *value
		*field = &v
	}
}
