/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"context"
	"fmt"
	"net/http"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/controller"
	workspaceutil "github.com/jupyter-ai-contrib/jupyter-k8s/internal/workspace"
)

// +kubebuilder:webhook:path=/validate-pods-exec-workspace,mutating=false,failurePolicy=fail,sideEffects=None,groups="",resources=pods/exec,verbs=connect,versions=v1,name=vpods-exec-workspace-v1.kb.io,admissionReviewVersions=v1,serviceName=jupyter-k8s-controller-manager,servicePort=9443

var podexeclog = logf.Log.WithName("pod-exec-webhook")

// PodExecValidator validates pod exec requests to ensure controller service account
// can only exec into workspace pods
type PodExecValidator struct {
	client.Client
}

// Handle validates pod exec requests
func (v *PodExecValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	podexeclog.Info("Validating pod exec request",
		"pod", req.Name,
		"namespace", req.Namespace,
		"user", req.UserInfo.Username)

	// Build the expected controller service account username from environment variables
	controllerNamespace := os.Getenv(controller.ControllerPodNamespaceEnv)
	if controllerNamespace == "" {
		podexeclog.Error(nil, "Required environment variable not set",
			"envVar", controller.ControllerPodNamespaceEnv)
		return admission.Errored(http.StatusInternalServerError,
			fmt.Errorf("required environment variable %s not set", controller.ControllerPodNamespaceEnv))
	}

	controllerServiceAccount := os.Getenv(controller.ControllerPodServiceAccountEnv)
	if controllerServiceAccount == "" {
		podexeclog.Error(nil, "Required environment variable not set",
			"envVar", controller.ControllerPodServiceAccountEnv)
		return admission.Errored(http.StatusInternalServerError,
			fmt.Errorf("required environment variable %s not set", controller.ControllerPodServiceAccountEnv))
	}

	expectedUser := fmt.Sprintf("system:serviceaccount:%s:%s", controllerNamespace, controllerServiceAccount)

	// Check if request is from controller service account
	if req.UserInfo.Username != expectedUser {
		// Not controller SA - allow immediately without fetching pod
		podexeclog.Info("Allowing non-controller exec to any pod",
			"pod", req.Name,
			"namespace", req.Namespace,
			"user", req.UserInfo.Username)
		return admission.Allowed("exec request allowed")
	}

	// Controller SA - validate it can only exec into workspace pods
	// Get the target pod
	pod := &corev1.Pod{}
	if err := v.Get(ctx, types.NamespacedName{
		Name:      req.Name,
		Namespace: req.Namespace,
	}, pod); err != nil {
		podexeclog.Error(err, "Failed to get pod for exec validation")
		return admission.Errored(http.StatusInternalServerError,
			fmt.Errorf("failed to get pod: %w", err))
	}

	// Controller SA can only exec into workspace pods
	_, hasWorkspace := pod.Labels[workspaceutil.LabelWorkspaceName]
	if !hasWorkspace {
		podexeclog.Info("Denying controller exec to non-workspace pod",
			"user", req.UserInfo.Username,
			"pod", req.Name,
			"namespace", req.Namespace)
		return admission.Denied("controller service account can only exec into workspace pods")
	}

	podexeclog.Info("Allowing controller exec to workspace pod",
		"pod", req.Name,
		"workspace", pod.Labels[workspaceutil.LabelWorkspaceName])

	return admission.Allowed("exec request allowed")
}

// SetupPodExecWebhookWithManager registers the pod exec webhook with the manager
func SetupPodExecWebhookWithManager(mgr ctrl.Manager) error {
	podExecValidator := &PodExecValidator{
		Client: mgr.GetClient(),
	}

	mgr.GetWebhookServer().Register("/validate-pods-exec-workspace",
		&admission.Webhook{Handler: podExecValidator})
	return nil
}
