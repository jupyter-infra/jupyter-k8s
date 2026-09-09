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
)

// deploymentTimedOutReason is the reason the deployment controller sets on Progressing=False when
// progressDeadlineSeconds is exceeded; k8s.io/api does not export the constant.
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

// IsDeploymentProgressDeadlineExceeded reports whether the deployment controller has declared the
// rollout stalled (Progressing=False with reason ProgressDeadlineExceeded), and returns the
// condition's message.
func (rm *ResourceManager) IsDeploymentProgressDeadlineExceeded(deployment *appsv1.Deployment) (bool, string) {
	if deployment == nil {
		return false, ""
	}
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentProgressing {
			if condition.Status == corev1.ConditionFalse && condition.Reason == deploymentTimedOutReason {
				return true, condition.Message
			}
			return false, ""
		}
	}
	return false, ""
}

// WorkspacePodSchedulingMessage returns the PodScheduled=False condition message of a workspace
// pod (the scheduler's verdict, e.g. "0/1 nodes are available: 1 Insufficient nvidia.com/gpu."),
// or "" when no pod carries one.
func (rm *ResourceManager) WorkspacePodSchedulingMessage(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
) string {
	podList := &corev1.PodList{}
	if err := rm.client.List(ctx, podList,
		client.InNamespace(workspace.Namespace),
		client.MatchingLabels(GenerateLabels(workspace.Name)),
	); err != nil {
		return ""
	}
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.DeletionTimestamp != nil {
			continue
		}
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodScheduled &&
				condition.Status == corev1.ConditionFalse && condition.Message != "" {
				return condition.Message
			}
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
