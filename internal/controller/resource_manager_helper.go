/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// deploymentTimedOutReason is the reason the Deployment controller sets on Progressing=False once
// progressDeadlineSeconds passes without progress; k8s.io/api does not export the constant.
const deploymentTimedOutReason = "ProgressDeadlineExceeded"

// IsWorkspaceAvailable checks if the workspace is in Available=True state
func (rm *ResourceManager) IsWorkspaceAvailable(workspace *workspacev1alpha1.Workspace) bool {
	for _, condition := range workspace.Status.Conditions {
		if condition.Type == ConditionTypeAvailable {
			return condition.Status == metav1.ConditionTrue
		}
	}
	return false
}

// IsDeploymentAvailable checks if the Deployment is considered available
// based on its status conditions
func (rm *ResourceManager) IsDeploymentAvailable(deployment *appsv1.Deployment) bool {
	// If deployment is nil, it's not available
	if deployment == nil {
		return false
	}

	// Check if the deployment has the Available condition set to True
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentAvailable {
			return condition.Status == corev1.ConditionTrue
		}
	}

	// Fallback: also check the replica counts to determine availability
	// This is useful if the conditions aren't updated yet but replicas are running
	return deployment.Status.AvailableReplicas > 0 &&
		deployment.Status.ReadyReplicas >= *deployment.Spec.Replicas
}

// IsDeploymentProgressDeadlineExceeded reports whether the Deployment controller has declared the
// rollout stalled (Progressing=False with reason ProgressDeadlineExceeded) and returns that
// condition's message.
func (rm *ResourceManager) IsDeploymentProgressDeadlineExceeded(deployment *appsv1.Deployment) (bool, string) {
	if deployment == nil {
		return false, ""
	}
	for _, condition := range deployment.Status.Conditions {
		if condition.Type != appsv1.DeploymentProgressing {
			continue
		}
		if condition.Status == corev1.ConditionFalse && condition.Reason == deploymentTimedOutReason {
			return true, condition.Message
		}
		return false, ""
	}
	return false, ""
}

// WorkspacePodStallMessage returns what the workspace pod reports about why it is not running (see
// podStallMessage), or "" when no pod reports anything. Pods being deleted are skipped: under the
// Recreate strategy the previous pod can still be terminating while the next one is pending.
func (rm *ResourceManager) WorkspacePodStallMessage(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
) string {
	podList := &corev1.PodList{}
	if err := rm.client.List(ctx, podList,
		client.InNamespace(workspace.Namespace),
		client.MatchingLabels(GenerateLabels(workspace.Name)),
	); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to list workspace pods for the stall message")
		return ""
	}
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.DeletionTimestamp != nil {
			continue
		}
		if message := podStallMessage(pod); message != "" {
			return message
		}
	}
	return ""
}

// podStallMessage returns the scheduler's verdict while the pod is unscheduled (the PodScheduled=False
// message, e.g. "0/3 nodes are available: 3 Insufficient nvidia.com/gpu."), otherwise the reason and
// message of the first waiting container, init containers first (e.g. "ImagePullBackOff: Back-off
// pulling image ..."), otherwise "".
func podStallMessage(pod *corev1.Pod) string {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionFalse && condition.Message != "" {
			return condition.Message
		}
	}
	return waitingContainerMessage(pod)
}

// waitingContainerMessage returns the reason and message of the first waiting container, init
// containers first (e.g. "ImagePullBackOff: Back-off pulling image ..."), or "".
func waitingContainerMessage(pod *corev1.Pod) string {
	for _, statuses := range [][]corev1.ContainerStatus{pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses} {
		for _, containerStatus := range statuses {
			waiting := containerStatus.State.Waiting
			if waiting == nil || waiting.Reason == "" {
				continue
			}
			if waiting.Message == "" {
				return waiting.Reason
			}
			return waiting.Reason + ": " + waiting.Message
		}
	}
	return ""
}

// IsDeploymentMissingOrDeleting checks if the Deployment is either missing (nil)
// or in the process of being deleted
func (rm *ResourceManager) IsDeploymentMissingOrDeleting(deployment *appsv1.Deployment) bool {
	// If deployment is nil, it's missing
	if deployment == nil {
		return true
	}

	// Check if the deployment has a deletion timestamp (is being deleted)
	return !deployment.DeletionTimestamp.IsZero()
}

// IsServiceAvailable checks if the Service has acquired an IP address
func (rm *ResourceManager) IsServiceAvailable(service *corev1.Service) bool {
	// If service is nil, it's not available
	if service == nil {
		return false
	}

	if service.Spec.Type == corev1.ServiceTypeLoadBalancer {
		return len(service.Status.LoadBalancer.Ingress) > 0
	}

	// For any other type of service, assume it is available as soon as it's created
	return true
}

// IsServiceMissingOrDeleting checks if the Service is either missing (nil)
// or in the process of being deleted
func (rm *ResourceManager) IsServiceMissingOrDeleting(service *corev1.Service) bool {
	// If service is nil, it's missing
	if service == nil {
		return true
	}

	// Check if the service has a deletion timestamp (is being deleted)
	return !service.DeletionTimestamp.IsZero()
}

// IsPVCMissingOrDeleting checks if the PVC is either missing (nil)
// or in the process of being deleted
func (rm *ResourceManager) IsPVCMissingOrDeleting(pvc *corev1.PersistentVolumeClaim) bool {
	// If PVC is nil, it's missing
	if pvc == nil {
		return true
	}

	// Check if the PVC has a deletion timestamp (is being deleted)
	return !pvc.DeletionTimestamp.IsZero()
}
