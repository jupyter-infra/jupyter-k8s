/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"context"
	"fmt"
	"path"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
)

// VolumeValidator handles volume validation for webhooks
type VolumeValidator struct {
	client client.Client
}

// NewVolumeValidator creates a new VolumeValidator
func NewVolumeValidator(k8sClient client.Client) *VolumeValidator {
	return &VolumeValidator{
		client: k8sClient,
	}
}

// ValidateVolumeOwnership checks that volumes don't reference PVCs owned by other workspaces
func (vv *VolumeValidator) ValidateVolumeOwnership(ctx context.Context, workspace *workspacev1alpha1.Workspace) error {
	if violation := validateVolumeOwnership(ctx, vv.client, workspace); violation != nil {
		return fmt.Errorf("workspace violates volume ownership constraints: %s", violation.Message)
	}
	return nil
}

// findDefaultVolume returns the template's default volume of the given name, or nil.
func findDefaultVolume(template *workspacev1alpha1.WorkspaceTemplate, name string) *workspacev1alpha1.VolumeSpec {
	if template == nil {
		return nil
	}
	for i := range template.Spec.DefaultVolumes {
		if template.Spec.DefaultVolumes[i].Name == name {
			return &template.Spec.DefaultVolumes[i]
		}
	}
	return nil
}

// isTemplateDefaultVolume reports whether the volume is one of the template's default volumes, same
// name and same settings.
func isTemplateDefaultVolume(volume workspacev1alpha1.VolumeSpec, template *workspacev1alpha1.WorkspaceTemplate) bool {
	defaultVolume := findDefaultVolume(template, volume.Name)
	return defaultVolume != nil && equality.Semantic.DeepEqual(*defaultVolume, volume)
}

// validateSecondaryStorages checks if secondary storage volumes are allowed by template. Only the
// volumes the workspace adds count: the template's own default volumes are always allowed.
func validateSecondaryStorages(volumes []workspacev1alpha1.VolumeSpec, template *workspacev1alpha1.WorkspaceTemplate) *TemplateViolation {
	// Check AllowSecondaryStorages setting (default is true if not specified)
	if template.Spec.AllowSecondaryStorages == nil || *template.Spec.AllowSecondaryStorages {
		return nil
	}

	var added []string
	for _, volume := range volumes {
		if !isTemplateDefaultVolume(volume, template) {
			added = append(added, volume.Name)
		}
	}
	if len(added) == 0 {
		return nil
	}

	return &TemplateViolation{
		Type:    ViolationTypeSecondaryStorageNotAllowed,
		Field:   "spec.volumes",
		Message: fmt.Sprintf("Template '%s' does not allow secondary storage volumes, but workspace adds %d volume(s): %s", template.Name, len(added), strings.Join(added, ", ")),
		Allowed: "no volumes beyond the template's defaultVolumes",
		Actual:  fmt.Sprintf("%d volume(s)", len(added)),
	}
}

// validateLockedVolumes holds a workspace to the template's default volumes when the template's
// volumeOverrides lock them: every default volume must be present with the template's settings.
func validateLockedVolumes(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) []TemplateViolation {
	if !template.Spec.VolumeOverrides.Locked() {
		return nil
	}

	var violations []TemplateViolation
	for _, defaultVolume := range template.Spec.DefaultVolumes {
		field := fmt.Sprintf("spec.volumes[%s]", defaultVolume.Name)
		var found *workspacev1alpha1.VolumeSpec
		for i := range workspace.Spec.Volumes {
			if workspace.Spec.Volumes[i].Name == defaultVolume.Name {
				found = &workspace.Spec.Volumes[i]
				break
			}
		}
		switch {
		case found == nil:
			violations = append(violations, TemplateViolation{
				Type:    ViolationTypeVolumeOverrideNotAllowed,
				Field:   field,
				Message: fmt.Sprintf("%s: required by template %q, which does not allow overriding its defaultVolumes; add it as defined in the template", field, template.Name),
				Allowed: "the template's defaultVolumes entry",
				Actual:  "missing",
			})
		case !equality.Semantic.DeepEqual(*found, defaultVolume):
			violations = append(violations, TemplateViolation{
				Type:    ViolationTypeVolumeOverrideNotAllowed,
				Field:   field,
				Message: fmt.Sprintf("%s: pinned to its definition in template %q, which does not allow overriding its defaultVolumes", field, template.Name),
				Allowed: "the template's defaultVolumes entry",
				Actual:  "a modified volume",
			})
		}
	}

	return violations
}

