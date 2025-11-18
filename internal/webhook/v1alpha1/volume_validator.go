/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
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

// validateSecondaryStorages checks if secondary storage volumes are allowed by template
func validateSecondaryStorages(volumes []workspacev1alpha1.VolumeSpec, template *workspacev1alpha1.WorkspaceTemplate) *TemplateViolation {
	// Skip validation if no volumes specified
	if len(volumes) == 0 {
		return nil
	}

	// Check AllowSecondaryStorages setting (default is true if not specified)
	if template.Spec.AllowSecondaryStorages != nil && !*template.Spec.AllowSecondaryStorages {
		return &TemplateViolation{
			Type:    ViolationTypeSecondaryStorageNotAllowed,
			Field:   "spec.volumes",
			Message: fmt.Sprintf("Template '%s' does not allow secondary storage volumes, but workspace specifies %d volume(s)", template.Name, len(volumes)),
			Allowed: "no secondary volumes",
			Actual:  fmt.Sprintf("%d volume(s)", len(volumes)),
		}
	}

	return nil
}

// validateVolumeOwnership checks that volumes don't reference PVCs owned by other workspaces
func validateVolumeOwnership(ctx context.Context, k8sClient client.Client, workspace *workspacev1alpha1.Workspace) *TemplateViolation {
	for _, volume := range workspace.Spec.Volumes {
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
