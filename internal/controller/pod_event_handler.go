package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	awsutil "github.com/jupyter-ai-contrib/jupyter-k8s/internal/aws"
)

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
	ssmRemoteAccessStrategy *awsutil.SSMRemoteAccessStrategy
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
	workspaceName := pod.Labels[LabelWorkspaceName]

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

	// Handle SSM remote access strategy
	if accessStrategy != nil && accessStrategy.Name == "aws-ssm-remote-access" {
		if h.ssmRemoteAccessStrategy == nil {
			logger.Error(nil, "SSM remote access strategy not available - cannot initialize SSM agent")
		} else {
			if err := h.ssmRemoteAccessStrategy.InitSSMAgent(ctx, pod, workspace, accessStrategy); err != nil {
				logger.Error(err, "Failed to initialize SSM agent")
			}
		}
	}
}

// handlePodDeleted handles when a workspace pod is deleted
func (h *PodEventHandler) handlePodDeleted(ctx context.Context, pod *corev1.Pod, workspaceName string) {
	logger := logf.FromContext(ctx).WithValues("pod", pod.Name, "workspace", workspaceName)
	logger.Info("Workspace pod has been deleted", "podUID", pod.UID)

	// Clean up SSM managed nodes for this pod
	if h.ssmRemoteAccessStrategy == nil {
		logger.Error(nil, "SSM remote access strategy not available - cannot cleanup SSM managed nodes")
	} else {
		if err := h.ssmRemoteAccessStrategy.CleanupSSMManagedNodes(ctx, pod); err != nil {
			logger.Error(err, "Failed to cleanup SSM managed nodes")
		}
	}
}