// validateUniqueMountPaths rejects two mounts of the primary container at the same path: the primary
// storage and the workspace's volumes. The operator's /dev/shm volume is left out, since a volume at
// /dev/shm replaces it.
//
// The CRD does not catch this. Without the check, the workspace passes admission and the API server
// rejects its Deployment at reconcile time, so the workspace fails to start instead of failing
// kubectl apply. A CEL rule cannot replace it: a pairwise rule over the unbounded volumes list exceeds
// the CEL cost budget unless volumes gets a maxItems, which could reject existing workspaces; the
// primary storage mount path is often unset in the spec, and only the controller resolves it to its
// default; and CEL cannot clean paths, so /data and /data/ would both pass.
func validateUniqueMountPaths(workspace *workspacev1alpha1.Workspace) *TemplateViolation {
	type mountSource struct {
		mountPath   string
		description string
	}
	var mounts []mountSource
	if storage := controller.ResolveStorageConfig(workspace); storage != nil {
		mounts = append(mounts, mountSource{storage.MountPath, "primary storage"})
	}
	for _, volume := range workspace.Spec.Volumes {
		mounts = append(mounts, mountSource{volume.MountPath, fmt.Sprintf("volume '%s'", volume.Name)})
	}

	seen := map[string]mountSource{}
	for _, mount := range mounts {
		if mount.mountPath == "" {
			continue
		}
		cleaned := path.Clean(mount.mountPath)
		if first, duplicate := seen[cleaned]; duplicate {
			return &TemplateViolation{
				Type:    ViolationTypeDuplicateMountPath,
				Field:   "spec.volumes",
				Message: fmt.Sprintf("mount path %s is used by both %s and %s", cleaned, first.description, mount.description),
				Allowed: "one mount per path",
				Actual:  cleaned,
			}
		}
		seen[cleaned] = mount
	}
	return nil
}

// validateVolumeOwnership checks that volumes don't reference PVCs owned by other workspaces
func validateVolumeOwnership(ctx context.Context, k8sClient client.Client, workspace *workspacev1alpha1.Workspace) *TemplateViolation {
	for _, volume := range workspace.Spec.Volumes {
		if volume.PersistentVolumeClaimName == "" {
			// Non-PVC volumes, such as emptyDir, have no PVC ownership to validate.
			continue
		}

		// Get the PVC
		pvc := &corev1.PersistentVolumeClaim{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      volume.PersistentVolumeClaimName,
			Namespace: workspace.Namespace,
		}, pvc)

		// If PVC doesn't exist, skip validation (let other validation handle it)
		if err != nil {
			continue
		}

		// Check if PVC is owned by another workspace
		for _, ownerRef := range pvc.OwnerReferences {
			if ownerRef.APIVersion == "workspace.jupyter.org/v1alpha1" &&
				ownerRef.Kind == "Workspace" &&
				ownerRef.UID != workspace.UID {
				return &TemplateViolation{
					Type:    ViolationTypeVolumeOwnedByAnotherWorkspace,
					Field:   fmt.Sprintf("spec.volumes[%s].persistentVolumeClaimName", volume.Name),
					Message: fmt.Sprintf("Volume '%s' references PVC '%s' which is owned by another workspace '%s'", volume.Name, volume.PersistentVolumeClaimName, ownerRef.Name),
					Allowed: "PVCs not owned by other workspaces",
					Actual:  fmt.Sprintf("PVC owned by workspace '%s'", ownerRef.Name),
				}
			}
		}
	}

	return nil
}
