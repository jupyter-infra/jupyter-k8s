/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
)

// StorageValidator handles storage validation that needs cluster state (the workspace's PVC).
type StorageValidator struct {
	client client.Client
}

// NewStorageValidator creates a new StorageValidator.
func NewStorageValidator(k8sClient client.Client) *StorageValidator {
	return &StorageValidator{
		client: k8sClient,
	}
}

// ValidateStorageSizeNotShrinking rejects a storage size decrease below what the workspace's PVC
// already requests. Kubernetes does not support shrinking a PVC below its current size, so a
// resize-down otherwise passes admission (including dryRun=All) and then silently fails to
// reconcile. Rejecting it here makes admission authoritative for the shrink case.
//
// The check is anchored on the live PVC, not the old workspace spec:
//   - If no PVC exists yet (workspace never started, or storage never provisioned), there is
//     nothing Kubernetes can reject at reconcile, so any size is allowed.
//   - If the PVC exists, the new size must be >= the PVC's current requested size. Comparing to
//     the PVC (rather than the old spec) tracks what the API will actually accept, including the
//     narrow RecoverVolumeExpansionFailure path where a request may be lowered toward capacity.
//
// PVC lookup failures fail closed, matching the workspace validating webhook's failurePolicy=fail
// stance that admission is mandatory, not best-effort: a webhook that is up but blind (missing RBAC,
// API outage) must not silently wave shrinks through. Only NotFound is treated as success - it is
// the legitimate "no PVC yet, nothing to shrink" case (workspace never started or no storage
// provisioned). A transient error blocks the update and the caller retries, as with any
// failurePolicy=fail rejection.
//
// Growth is intentionally not blocked here: whether an increase succeeds depends on the
// StorageClass's allowVolumeExpansion, which the webhook does not resolve; that gotcha still
// surfaces at reconcile time.
func (sv *StorageValidator) ValidateStorageSizeNotShrinking(ctx context.Context, workspace *workspacev1alpha1.Workspace) error {
	if workspace.Spec.Storage == nil || workspace.Spec.Storage.Size.IsZero() {
		return nil
	}
	newSize := workspace.Spec.Storage.Size

	pvc := &corev1.PersistentVolumeClaim{}
	err := sv.client.Get(ctx, types.NamespacedName{
		Name:      controller.GeneratePVCName(workspace.Name),
		Namespace: workspace.Namespace,
	}, pvc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// No PVC yet: nothing provisioned to shrink.
			return nil
		}
		// Fail closed: we cannot confirm the current size, so we do not let the update through.
		workspacelog.Error(err, "Failed to get PVC for storage shrink validation; rejecting update",
			"workspace", workspace.GetName(), "namespace", workspace.GetNamespace())
		return fmt.Errorf("unable to validate storage size against existing volume: %w", err)
	}

	if violation := validateStorageSizeNotBelowProvisioned(newSize, pvc); violation != nil {
		return fmt.Errorf("workspace violates storage constraints: %s", violation.Message)
	}
	return nil
}

// validateStorageSizeNotBelowProvisioned returns a violation when newSize is smaller than the
// PVC's current requested storage. A PVC with no storage request recorded cannot be shrunk below
// an unknown value, so it is treated as allowed.
func validateStorageSizeNotBelowProvisioned(newSize resource.Quantity, pvc *corev1.PersistentVolumeClaim) *TemplateViolation {
	provisioned, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	if !ok || provisioned.IsZero() {
		return nil
	}

	if newSize.Cmp(provisioned) < 0 {
		return &TemplateViolation{
			Type:    ViolationTypeStorageExceeded,
			Field:   fieldStorageSize,
			Message: fmt.Sprintf("Storage size cannot be decreased from %s to %s: Kubernetes does not support shrinking a volume below its current size", provisioned.String(), newSize.String()),
			Allowed: fmt.Sprintf("size >= %s", provisioned.String()),
			Actual:  newSize.String(),
		}
	}

	return nil
}

// validateStorageSize checks if storage size is within template bounds
func validateStorageSize(size resource.Quantity, template *workspacev1alpha1.WorkspaceTemplate) *TemplateViolation {
	config := template.Spec.PrimaryStorage
	if config == nil {
		return nil
	}

	if config.MinSize != nil && size.Cmp(*config.MinSize) < 0 {
		return &TemplateViolation{
			Type:    ViolationTypeStorageExceeded,
			Field:   fieldStorageSize,
			Message: fmt.Sprintf("Storage size %s is below minimum %s required by template '%s'", size.String(), config.MinSize.String(), template.Name),
			Allowed: fmt.Sprintf("min: %s", config.MinSize.String()),
			Actual:  size.String(),
		}
	}

	if config.MaxSize != nil && size.Cmp(*config.MaxSize) > 0 {
		return &TemplateViolation{
			Type:    ViolationTypeStorageExceeded,
			Field:   fieldStorageSize,
			Message: fmt.Sprintf("Storage size %s exceeds maximum %s allowed by template '%s'", size.String(), config.MaxSize.String(), template.Name),
			Allowed: fmt.Sprintf("max: %s", config.MaxSize.String()),
			Actual:  size.String(),
		}
	}

	return nil
}

// validateTemplateStorageConsistency rejects a template whose primaryStorage bounds are
// self-contradictory (minSize > maxSize). Such a template can never admit any workspace storage
// size. CEL cannot express this on resource.Quantity, so it is enforced at template admission.
func validateTemplateStorageConsistency(template *workspacev1alpha1.WorkspaceTemplate) error {
	config := template.Spec.PrimaryStorage
	if config == nil || config.MinSize == nil || config.MaxSize == nil {
		return nil
	}

	if config.MinSize.Cmp(*config.MaxSize) > 0 {
		return fmt.Errorf(
			"primaryStorage.minSize %s is greater than primaryStorage.maxSize %s: no storage size can satisfy these bounds (template %q)",
			config.MinSize.String(), config.MaxSize.String(), template.GetName(),
		)
	}

	return nil
}
