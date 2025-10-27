package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

// WorkspaceIdleChecker provides utilities for checking workspace idle status
type WorkspaceIdleChecker struct {
	client client.Client
}

// NewWorkspaceIdleChecker creates a new WorkspaceIdleChecker instance
func NewWorkspaceIdleChecker(k8sClient client.Client) *WorkspaceIdleChecker {
	return &WorkspaceIdleChecker{
		client: k8sClient,
	}
}

// CheckWorkspaceIdle checks if a workspace is idle using the configured detection method
func (w *WorkspaceIdleChecker) CheckWorkspaceIdle(ctx context.Context, workspace *workspacev1alpha1.Workspace, idleConfig *workspacev1alpha1.IdleShutdownSpec) (bool, bool, error) {
	logger := logf.FromContext(ctx).WithValues("workspace", workspace.Name, "namespace", workspace.Namespace)

	// Find the workspace pod
	pod, err := w.findWorkspacePod(ctx, workspace)
	if err != nil {
		logger.Error(err, "Failed to find workspace pod")
		return false, true, fmt.Errorf("failed to find workspace pod: %w", err)
	}

	logger.V(1).Info("Found workspace pod", "pod", pod.Name)

	// Create appropriate detector
	detector, err := CreateIdleDetector(&idleConfig.Detection)
	if err != nil {
		logger.Error(err, "Failed to create idle detector")
		return false, false, fmt.Errorf("failed to create idle detector: %w", err)
	}

	// Use detector to check idle status
	return detector.CheckIdle(ctx, workspace.Name, pod, idleConfig)
}

// findWorkspacePod finds the pod for a workspace
func (w *WorkspaceIdleChecker) findWorkspacePod(ctx context.Context, workspace *workspacev1alpha1.Workspace) (*corev1.Pod, error) {
	logger := logf.FromContext(ctx).WithValues("workspace", workspace.Name)

	// List pods with the workspace labels
	podList := &corev1.PodList{}
	labels := GenerateLabels(workspace.Name)

	if err := w.client.List(ctx, podList, client.InNamespace(workspace.Namespace), client.MatchingLabels(labels)); err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	// Find a running pod
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			logger.V(1).Info("Found running workspace pod", "pod", pod.Name)
			return &pod, nil
		}
	}

	return nil, fmt.Errorf("no running pod found for workspace")
}
