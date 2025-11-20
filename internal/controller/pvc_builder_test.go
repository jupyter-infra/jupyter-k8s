package controller

import (
	"context"
	"testing"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
)

func setupPVCBuilder() *PVCBuilder {
	s := runtime.NewScheme()
	_ = scheme.AddToScheme(s)
	_ = workspacev1alpha1.AddToScheme(s)
	return NewPVCBuilder(s)
}

func TestPVCBuilder_ExplicitStorage(t *testing.T) {
	builder := setupPVCBuilder()
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "test-workspace", Namespace: "default"},
		Spec: workspacev1alpha1.WorkspaceSpec{
			Storage: &workspacev1alpha1.StorageSpec{Size: resource.MustParse("5Gi")},
		},
	}

	pvc, err := builder.BuildPVC(workspace)
	if err != nil {
		t.Fatalf("BuildPVC failed: %v", err)
	}
	if pvc == nil {
		t.Fatal("Expected PVC, got nil")
		return
	}
	size := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	if size.String() != "5Gi" {
		t.Errorf("Expected size 5Gi, got %s", size.String())
	}
}

func TestPVCBuilder_TemplateStorage(t *testing.T) {
	// Note: Template defaults are now applied via webhooks during admission
	// This test verifies workspace storage spec is respected
	builder := setupPVCBuilder()
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "test-workspace", Namespace: "default"},
		Spec: workspacev1alpha1.WorkspaceSpec{
			Storage: &workspacev1alpha1.StorageSpec{Size: resource.MustParse("50Gi")},
		},
	}

	pvc, err := builder.BuildPVC(workspace)
	if err != nil {
		t.Fatalf("BuildPVC failed: %v", err)
	}
	if pvc == nil {
		t.Fatal("Expected PVC, got nil")
		return
	}
	size := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	if size.String() != "50Gi" {
		t.Errorf("Expected size 50Gi, got %s", size.String())
	}
}

func TestPVCBuilder_NoStorage(t *testing.T) {
	builder := setupPVCBuilder()
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "test-workspace", Namespace: "default"},
		Spec:       workspacev1alpha1.WorkspaceSpec{},
	}

	pvc, err := builder.BuildPVC(workspace)
	if err != nil {
		t.Fatalf("BuildPVC failed: %v", err)
	}
	if pvc != nil {
		t.Fatal("Expected nil PVC, got PVC")
	}
}

func TestPVCBuilder_DefaultSize(t *testing.T) {
	builder := setupPVCBuilder()
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "test-workspace", Namespace: "default"},
		Spec: workspacev1alpha1.WorkspaceSpec{
			Storage: &workspacev1alpha1.StorageSpec{Size: resource.Quantity{}},
		},
	}

	pvc, err := builder.BuildPVC(workspace)
	if err != nil {
		t.Fatalf("BuildPVC failed: %v", err)
	}
	if pvc == nil {
		t.Fatal("Expected PVC, got nil")
		return
	}
	size := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	if size.String() != "10Gi" {
		t.Errorf("Expected default size 10Gi, got %s", size.String())
	}
}

func TestPVCBuilder_StorageClass(t *testing.T) {
	builder := setupPVCBuilder()
	storageClassName := "fast-ssd"
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "test-workspace", Namespace: "default"},
		Spec: workspacev1alpha1.WorkspaceSpec{
			Storage: &workspacev1alpha1.StorageSpec{
				Size:             resource.MustParse("10Gi"),
				StorageClassName: &storageClassName,
			},
		},
	}

	pvc, err := builder.BuildPVC(workspace)
	if err != nil {
		t.Fatalf("BuildPVC failed: %v", err)
	}
	if pvc == nil {
		t.Fatal("Expected PVC, got nil")
		return
	}
	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != storageClassName {
		t.Errorf("Expected storage class %s, got %v", storageClassName, pvc.Spec.StorageClassName)
	}
}

func TestPVCBuilder_Metadata(t *testing.T) {
	builder := setupPVCBuilder()
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "test-workspace", Namespace: "test-namespace"},
		Spec: workspacev1alpha1.WorkspaceSpec{
			Storage: &workspacev1alpha1.StorageSpec{Size: resource.MustParse("5Gi")},
		},
	}

	pvc, err := builder.BuildPVC(workspace)
	if err != nil {
		t.Fatalf("BuildPVC failed: %v", err)
	}
	if pvc == nil {
		t.Fatal("Expected PVC, got nil")
		return
	}

	expectedName := GeneratePVCName("test-workspace")
	if pvc.Name != expectedName {
		t.Errorf("Expected PVC name %s, got %s", expectedName, pvc.Name)
	}
	if pvc.Namespace != "test-namespace" {
		t.Errorf("Expected namespace test-namespace, got %s", pvc.Namespace)
	}
}

func TestPVCBuilder_OwnerReference(t *testing.T) {
	builder := setupPVCBuilder()
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "test-workspace", Namespace: "default", UID: "test-uid"},
		Spec: workspacev1alpha1.WorkspaceSpec{
			Storage: &workspacev1alpha1.StorageSpec{Size: resource.MustParse("5Gi")},
		},
	}

	pvc, err := builder.BuildPVC(workspace)
	if err != nil {
		t.Fatalf("BuildPVC failed: %v", err)
	}
	if pvc == nil {
		t.Fatal("Expected PVC, got nil")
		return
	}
	if len(pvc.OwnerReferences) == 0 {
		t.Fatal("Expected owner reference, got none")
	}

	ownerRef := pvc.OwnerReferences[0]
	if ownerRef.UID != workspace.UID || ownerRef.Name != workspace.Name {
		t.Errorf("Expected owner %s/%s, got %s/%s", workspace.Name, workspace.UID, ownerRef.Name, ownerRef.UID)
	}
}

func TestPVCBuilder_UpdateDetection(t *testing.T) {
	ctx := context.Background()
	s := runtime.NewScheme()
	if err := workspacev1alpha1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	builder := NewPVCBuilder(s)

	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "test-workspace", Namespace: "default"},
		Spec: workspacev1alpha1.WorkspaceSpec{
			Storage: &workspacev1alpha1.StorageSpec{Size: resource.MustParse("10Gi")},
		},
	}

	existingPVC, err := builder.BuildPVC(workspace)
	if err != nil {
		t.Fatal(err)
	}

	needsUpdate, err := builder.NeedsUpdate(ctx, existingPVC, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if needsUpdate {
		t.Error("Expected no update needed")
	}

	workspace.Spec.Storage.Size = resource.MustParse("20Gi")
	needsUpdate, err = builder.NeedsUpdate(ctx, existingPVC, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if !needsUpdate {
		t.Error("Expected update needed")
	}
}
