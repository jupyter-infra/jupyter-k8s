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

	corev1 "k8s.io/api/core/v1"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/posixidentity"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
)

// ViolationTypeIdentityMismatch reports an identity field that differs from the value its template
// resolves for the workspace's owner and namespace.
const ViolationTypeIdentityMismatch = "IdentityMismatch"

// resolveIdentity resolves the security context defaults of a template whose identity fields hold
// templates, for the workspace's owner and namespace. It reads the owner's PosixIdentity and never
// allocates. It returns nil and no violation for a template without identity templates, and fails
// closed for a validator configured without an identity resolver: the pinned-context check skips such
// templates, so skipping here too would admit the workspace unchecked.
func (tv *TemplateValidator) resolveIdentity(
	ctx context.Context, workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate,
) (*posixidentity.Identity, []TemplateViolation) {
	if !template.Spec.HasIdentityTemplates() {
		return nil, nil
	}
	if tv.identity == nil {
		return nil, []TemplateViolation{{
			Type:    ViolationTypeIdentityMismatch,
			Field:   "spec.podSecurityContext",
			Message: fmt.Sprintf("template %q uses identity templates, but no identity resolver is configured to check them", template.Name),
		}}
	}
	identity, err := tv.identity.Resolve(ctx, &template.Spec, workspace.Annotations[workspaceutil.AnnotationCreatedBy], workspace.Namespace, true)
	if err != nil {
		return nil, []TemplateViolation{{
			Type:    ViolationTypeIdentityMismatch,
			Field:   "spec.podSecurityContext",
			Message: fmt.Sprintf("the identity of template %q cannot be resolved: %v", template.Name, err),
		}}
	}
	return identity, nil
}

// validateIdentity holds a workspace's templated identity fields to their resolved values, whatever the
// template's security context override policies: a user never picks a uid.
func validateIdentity(workspace *workspacev1alpha1.Workspace, identity *posixidentity.Identity) []TemplateViolation {
	var violations []TemplateViolation
	pod := workspace.Spec.PodSecurityContext
	if pod == nil {
		pod = &corev1.PodSecurityContext{}
	}
	resolvedPod := identity.Pod
	if resolvedPod == nil {
		resolvedPod = &corev1.PodSecurityContext{}
	}
	check := func(templated bool, field string, want, got *int64) {
		if templated && want != nil && (got == nil || *got != *want) {
			violations = append(violations, identityViolation(field, strconv.FormatInt(*want, 10), formatID(got)))
		}
	}
	check(identity.PodTemplated["runAsUser"], "spec.podSecurityContext.runAsUser", resolvedPod.RunAsUser, pod.RunAsUser)
	check(identity.PodTemplated["runAsGroup"], "spec.podSecurityContext.runAsGroup", resolvedPod.RunAsGroup, pod.RunAsGroup)
	check(identity.PodTemplated["fsGroup"], "spec.podSecurityContext.fsGroup", resolvedPod.FSGroup, pod.FSGroup)
	if identity.PodTemplated["supplementalGroups"] && !slices.Equal(resolvedPod.SupplementalGroups, pod.SupplementalGroups) {
		violations = append(violations, identityViolation("spec.podSecurityContext.supplementalGroups",
			fmt.Sprint(resolvedPod.SupplementalGroups), fmt.Sprint(pod.SupplementalGroups)))
	}
	if pod.RunAsNonRoot == nil || !*pod.RunAsNonRoot {
		violations = append(violations, identityViolation("spec.podSecurityContext.runAsNonRoot", "true", "unset or false"))
	}
	if pod.SupplementalGroupsPolicy == nil || *pod.SupplementalGroupsPolicy != corev1.SupplementalGroupsPolicyStrict {
		violations = append(violations, identityViolation("spec.podSecurityContext.supplementalGroupsPolicy",
			string(corev1.SupplementalGroupsPolicyStrict), "unset or Merge"))
	}

	container := workspace.Spec.ContainerSecurityContext
	if container == nil {
		container = &corev1.SecurityContext{}
	}
	if resolved := identity.Container; resolved != nil {
		check(identity.ContainerTemplated["runAsUser"], "spec.containerSecurityContext.runAsUser", resolved.RunAsUser, container.RunAsUser)
		check(identity.ContainerTemplated["runAsGroup"], "spec.containerSecurityContext.runAsGroup", resolved.RunAsGroup, container.RunAsGroup)
	}

	if identity.UID != 0 {
		if claimed := workspace.Labels[workspaceutil.LabelPosixUID]; claimed != strconv.FormatInt(identity.UID, 10) {
			violations = append(violations, identityViolation("metadata.labels["+workspaceutil.LabelPosixUID+"]",
				strconv.FormatInt(identity.UID, 10), claimed))
		}
	}
	return violations
}

// ValidatePosixUIDAdded rejects a posix-uid label added by an update unless it is the uid the
// workspace's template resolves for its owner: only the identity defaulter adds it. Changes and removals
// are rejected by the reserved-prefix validation.
func (tv *TemplateValidator) ValidatePosixUIDAdded(ctx context.Context, oldWorkspace, newWorkspace *workspacev1alpha1.Workspace) error {
	added := newWorkspace.Labels[workspaceutil.LabelPosixUID]
	if added == "" || oldWorkspace.Labels[workspaceutil.LabelPosixUID] != "" {
		return nil
	}
	notAllowed := fmt.Errorf("label '%s' is set by the operator only", workspaceutil.LabelPosixUID)
	if tv.identity == nil || newWorkspace.Spec.TemplateRef == nil {
		return notAllowed
	}
	template, err := tv.fetchTemplate(ctx, newWorkspace.Spec.TemplateRef, newWorkspace.Namespace)
	if err != nil {
		return err
	}
	if !template.Spec.HasIdentityTemplates() {
		return notAllowed
	}
	identity, err := tv.identity.Resolve(ctx, &template.Spec, newWorkspace.Annotations[workspaceutil.AnnotationCreatedBy], newWorkspace.Namespace, true)
	if err != nil {
		return err
	}
	if identity.UID == 0 || added != strconv.FormatInt(identity.UID, 10) {
		return notAllowed
	}
	return nil
}

func identityViolation(field, allowed, actual string) TemplateViolation {
	return TemplateViolation{
		Type:    ViolationTypeIdentityMismatch,
		Field:   field,
		Message: fmt.Sprintf("%s: must be %s, set by the template's identity templates", field, allowed),
		Allowed: allowed,
		Actual:  actual,
	}
}

func formatID(id *int64) string {
	if id == nil {
		return "unset"
	}
	return strconv.FormatInt(*id, 10)
}
