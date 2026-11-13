/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"context"
	"fmt"
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

	// On update, resolve only when the workspace still lacks an identity field or the posix-uid label.
	// Resolution reads the owner's PosixIdentity and the namespace's gid, and fails when either is
	// broken (a conflict, a lost label). Failing here would block every update, a stop and the
	// operator's own writes (idle shutdown) included, before the validator's exemptions for those apply.
	// Skipping is safe: the validator still resolves and holds every other update to the resolved values.
	if req.Operation == "UPDATE" && !needsIdentity(workspace, template) {
		return nil
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

// needsIdentity reports whether the defaulter has something to fill on a workspace: a security
// context the template has a default for and the workspace leaves unset, or the posix-uid label of a
// template referencing .User. It reads the template only, without resolving any input.
func needsIdentity(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) bool {
	if workspace.Spec.PodSecurityContext == nil && template.Spec.DefaultPodSecurityContext != nil {
		return true
	}
	if workspace.Spec.ContainerSecurityContext == nil && template.Spec.DefaultContainerSecurityContext != nil {
		return true
	}
	if posixidentity.ReferencesUser(&template.Spec) {
		if _, ok := workspace.Labels[workspaceutil.LabelPosixUID]; !ok {
			return true
		}
	}
	return false
}

// applyIdentity applies the resolved security context defaults like every template default: each
// context in whole, only to a workspace that sets none. A workspace setting a context gets nothing
// merged into it; the validator holds its identity fields to the resolved values, naming any that
// differ.
func applyIdentity(workspace *workspacev1alpha1.Workspace, identity *posixidentity.Identity) {
	if workspace.Spec.PodSecurityContext == nil {
		workspace.Spec.PodSecurityContext = identity.Pod.DeepCopy()
	}
	if workspace.Spec.ContainerSecurityContext == nil {
		workspace.Spec.ContainerSecurityContext = identity.Container.DeepCopy()
	}
}
