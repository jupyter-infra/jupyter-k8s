/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package controller

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	awsutil "github.com/jupyter-infra/jupyter-k8s/internal/aws"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
)

// SSMRemoteAccessStrategyInterface defines the interface for SSM remote access operations
type SSMRemoteAccessStrategyInterface interface {
	SetupContainers(ctx context.Context, pod *corev1.Pod, workspace *workspacev1alpha1.Workspace, accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) error
	CleanupSSMManagedNodes(ctx context.Context, pod *corev1.Pod) error
}

// Variables for dependency injection in tests
var (
	newSSMRemoteAccessStrategy = func(podExecUtil awsutil.PodExecInterface) (*awsutil.SSMRemoteAccessStrategy, error) {
		return awsutil.NewSSMRemoteAccessStrategy(nil, podExecUtil)
	}
	newPodExecUtil = NewPodExecUtil
)

// PodEventHandler handles pod events for workspace pods
type PodEventHandler struct {
	client                  client.Client
	resourceManager         *ResourceManager
	ssmRemoteAccessStrategy SSMRemoteAccessStrategyInterface
}

// NewPodEventHandler creates a new PodEventHandler
func NewPodEventHandler(k8sClient client.Client, resourceManager *ResourceManager) *PodEventHandler {
	// Create PodExecUtil for SSM strategy
	podExecUtil, err := newPodExecUtil()
	if err != nil {
		logf.Log.Error(err, "Failed to initialize PodExecUtil - SSM features will be disabled")
		return &PodEventHandler{
			client:                  k8sClient,
			resourceManager:         resourceManager,
			ssmRemoteAccessStrategy: nil,
		}
	}

	ssmStrategy, err := newSSMRemoteAccessStrategy(podExecUtil)
	if err != nil {
		logf.Log.Error(err, "Failed to initialize SSM remote access strategy - SSM features will be disabled")
		ssmStrategy = nil
	}

	return &PodEventHandler{
		client:                  k8sClient,
		resourceManager:         resourceManager,
		ssmRemoteAccessStrategy: ssmStrategy,
	}
}

// HandleWorkspacePodEvents handles pod events for workspace pods
func (h *PodEventHandler) HandleWorkspacePodEvents(ctx context.Context, obj client.Object) []reconcile.Request {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return nil
	}

	logger := logf.FromContext(ctx).WithValues("pod", pod.Name, "namespace", pod.Namespace)
	logger.V(1).Info("Received pod event", "phase", pod.Status.Phase)

	// Get workspace name from labels (predicate ensures this exists)
	workspaceName := pod.Labels[workspaceutil.LabelWorkspaceName]

	logger.Info("Processing workspace pod event",
		"workspaceName", workspaceName,
		"phase", pod.Status.Phase,
		"containerCount", len(pod.Status.ContainerStatuses))

	// Handle deleted pods
	if pod.DeletionTimestamp != nil {
		h.handlePodDeleted(ctx, pod, workspaceName)
		return nil
	}

	// Log container statuses for debugging
	for _, containerStatus := range pod.Status.ContainerStatuses {
		logger.Info("Container status",
			"container", containerStatus.Name,
			"ready", containerStatus.Ready,
			"running", containerStatus.State.Running != nil)
	}

	// Handle specific pod events
	if pod.DeletionTimestamp == nil && pod.Status.Phase == corev1.PodRunning {
		h.handlePodRunning(ctx, pod, workspaceName)
	}

	// Don't trigger workspace reconciliation (prevents race conditions)
	logger.V(1).Info("Pod event processed, not triggering workspace reconciliation")
	return nil
}

// HandleKubernetesEvents processes Kubernetes events for preemption detection
func (h *PodEventHandler) HandleKubernetesEvents(ctx context.Context, obj client.Object) []reconcile.Request {
	event, ok := obj.(*corev1.Event)
	if !ok {
		return nil
	}

	logger := logf.FromContext(ctx).WithValues("event", event.Name, "pod", event.InvolvedObject.Name)

	// Check if this is a preemption event
	if event.InvolvedObject.Kind == KindPod &&
		event.Reason == DesiredStateStopped &&
		strings.Contains(event.Message, "Preempted") {

		logger.Info("Detected pod preemption event",
			"pod", event.InvolvedObject.Name,
			"namespace", event.InvolvedObject.Namespace,
			"message", event.Message)

		// Extract workspace name from pod name
		podName := event.InvolvedObject.Name
		if !strings.HasPrefix(podName, "jupyter-") {
			return nil
		}

		parts := strings.Split(podName, "-")
		if len(parts) < 4 {
			return nil
		}

		// Remove "jupyter-" prefix and last two hash suffixes
		workspaceName := strings.Join(parts[1:len(parts)-2], "-")

		logger.Info("Pod was preempted, updating workspace desiredStatus to Stopped",
			"workspace", workspaceName)
		h.updateWorkspaceDesiredStatus(ctx, workspaceName, event.InvolvedObject.Namespace, DesiredStateStopped)

		// Return reconciliation request to trigger workspace reconciliation
		return []reconcile.Request{
			{
				NamespacedName: client.ObjectKey{
					Name:      workspaceName,
					Namespace: event.InvolvedObject.Namespace,
				},
			},
		}
	}

	return nil
}

