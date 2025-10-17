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
	"encoding/json"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	workspacesv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/controller"
)

const (
	accessTypeOwnerOnly = "OwnerOnly"
	accessTypePublic    = "Public"
)

// nolint:unused
// log is for logging in this package.
var workspacelog = logf.Log.WithName("workspace-resource")

func sanitizeUsername(username string) string {
	// Use Go's JSON marshaling to properly escape the string
	escaped, _ := json.Marshal(username)
	// Remove the surrounding quotes that json.Marshal adds
	return string(escaped[1 : len(escaped)-1])
}

// SetupWorkspaceWebhookWithManager registers the webhook for Workspace in the manager.
func SetupWorkspaceWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&workspacesv1alpha1.Workspace{}).
		WithValidator(&WorkspaceCustomValidator{}).
		WithDefaulter(&WorkspaceCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-workspaces-jupyter-org-v1alpha1-workspace,mutating=true,failurePolicy=fail,sideEffects=None,groups=workspaces.jupyter.org,resources=workspaces,verbs=create;update,versions=v1alpha1,name=mworkspace-v1alpha1.kb.io,admissionReviewVersions=v1,serviceName=jupyter-k8s-controller-manager,servicePort=9443

// WorkspaceCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Workspace when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type WorkspaceCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &WorkspaceCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Workspace.
func (d *WorkspaceCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	workspace, ok := obj.(*workspacesv1alpha1.Workspace)

	if !ok {
		return fmt.Errorf("expected an Workspace object but got %T", obj)
	}
	workspacelog.Info("Defaulting for Workspace", "name", workspace.GetName())

	// Add ownership tracking annotations
	if workspace.Annotations == nil {
		workspace.Annotations = make(map[string]string)
	}

	// Extract user info from request context
	if req, err := admission.RequestFromContext(ctx); err == nil {
		sanitizedUsername := sanitizeUsername(req.UserInfo.Username)

		// Only set created-by if it doesn't exist (CREATE operation)
		if _, exists := workspace.Annotations[controller.AnnotationCreatedBy]; !exists {
			workspace.Annotations[controller.AnnotationCreatedBy] = sanitizedUsername
			workspacelog.Info("Added created-by annotation", "workspace", workspace.GetName(), "user", sanitizedUsername)
		}

		// Always set last-updated-by (CREATE and UPDATE operations)
		workspace.Annotations[controller.AnnotationLastUpdatedBy] = sanitizedUsername
		workspacelog.Info("Added last-updated-by annotation", "workspace", workspace.GetName(), "user", sanitizedUsername)
	}

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-workspaces-jupyter-org-v1alpha1-workspace,mutating=false,failurePolicy=fail,sideEffects=None,groups=workspaces.jupyter.org,resources=workspaces,verbs=create;update,versions=v1alpha1,name=vworkspace-v1alpha1.kb.io,admissionReviewVersions=v1,serviceName=jupyter-k8s-controller-manager,servicePort=9443

// WorkspaceCustomValidator struct is responsible for validating the Workspace resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type WorkspaceCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &WorkspaceCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Workspace.
func (v *WorkspaceCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	workspace, ok := obj.(*workspacesv1alpha1.Workspace)
	if !ok {
		return nil, fmt.Errorf("expected a Workspace object but got %T", obj)
	}
	workspacelog.Info("Validation for Workspace upon creation", "name", workspace.GetName())

	// TODO(user): fill in your validation logic upon object creation.

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Workspace.
func (v *WorkspaceCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldWorkspace, ok := oldObj.(*workspacesv1alpha1.Workspace)
	if !ok {
		return nil, fmt.Errorf("expected a Workspace object for the oldObj but got %T", oldObj)
	}
	newWorkspace, ok := newObj.(*workspacesv1alpha1.Workspace)
	if !ok {
		return nil, fmt.Errorf("expected a Workspace object for the newObj but got %T", newObj)
	}

	// Validate that ownership annotations are immutable
	if oldWorkspace.Annotations != nil && newWorkspace.Annotations != nil {
		if oldCreatedBy := oldWorkspace.Annotations[controller.AnnotationCreatedBy]; oldCreatedBy != "" {
			if newCreatedBy := newWorkspace.Annotations[controller.AnnotationCreatedBy]; newCreatedBy != oldCreatedBy {
				return nil, fmt.Errorf("created-by annotation is immutable")
			}
		}
		if oldLastUpdatedBy := oldWorkspace.Annotations[controller.AnnotationLastUpdatedBy]; oldLastUpdatedBy != "" {
			if newLastUpdatedBy := newWorkspace.Annotations[controller.AnnotationLastUpdatedBy]; newLastUpdatedBy != oldLastUpdatedBy {
				return nil, fmt.Errorf("last-updated-by annotation is immutable")
			}
		}
	}

	// Check accessType transition rules first
	if oldWorkspace.Spec.AccessType == accessTypePublic && newWorkspace.Spec.AccessType == accessTypeOwnerOnly {
		return nil, fmt.Errorf("cannot change accessType from Public to OwnerOnly")
	}

	// For OwnerOnly workspaces, check if user has permission
	if newWorkspace.Spec.AccessType == accessTypeOwnerOnly {
		userHasPermission := false
		if req, err := admission.RequestFromContext(ctx); err == nil {
			// Check if user is cluster admin
			clusterAdminGroup := os.Getenv("CLUSTER_ADMIN_GROUP")
			for _, group := range req.UserInfo.Groups {
				if group == clusterAdminGroup {
					userHasPermission = true
					break
				}
			}

			// Check if user is the owner
			if !userHasPermission && oldWorkspace.Annotations != nil {
				if createdBy := oldWorkspace.Annotations[controller.AnnotationCreatedBy]; createdBy == sanitizeUsername(req.UserInfo.Username) {
					userHasPermission = true
				}
			}
		}
		if !userHasPermission {
			return nil, fmt.Errorf("access denied: only workspace owner or cluster admins can modify OwnerOnly workspaces")
		}
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Workspace.
func (v *WorkspaceCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	workspace, ok := obj.(*workspacesv1alpha1.Workspace)
	if !ok {
		return nil, fmt.Errorf("expected a Workspace object but got %T", obj)
	}

	// For OwnerOnly workspaces, check if user has permission
	if workspace.Spec.AccessType == accessTypeOwnerOnly {
		userHasPermission := false
		if req, err := admission.RequestFromContext(ctx); err == nil {
			// Check if user is cluster admin
			clusterAdminGroup := os.Getenv("CLUSTER_ADMIN_GROUP")
			for _, group := range req.UserInfo.Groups {
				if group == clusterAdminGroup {
					userHasPermission = true
					break
				}
			}

			// Check if user is the owner
			if !userHasPermission && workspace.Annotations != nil {
				if createdBy := workspace.Annotations[controller.AnnotationCreatedBy]; createdBy == sanitizeUsername(req.UserInfo.Username) {
					userHasPermission = true
				}
			}
		}
		if !userHasPermission {
			return nil, fmt.Errorf("access denied: only workspace owner or cluster admins can delete OwnerOnly workspaces")
		}
	}

	return nil, nil
}
