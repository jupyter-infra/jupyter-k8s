package controller

import (
	"fmt"

	workspacesv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"

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

// BuildPVC creates a PersistentVolumeClaim resource for the given Workspace
// It uses workspace storage if specified, otherwise falls back to template storage configuration
func (pb *PVCBuilder) BuildPVC(workspace *workspacesv1alpha1.Workspace, resolvedTemplate *ResolvedTemplate) (*corev1.PersistentVolumeClaim, error) {
	// Determine storage configuration from workspace or template
	var size resource.Quantity
	var storageClassName *string

	if workspace.Spec.Storage != nil {
		// Workspace storage takes precedence
		size = workspace.Spec.Storage.Size
		storageClassName = workspace.Spec.Storage.StorageClassName
	} else if resolvedTemplate != nil && resolvedTemplate.StorageConfiguration != nil {
		// Fall back to template storage configuration
		size = resolvedTemplate.StorageConfiguration.DefaultSize
		// Use default storage class from cluster (storageClassName remains nil)
	} else {
		// No storage requested from either source
		return nil, nil
	}

	// Ensure size is set to a default if zero
	if size.IsZero() {
		size = resource.MustParse("10Gi")
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: pb.buildObjectMeta(workspace),
		Spec:       pb.buildPVCSpecWithSize(size, storageClassName),
	}

	// Set owner reference for garbage collection
	if err := controllerutil.SetControllerReference(workspace, pvc, pb.scheme); err != nil {
		return nil, fmt.Errorf("failed to set controller reference: %w", err)
	}

	return pvc, nil
}

// buildObjectMeta creates the metadata for the PVC
func (pb *PVCBuilder) buildObjectMeta(workspace *workspacesv1alpha1.Workspace) metav1.ObjectMeta {
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
