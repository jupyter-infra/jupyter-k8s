package controller

import (
	"context"
	"fmt"
	"reflect"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// PVCBuilder handles creation of PersistentVolumeClaim resources for Workspace
type PVCBuilder struct {
	scheme *runtime.Scheme
}

// NewPVCBuilder creates a new PVCBuilder
func NewPVCBuilder(scheme *runtime.Scheme) *PVCBuilder {
	return &PVCBuilder{
		scheme: scheme,
	}
}

// ResolvedStorageConfig contains all resolved storage configuration
type ResolvedStorageConfig struct {
	Size             resource.Quantity
	StorageClassName *string
	MountPath        string
}

// resolveStorageSize returns the storage size from workspace, with fallback to default
func resolveStorageSize(workspace *workspacev1alpha1.Workspace) resource.Quantity {
	if workspace.Spec.Storage != nil && !workspace.Spec.Storage.Size.IsZero() {
		return workspace.Spec.Storage.Size
	}
	return resource.MustParse("10Gi")
}

// resolveStorageClassName returns the storage class name from workspace
func resolveStorageClassName(workspace *workspacev1alpha1.Workspace) *string {
	if workspace.Spec.Storage != nil && workspace.Spec.Storage.StorageClassName != nil {
		return workspace.Spec.Storage.StorageClassName
	}
	return nil
}

// resolveMountPath returns the mount path from workspace, with fallback to default
func resolveMountPath(workspace *workspacev1alpha1.Workspace) string {
	if workspace.Spec.Storage != nil && workspace.Spec.Storage.MountPath != "" {
		return workspace.Spec.Storage.MountPath
	}
	return DefaultMountPath
}

// ResolveStorageConfig determines storage configuration from workspace
// Returns nil if no storage is requested
func ResolveStorageConfig(workspace *workspacev1alpha1.Workspace) *ResolvedStorageConfig {
	// Check if storage is requested
	if workspace.Spec.Storage == nil {
		return nil
	}

	return &ResolvedStorageConfig{
		Size:             resolveStorageSize(workspace),
		StorageClassName: resolveStorageClassName(workspace),
		MountPath:        resolveMountPath(workspace),
	}
}

// BuildPVC creates a PersistentVolumeClaim resource for the given Workspace
// It uses workspace storage configuration
func (pb *PVCBuilder) BuildPVC(workspace *workspacev1alpha1.Workspace) (*corev1.PersistentVolumeClaim, error) {
	storageConfig := ResolveStorageConfig(workspace)
	if storageConfig == nil {
		return nil, nil // No storage requested
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: pb.buildObjectMeta(workspace),
		Spec:       pb.buildPVCSpecWithSize(storageConfig.Size, storageConfig.StorageClassName),
	}

	// Set owner reference for garbage collection
	if err := controllerutil.SetControllerReference(workspace, pvc, pb.scheme); err != nil {
		return nil, fmt.Errorf("failed to set controller reference: %w", err)
	}

	return pvc, nil
}

// buildObjectMeta creates the metadata for the PVC
func (pb *PVCBuilder) buildObjectMeta(workspace *workspacev1alpha1.Workspace) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      GeneratePVCName(workspace.Name),
		Namespace: workspace.Namespace,
		Labels:    GenerateLabels(workspace.Name),
	}
}

// buildPVCSpecWithSize creates the PVC specification with the given size and storage class
func (pb *PVCBuilder) buildPVCSpecWithSize(size resource.Quantity, storageClassName *string) corev1.PersistentVolumeClaimSpec {
	spec := corev1.PersistentVolumeClaimSpec{
		AccessModes: []corev1.PersistentVolumeAccessMode{
			corev1.ReadWriteOnce,
		},
		Resources: corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: size,
			},
		},
	}

	if storageClassName != nil {
		spec.StorageClassName = storageClassName
	}

	return spec
}

// NeedsUpdate checks if the existing PVC needs to be updated based on workspace changes
func (pb *PVCBuilder) NeedsUpdate(ctx context.Context, existingPVC *corev1.PersistentVolumeClaim, workspace *workspacev1alpha1.Workspace) (bool, error) {
	// Build the desired PVC spec
	desiredPVC, err := pb.BuildPVC(workspace)
	if err != nil {
		return false, fmt.Errorf("failed to build desired PVC: %w", err)
	}

	if desiredPVC == nil {
		// No storage requested, existing PVC should be deleted (handled elsewhere)
		return false, nil
	}

	// Compare only the fields we control (ignore Kubernetes-managed fields)

	// 1. Check AccessModes
	if !reflect.DeepEqual(existingPVC.Spec.AccessModes, desiredPVC.Spec.AccessModes) {
		return true, nil
	}

	// 2. Check Storage Size
	existingStorage := existingPVC.Spec.Resources.Requests[corev1.ResourceStorage]
	desiredStorage := desiredPVC.Spec.Resources.Requests[corev1.ResourceStorage]
	if !existingStorage.Equal(desiredStorage) {
		return true, nil
	}

	// 3. Check StorageClassName (handle nil cases)
	if !reflect.DeepEqual(existingPVC.Spec.StorageClassName, desiredPVC.Spec.StorageClassName) {
		return true, nil
	}

	return false, nil
}

// UpdatePVCSpec updates the existing PVC with the desired spec
func (pb *PVCBuilder) UpdatePVCSpec(ctx context.Context, existingPVC *corev1.PersistentVolumeClaim, workspace *workspacev1alpha1.Workspace) error {
	// Build the desired PVC spec
	desiredPVC, err := pb.BuildPVC(workspace)
	if err != nil {
		return fmt.Errorf("failed to build desired PVC: %w", err)
	}

	if desiredPVC == nil {
		return fmt.Errorf("cannot update PVC to nil spec")
	}

	// Update only the fields we control (preserve Kubernetes-managed fields)
	existingPVC.Spec.AccessModes = desiredPVC.Spec.AccessModes
	existingPVC.Spec.Resources = desiredPVC.Spec.Resources
	existingPVC.Spec.StorageClassName = desiredPVC.Spec.StorageClassName

	return nil
}
