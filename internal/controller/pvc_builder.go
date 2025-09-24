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
func (pb *PVCBuilder) BuildPVC(workspace *workspacesv1alpha1.Workspace) (*corev1.PersistentVolumeClaim, error) {
	if workspace.Spec.Storage == nil {
		return nil, nil // No storage requested
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: pb.buildObjectMeta(workspace),
		Spec:       pb.buildPVCSpec(workspace.Spec.Storage),
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

// buildPVCSpec creates the PVC specification
func (pb *PVCBuilder) buildPVCSpec(storage *workspacesv1alpha1.StorageSpec) corev1.PersistentVolumeClaimSpec {
	size := storage.Size
	if size.IsZero() {
		size = resource.MustParse("10Gi") // Default size
	}

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

	if storage.StorageClassName != nil {
		spec.StorageClassName = storage.StorageClassName
	}

	return spec
}
