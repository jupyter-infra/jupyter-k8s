/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/jupyter-infra/jupyter-k8s-plugin/plugin"
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/awsadapter"
	"github.com/jupyter-infra/jupyter-k8s/internal/pluginadapters"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
)

// PodEventHandler handles pod events for workspace pods
type PodEventHandler struct {
	client          client.Client
	resourceManager *ResourceManager
	// podEventAdapters maps plugin names (e.g. "aws") to their pod event adapter implementation.
	podEventAdapters map[string]pluginadapters.PodEventPluginAdapter
}

// NewPodEventHandler creates a new PodEventHandler.
// pluginClients maps plugin names to their remote access client implementations.
// An empty or nil map disables all plugin-based remote access features.
func NewPodEventHandler(k8sClient client.Client, resourceManager *ResourceManager, pluginClients map[string]plugin.RemoteAccessPluginApis) *PodEventHandler {
	if len(pluginClients) == 0 {
		logf.Log.Info("No plugin clients provided - remote access features will be disabled")
		return &PodEventHandler{
			client:           k8sClient,
			resourceManager:  resourceManager,
			podEventAdapters: nil,
		}
	}

	// Create PodExecUtil (shared across all adapters)
	podExecUtil, err := NewPodExecUtil()
	if err != nil {
		logf.Log.Error(err, "Failed to initialize PodExecUtil - remote access features will be disabled")
		return &PodEventHandler{
			client:           k8sClient,
			resourceManager:  resourceManager,
			podEventAdapters: nil,
		}
	}

	podEventAdapters := map[string]pluginadapters.PodEventPluginAdapter{}
	for name, pluginClient := range pluginClients {
		switch name {
		case "aws":
			adapter, err := awsadapter.NewAwsSsmPodEventAdapter(pluginClient, podExecUtil)
			if err != nil {
				logf.Log.Error(err, "Failed to initialize AWS SSM pod event adapter", "plugin", name)
				continue
			}
			podEventAdapters[name] = adapter
		default:
			logf.Log.Info("No pod event adapter mapped for plugin - skipping", "plugin", name)
		}
	}

	return &PodEventHandler{
		client:           k8sClient,
		resourceManager:  resourceManager,
		podEventAdapters: podEventAdapters,
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

	// Dispatch to the appropriate pod event handler by plugin name
	if accessStrategy != nil && accessStrategy.Spec.PodEventsHandler != "" {
		pluginName, _ := plugin.ParseHandlerRef(accessStrategy.Spec.PodEventsHandler)
		adapter, ok := h.podEventAdapters[pluginName]
		if !ok || adapter == nil {
			logger.Error(nil, "Pod event adapter not available - cannot setup containers", "plugin", pluginName)
		} else {
			// Resolve dynamic values in pod events context
			resolvedCtx, err := pluginadapters.ResolvePodContext(accessStrategy.Spec.PodEventsContext, pod)
			if err != nil {
				logger.Error(err, "Failed to resolve pod events context", "plugin", pluginName)
			} else if err := adapter.HandlePodRunning(ctx, pod, workspaceName, pod.Namespace, resolvedCtx); err != nil {
				logger.Error(err, "Failed to setup containers", "plugin", pluginName)
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

	// Dispatch to the appropriate pod event adapter by plugin name
	if accessStrategy.Spec.PodEventsHandler != "" {
		pluginName, _ := plugin.ParseHandlerRef(accessStrategy.Spec.PodEventsHandler)
		adapter, ok := h.podEventAdapters[pluginName]
		if !ok || adapter == nil {
			logger.Error(nil, "Pod event adapter not available - cannot cleanup managed nodes", "plugin", pluginName)
		} else {
			// Resolve dynamic values in pod events context
			resolvedCtx, err := pluginadapters.ResolvePodContext(accessStrategy.Spec.PodEventsContext, pod)
			if err != nil {
				logger.Error(err, "Failed to resolve pod events context", "plugin", pluginName)
			} else if err := adapter.HandlePodDeleted(ctx, pod, resolvedCtx); err != nil {
				logger.Error(err, "Failed to cleanup managed nodes", "plugin", pluginName)
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