// handlePodRunning handles when a workspace pod enters running state
func (h *PodEventHandler) handlePodRunning(ctx context.Context, pod *corev1.Pod, workspaceName string) {
	logger := logf.FromContext(ctx).WithValues("pod", pod.Name, "workspace", workspaceName)
	logger.Info("Workspace pod is now running")

	// Get the workspace
	workspace := &workspacev1alpha1.Workspace{}
	err := h.client.Get(ctx, client.ObjectKey{
		Name:      workspaceName,
		Namespace: pod.Namespace,
	}, workspace)
	if err != nil {
		logger.V(1).Info("Workspace already deleted, skipping pod event processing - this is expected during workspace cleanup")
		return
	}

	// Get access strategy using resource manager
	accessStrategy, err := h.resourceManager.GetAccessStrategyForWorkspace(ctx, workspace)
	if err != nil {
		logger.Error(err, "Failed to get access strategy for workspace")
		return
	}

	// Handle AWS pod events (SSM remote access strategy)
	if accessStrategy != nil && accessStrategy.Spec.PodEventsHandler == "aws" {
		if h.ssmRemoteAccessStrategy == nil {
			logger.Error(nil, "SSM remote access strategy not available - cannot setup containers")
		} else {
			if err := h.ssmRemoteAccessStrategy.SetupContainers(ctx, pod, workspace, accessStrategy); err != nil {
				logger.Error(err, "Failed to setup containers")
			}
		}
	}
}

// handlePodDeleted handles when a workspace pod is deleted
func (h *PodEventHandler) handlePodDeleted(ctx context.Context, pod *corev1.Pod, workspaceName string) {
	logger := logf.FromContext(ctx).WithValues("pod", pod.Name, "workspace", workspaceName)
	logger.Info("Workspace pod has been deleted", "podUID", pod.UID)

	// Check if this pod uses SSM remote access strategy by checking labels
	// AccessStrategy labels are set by workspace reconciler and propagated: Workspace.labels -> Deployment.labels -> Pod.labels
	accessStrategyName := pod.Labels[LabelAccessStrategyName]
	if accessStrategyName == "" {
		logger.V(1).Info("Pod has no access strategy label, skipping resource cleanup")
		return
	}

	accessStrategyNamespace := pod.Labels[LabelAccessStrategyNamespace]
	if accessStrategyNamespace == "" {
		logger.V(1).Info("Pod has no access strategy namespace label, skipping resource cleanup")
		return
	}

	// Fetch the access strategy
	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{}
	err := h.client.Get(ctx, client.ObjectKey{
		Name:      accessStrategyName,
		Namespace: accessStrategyNamespace,
	}, accessStrategy)
	if err != nil {
		logger.Error(err, "Failed to get access strategy, skipping resource cleanup")
		return
	}

	// Handle AWS pod events (SSM remote access strategy)
	if accessStrategy != nil && accessStrategy.Spec.PodEventsHandler == "aws" {
		if h.ssmRemoteAccessStrategy == nil {
			logger.Error(nil, "SSM remote access strategy not available - cannot cleanup SSM managed nodes")
		} else {
			if err := h.ssmRemoteAccessStrategy.CleanupSSMManagedNodes(ctx, pod); err != nil {
				logger.Error(err, "Failed to cleanup SSM managed nodes")
			}
		}
	} else {
		logger.V(1).Info("Pod does not require resource cleanup, skipping",
			"accessStrategy", accessStrategyName)
	}
}

// updateWorkspaceDesiredStatus updates the workspace desiredStatus
func (h *PodEventHandler) updateWorkspaceDesiredStatus(ctx context.Context, workspaceName string, namespace string, desiredStatus string) {
	logger := logf.FromContext(ctx).WithValues("workspace", workspaceName, "namespace", namespace)

	workspace := &workspacev1alpha1.Workspace{}
	err := h.client.Get(ctx, client.ObjectKey{
		Name:      workspaceName,
		Namespace: namespace,
	}, workspace)
	if err != nil {
		logger.Error(err, "Failed to get workspace for status update")
		return
	}

	// Add annotation to track preemption reason
	if desiredStatus == DesiredStateStopped {
		if workspace.Annotations == nil {
			workspace.Annotations = make(map[string]string)
		}
		workspace.Annotations[PreemptionReasonAnnotation] = PreemptedReason
	}

	if workspace.Spec.DesiredStatus != desiredStatus {
		workspace.Spec.DesiredStatus = desiredStatus
	}

	if err := h.client.Update(ctx, workspace); err != nil {
		logger.Error(err, "Failed to update workspace")
	} else {
		logger.Info("Successfully updated workspace due to preemption", "desiredStatus", desiredStatus)
	}
}
