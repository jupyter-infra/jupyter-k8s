/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// IdleCheckResult represents the result of an idle check operation
type IdleCheckResult struct {
	// IsIdle indicates whether the workspace is idle and should be shut down
	IsIdle bool

	// ShouldRetry indicates whether a failed check should be retried
	// true = temporary failure, retry later
	// false = permanent failure, stop checking
	ShouldRetry bool
}

// WorkspaceIdleChecker provides utilities for checking workspace idle status
type WorkspaceIdleChecker struct {
	client        client.Client
	checkInterval time.Duration

	// httpClient is shared across all network idle checks so TCP (and TLS)
	// connections to frequently-probed services are reused between cycles
	// instead of re-handshaked. http.Client is safe for concurrent use; the
	// per-request timeout is applied via context in the detector.
	httpClient *http.Client
}

// NewWorkspaceIdleChecker creates a new WorkspaceIdleChecker instance.
// If checkInterval is zero or negative, DefaultIdleCheckInterval is used.
// Positive values below MinIdleCheckInterval are clamped up to that floor.
func NewWorkspaceIdleChecker(k8sClient client.Client, checkInterval time.Duration) *WorkspaceIdleChecker {
	switch {
	case checkInterval <= 0:
		checkInterval = DefaultIdleCheckInterval
	case checkInterval < MinIdleCheckInterval:
		checkInterval = MinIdleCheckInterval
	}
	return &WorkspaceIdleChecker{
		client:        k8sClient,
		checkInterval: checkInterval,
		httpClient: &http.Client{
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// CheckInterval returns the configured interval between idle checks.
func (w *WorkspaceIdleChecker) CheckInterval() time.Duration {
	return w.checkInterval
}

// CheckWorkspaceIdle checks if a workspace is idle using the configured detection method.
// For transport:network, it uses the service ClusterIP (already available from the reconcile).
// For transport:podExec, it finds a running pod and execs curl into it.
func (w *WorkspaceIdleChecker) CheckWorkspaceIdle(ctx context.Context, workspace *workspacev1alpha1.Workspace, service *corev1.Service, idleConfig *workspacev1alpha1.IdleShutdownSpec) (*IdleCheckResult, error) {
	transport := transportPodExec
	if idleConfig.Detection.HTTPGet != nil && idleConfig.Detection.HTTPGet.Transport != "" {
		transport = idleConfig.Detection.HTTPGet.Transport
	}

	if transport == transportNetwork {
		return w.checkViaNetwork(ctx, workspace, service, idleConfig)
	}
	return w.checkViaPodExec(ctx, workspace, idleConfig)
}

func (w *WorkspaceIdleChecker) checkViaNetwork(ctx context.Context, workspace *workspacev1alpha1.Workspace, service *corev1.Service, idleConfig *workspacev1alpha1.IdleShutdownSpec) (*IdleCheckResult, error) {
	logger := logf.FromContext(ctx).WithValues("workspace", workspace.Name)

	if service == nil {
		logger.Error(nil, "No service available for network idle check")
		return &IdleCheckResult{IsIdle: false, ShouldRetry: true}, fmt.Errorf("no service available for workspace")
	}

	host := service.Spec.ClusterIP
	if host == "" || host == "None" {
		return &IdleCheckResult{IsIdle: false, ShouldRetry: true}, fmt.Errorf("service has no ClusterIP")
	}

	detector, err := CreateIdleDetector(&idleConfig.Detection, w.httpClient)
	if err != nil {
		logger.Error(err, "Failed to create idle detector")
		return &IdleCheckResult{IsIdle: false, ShouldRetry: false}, fmt.Errorf("failed to create idle detector: %w", err)
	}

	return detector.CheckIdle(ctx, workspace, host, idleConfig)
}

func (w *WorkspaceIdleChecker) checkViaPodExec(ctx context.Context, workspace *workspacev1alpha1.Workspace, idleConfig *workspacev1alpha1.IdleShutdownSpec) (*IdleCheckResult, error) {
	logger := logf.FromContext(ctx).WithValues("workspace", workspace.Name)

	pod, err := w.findWorkspacePod(ctx, workspace)
	if err != nil {
		logger.Error(err, "Failed to find workspace pod")
		return &IdleCheckResult{IsIdle: false, ShouldRetry: true}, fmt.Errorf("failed to find workspace pod: %w", err)
	}

	detector := NewPodExecHTTPGetDetectorForPod(pod)
	return detector.CheckIdle(ctx, workspace, "localhost", idleConfig)
}

// findWorkspacePod finds the pod for a workspace
func (w *WorkspaceIdleChecker) findWorkspacePod(ctx context.Context, workspace *workspacev1alpha1.Workspace) (*corev1.Pod, error) {
	logger := logf.FromContext(ctx).WithValues("workspace", workspace.Name)

	podList := &corev1.PodList{}
	labels := GenerateLabels(workspace.Name)

	if err := w.client.List(ctx, podList, client.InNamespace(workspace.Namespace), client.MatchingLabels(labels)); err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			logger.V(1).Info("Found running workspace pod", "pod", pod.Name)
			return &pod, nil
		}
	}

	return nil, fmt.Errorf("no running pod found for workspace")
}
