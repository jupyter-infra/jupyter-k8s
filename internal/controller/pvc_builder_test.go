package controller

import (
	"testing"

	workspacesv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestPVCBuilder_BuildPVC(t *testing.T) {
	// Register our types with the scheme
	s := runtime.NewScheme()
	_ = scheme.AddToScheme(s)
	_ = workspacesv1alpha1.AddToScheme(s)

	builder := NewPVCBuilder(s)

	t.Run("workspace with explicit storage should use workspace storage", func(t *testing.T) {
		workspace := &workspacesv1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workspace",
				Namespace: "default",
			},
			Spec: workspacesv1alpha1.WorkspaceSpec{
				Storage: &workspacesv1alpha1.StorageSpec{
					Size: resource.MustParse("5Gi"),
				},
			},
		}

		template := &ResolvedTemplate{
			StorageConfiguration: &workspacesv1alpha1.StorageConfig{
				DefaultSize: resource.MustParse("50Gi"),
			},
		}

		pvc, err := builder.BuildPVC(workspace, template)
		if err != nil {
			t.Fatalf("BuildPVC failed: %v", err)
		}

		if pvc == nil {
			t.Fatal("Expected PVC, got nil")
		}

		size := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
		if size.String() != "5Gi" {
			t.Errorf("Expected size 5Gi, got %s", size.String())
		}
	})

	t.Run("workspace without explicit storage should use template storage", func(t *testing.T) {
		workspace := &workspacesv1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workspace",
				Namespace: "default",
			},
			Spec: workspacesv1alpha1.WorkspaceSpec{
				// No Storage field
			},
		}

		template := &ResolvedTemplate{
			StorageConfiguration: &workspacesv1alpha1.StorageConfig{
				DefaultSize: resource.MustParse("50Gi"),
			},
		}

		pvc, err := builder.BuildPVC(workspace, template)
		if err != nil {
			t.Fatalf("BuildPVC failed: %v", err)
		}

		if pvc == nil {
			t.Fatal("Expected PVC, got nil")
		}

		size := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
		if size.String() != "50Gi" {
			t.Errorf("Expected size 50Gi (from template), got %s", size.String())
		}
	})

	t.Run("workspace without storage and no template should return nil", func(t *testing.T) {
		workspace := &workspacesv1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workspace",
				Namespace: "default",
			},
			Spec: workspacesv1alpha1.WorkspaceSpec{
				// No Storage field
			},
		}

		pvc, err := builder.BuildPVC(workspace, nil)
		if err != nil {
			t.Fatalf("BuildPVC failed: %v", err)
		}

		if pvc != nil {
			t.Fatal("Expected nil PVC (no storage), got PVC")
		}
	})

	t.Run("workspace without storage and template without storage should return nil", func(t *testing.T) {
		workspace := &workspacesv1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workspace",
				Namespace: "default",
			},
			Spec: workspacesv1alpha1.WorkspaceSpec{
				// No Storage field
			},
		}

		template := &ResolvedTemplate{
			// No StorageConfiguration
		}

		pvc, err := builder.BuildPVC(workspace, template)
		if err != nil {
			t.Fatalf("BuildPVC failed: %v", err)
		}

		if pvc != nil {
			t.Fatal("Expected nil PVC (no storage), got PVC")
		}
	})

	t.Run("workspace with zero size should default to 10Gi", func(t *testing.T) {
		workspace := &workspacesv1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workspace",
				Namespace: "default",
			},
			Spec: workspacesv1alpha1.WorkspaceSpec{
				Storage: &workspacesv1alpha1.StorageSpec{
					Size: resource.Quantity{}, // Zero value
				},
			},
		}

		pvc, err := builder.BuildPVC(workspace, nil)
		if err != nil {
			t.Fatalf("BuildPVC failed: %v", err)
		}

		if pvc == nil {
			t.Fatal("Expected PVC, got nil")
		}

		size := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
		if size.String() != "10Gi" {
			t.Errorf("Expected default size 10Gi, got %s", size.String())
		}
	})

	t.Run("workspace with storage class name should set storage class", func(t *testing.T) {
		storageClassName := "fast-ssd"
		workspace := &workspacesv1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workspace",
				Namespace: "default",
			},
			Spec: workspacesv1alpha1.WorkspaceSpec{
				Storage: &workspacesv1alpha1.StorageSpec{
					Size:             resource.MustParse("10Gi"),
					StorageClassName: &storageClassName,
				},
			},
		}

		pvc, err := builder.BuildPVC(workspace, nil)
		if err != nil {
			t.Fatalf("BuildPVC failed: %v", err)
		}

		if pvc == nil {
			t.Fatal("Expected PVC, got nil")
		}

		if pvc.Spec.StorageClassName == nil {
			t.Fatal("Expected storage class name, got nil")
		}

		if *pvc.Spec.StorageClassName != storageClassName {
			t.Errorf("Expected storage class %s, got %s", storageClassName, *pvc.Spec.StorageClassName)
		}
	})

	t.Run("PVC should have correct metadata", func(t *testing.T) {
		workspace := &workspacesv1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workspace",
				Namespace: "test-namespace",
			},
			Spec: workspacesv1alpha1.WorkspaceSpec{
				Storage: &workspacesv1alpha1.StorageSpec{
					Size: resource.MustParse("5Gi"),
				},
			},
		}

		pvc, err := builder.BuildPVC(workspace, nil)
		if err != nil {
			t.Fatalf("BuildPVC failed: %v", err)
		}

		if pvc == nil {
			t.Fatal("Expected PVC, got nil")
		}

		expectedName := GeneratePVCName("test-workspace")
		if pvc.Name != expectedName {
			t.Errorf("Expected PVC name %s, got %s", expectedName, pvc.Name)
		}

		if pvc.Namespace != "test-namespace" {
			t.Errorf("Expected namespace test-namespace, got %s", pvc.Namespace)
		}

		labels := GenerateLabels("test-workspace")
		for key, expectedValue := range labels {
			if actualValue, ok := pvc.Labels[key]; !ok {
				t.Errorf("Expected label %s, not found", key)
			} else if actualValue != expectedValue {
				t.Errorf("Expected label %s=%s, got %s=%s", key, expectedValue, key, actualValue)
			}
		}
	})

	t.Run("PVC should have owner reference", func(t *testing.T) {
		workspace := &workspacesv1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workspace",
				Namespace: "default",
				UID:       "test-uid",
			},
			Spec: workspacesv1alpha1.WorkspaceSpec{
				Storage: &workspacesv1alpha1.StorageSpec{
					Size: resource.MustParse("5Gi"),
				},
			},
		}

		pvc, err := builder.BuildPVC(workspace, nil)
		if err != nil {
			t.Fatalf("BuildPVC failed: %v", err)
		}

		if pvc == nil {
			t.Fatal("Expected PVC, got nil")
		}

		if len(pvc.OwnerReferences) == 0 {
			t.Fatal("Expected owner reference, got none")
		}

		ownerRef := pvc.OwnerReferences[0]
		if ownerRef.UID != workspace.UID {
			t.Errorf("Expected owner UID %s, got %s", workspace.UID, ownerRef.UID)
		}

		if ownerRef.Name != workspace.Name {
			t.Errorf("Expected owner name %s, got %s", workspace.Name, ownerRef.Name)
		}
	})
}
