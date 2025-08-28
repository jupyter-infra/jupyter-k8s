package controller

import (
	"context"
	"testing"

	"github.com/jupyter-k8s/jupyter-k8s/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func setupTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	return scheme
}

func createTestJupyterServer(name, namespace string) *v1alpha1.JupyterServer {
	return &v1alpha1.JupyterServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1alpha1.JupyterServerSpec{
			Name:          name,
			Image:         "jupyter/base-notebook:latest",
			DesiredStatus: "Running",
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
			},
		},
	}
}

func TestJupyterServerController_Reconcile_CreateResources(t *testing.T) {
	scheme := setupTestScheme()
	jupyterServer := createTestJupyterServer("test-jupyter", "default")
	
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(jupyterServer).
		WithStatusSubresource(&v1alpha1.JupyterServer{}).
		Build()

	// Create reconciler with dependencies
	deploymentBuilder := NewDeploymentBuilder(scheme)
	serviceBuilder := NewServiceBuilder(scheme)
	statusManager := NewStatusManager(fakeClient)
	resourceManager := NewResourceManager(fakeClient, deploymentBuilder, serviceBuilder, statusManager)
	stateMachine := NewStateMachine(resourceManager, statusManager)

	reconciler := &JupyterServerReconciler{
		Client:       fakeClient,
		Scheme:       scheme,
		stateMachine: stateMachine,
	}

	// Create reconcile request
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-jupyter",
			Namespace: "default",
		},
	}

	// Reconcile
	ctx := context.Background()
	result, err := reconciler.Reconcile(ctx, req)

	// Assertions
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	if result.Requeue {
		t.Error("Expected no requeue")
	}

	// Check if Deployment was created
	deployment := &appsv1.Deployment{}
	err = fakeClient.Get(ctx, types.NamespacedName{
		Name:      "jupyter-test-jupyter",
		Namespace: "default",
	}, deployment)
	if err != nil {
		t.Fatalf("Expected Deployment to be created: %v", err)
	}

	// Check if Service was created
	service := &corev1.Service{}
	err = fakeClient.Get(ctx, types.NamespacedName{
		Name:      "jupyter-test-jupyter-service",
		Namespace: "default",
	}, service)
	if err != nil {
		t.Fatalf("Expected Service to be created: %v", err)
	}

	// Verify deployment has correct image
	if len(deployment.Spec.Template.Spec.Containers) == 0 {
		t.Fatal("Expected at least one container in deployment")
	}
	
	container := deployment.Spec.Template.Spec.Containers[0]
	if container.Image != "jupyter/base-notebook:latest" {
		t.Errorf("Expected image 'jupyter/base-notebook:latest', got: %s", container.Image)
	}

	// Verify service has correct port
	if len(service.Spec.Ports) == 0 {
		t.Fatal("Expected at least one port in service")
	}
	
	if service.Spec.Ports[0].Port != 8888 {
		t.Errorf("Expected port 8888, got: %d", service.Spec.Ports[0].Port)
	}

	// Verify labels are correct
	expectedLabels := map[string]string{
		"app":                                    "jupyter",
		"jupyterserver.servers.jupyter.org/name": "test-jupyter",
	}
	for key, expectedValue := range expectedLabels {
		if actualValue, exists := deployment.Labels[key]; !exists || actualValue != expectedValue {
			t.Errorf("Expected deployment label %s=%s, got: %s", key, expectedValue, actualValue)
		}
		if actualValue, exists := service.Labels[key]; !exists || actualValue != expectedValue {
			t.Errorf("Expected service label %s=%s, got: %s", key, expectedValue, actualValue)
		}
	}
}

func TestJupyterServerController_Reconcile_StoppedState(t *testing.T) {
	scheme := setupTestScheme()
	jupyterServer := createTestJupyterServer("test-jupyter", "default")
	jupyterServer.Spec.DesiredStatus = "Stopped"

	// Create existing deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "jupyter-test-jupyter",
			Namespace: "default",
			Labels: map[string]string{
				"app":                                    "jupyter",
				"jupyterserver.servers.jupyter.org/name": "test-jupyter",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"jupyterserver.servers.jupyter.org/name": "test-jupyter",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"jupyterserver.servers.jupyter.org/name": "test-jupyter",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "jupyter",
							Image: "jupyter/base-notebook:latest",
						},
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(jupyterServer, deployment).
		WithStatusSubresource(&v1alpha1.JupyterServer{}).
		Build()

	// Create reconciler with dependencies
	deploymentBuilder := NewDeploymentBuilder(scheme)
	serviceBuilder := NewServiceBuilder(scheme)
	statusManager := NewStatusManager(fakeClient)
	resourceManager := NewResourceManager(fakeClient, deploymentBuilder, serviceBuilder, statusManager)
	stateMachine := NewStateMachine(resourceManager, statusManager)

	reconciler := &JupyterServerReconciler{
		Client:       fakeClient,
		Scheme:       scheme,
		stateMachine: stateMachine,
	}

	// Create reconcile request
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-jupyter",
			Namespace: "default",
		},
	}

	// Reconcile
	ctx := context.Background()
	result, err := reconciler.Reconcile(ctx, req)

	// Assertions
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	if result.Requeue {
		t.Error("Expected no requeue")
	}

	// Check if Deployment was deleted
	deploymentCheck := &appsv1.Deployment{}
	err = fakeClient.Get(ctx, types.NamespacedName{
		Name:      "jupyter-test-jupyter",
		Namespace: "default",
	}, deploymentCheck)
	if err == nil {
		t.Error("Expected Deployment to be deleted")
	}

	// Check if status was updated
	updatedJupyter := &v1alpha1.JupyterServer{}
	err = fakeClient.Get(ctx, types.NamespacedName{
		Name:      "test-jupyter",
		Namespace: "default",
	}, updatedJupyter)
	if err != nil {
		t.Fatalf("Failed to get updated JupyterServer: %v", err)
	}

	if updatedJupyter.Status.Phase != "Stopped" {
		t.Errorf("Expected status phase to be 'Stopped', got: %s", updatedJupyter.Status.Phase)
	}
}

func TestJupyterServerController_Reconcile_NotFound(t *testing.T) {
	scheme := setupTestScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	reconciler := &JupyterServerReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "non-existent",
			Namespace: "default",
		},
	}

	ctx := context.Background()
	result, err := reconciler.Reconcile(ctx, req)

	// Should not error when resource is not found
	if err != nil {
		t.Fatalf("Reconcile should not error when resource not found: %v", err)
	}

	if result.Requeue {
		t.Error("Expected no requeue when resource not found")
	}
}

func TestJupyterServerController_getJupyterServer(t *testing.T) {
	scheme := setupTestScheme()
	jupyterServer := createTestJupyterServer("test-jupyter", "default")
	
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(jupyterServer).
		Build()

	reconciler := &JupyterServerReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-jupyter",
			Namespace: "default",
		},
	}

	ctx := context.Background()
	result, err := reconciler.getJupyterServer(ctx, req)

	if err != nil {
		t.Fatalf("getJupyterServer failed: %v", err)
	}

	if result.Name != "test-jupyter" {
		t.Errorf("Expected name 'test-jupyter', got: %s", result.Name)
	}

	if result.Spec.Image != "jupyter/base-notebook:latest" {
		t.Errorf("Expected image 'jupyter/base-notebook:latest', got: %s", result.Spec.Image)
	}
}
