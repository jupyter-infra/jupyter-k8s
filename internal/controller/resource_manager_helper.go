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
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
