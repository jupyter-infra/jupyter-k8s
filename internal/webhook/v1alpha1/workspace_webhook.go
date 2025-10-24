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

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/controller"
	webhookconst "github.com/jupyter-ai-contrib/jupyter-k8s/internal/webhook"
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

// getEffectiveOwnershipType returns the effective access type, treating empty as Public
// TODO: think of better way to convey defaults to user.
func getEffectiveOwnershipType(ownershipType string) string {
	if ownershipType == "" {
		return webhookconst.OwnershipTypePublic
	}
	return ownershipType
}

// isAdminUser checks if the user groups include any admin groups
func isAdminUser(groups []string) bool {
	adminGroups := []string{webhookconst.DefaultAdminGroup}
	if clusterAdminGroup := os.Getenv("CLUSTER_ADMIN_GROUP"); clusterAdminGroup != "" {
		adminGroups = append(adminGroups, clusterAdminGroup)
	}
	for _, group := range groups {
		for _, adminGroup := range adminGroups {
			if group == adminGroup {
				return true
			}
		}
	}
	return false
}

// validateOwnershipPermission checks if the user has permission to modify/delete an OwnerOnly workspace
func validateOwnershipPermission(ctx context.Context, workspace *workspacev1alpha1.Workspace) error {
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return fmt.Errorf("unable to extract user information from request context: %w", err)
	}

	currentUser := sanitizeUsername(req.UserInfo.Username)
	workspacelog.Info("Validating ownership permission", "currentUser", currentUser)

	// Check if user is the owner
	if workspace.Annotations != nil {
		if createdBy := workspace.Annotations[controller.AnnotationCreatedBy]; createdBy != "" {
			workspacelog.Info("Checking ownership", "createdBy", createdBy, "currentUser", currentUser, "match", createdBy == currentUser)
			if createdBy == currentUser {
				return nil
			}
		}
	}

	// Check if user is cluster admin
	workspacelog.Info("Checking admin status", "groups", req.UserInfo.Groups)
	if isAdminUser(req.UserInfo.Groups) {
		workspacelog.Info("User is admin, allowing access")
		return nil
	}

	return fmt.Errorf("access denied: only workspace owner or cluster admins can modify OwnerOnly workspaces")
}

// SetupWorkspaceWebhookWithManager registers the webhook for Workspace in the manager.
func SetupWorkspaceWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&workspacev1alpha1.Workspace{}).
		WithValidator(&WorkspaceCustomValidator{}).
		WithDefaulter(&WorkspaceCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-workspace-jupyter-org-v1alpha1-workspace,mutating=true,failurePolicy=fail,sideEffects=None,groups=workspace.jupyter.org,resources=workspaces,verbs=create;update,versions=v1alpha1,name=mworkspace-v1alpha1.kb.io,admissionReviewVersions=v1,serviceName=jupyter-k8s-controller-manager,servicePort=9443

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
	workspace, ok := obj.(*workspacev1alpha1.Workspace)

	if !ok {
		return fmt.Errorf("expected an Workspace object but got %T", obj)
	}
	workspacelog.Info("Defaulting for Workspace", "name", workspace.GetName(), "namespace", workspace.GetNamespace())

	// Add ownership tracking annotations
	if workspace.Annotations == nil {
		workspace.Annotations = make(map[string]string)
	}

	// Extract user info from request context
	if req, err := admission.RequestFromContext(ctx); err == nil {
		sanitizedUsername := sanitizeUsername(req.UserInfo.Username)

		// Always set created-by on CREATE operations
		if req.Operation == "CREATE" {
			workspace.Annotations[controller.AnnotationCreatedBy] = sanitizedUsername
			workspacelog.Info("Added created-by annotation", "workspace", workspace.GetName(), "user", sanitizedUsername, "namespace", workspace.GetNamespace())
		}

		// Always set last-updated-by (CREATE and UPDATE operations)
		workspace.Annotations[controller.AnnotationLastUpdatedBy] = sanitizedUsername
		workspacelog.Info("Added last-updated-by annotation", "workspace", workspace.GetName(), "user", sanitizedUsername, "namespace", workspace.GetNamespace())
	}

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-workspace-jupyter-org-v1alpha1-workspace,mutating=false,failurePolicy=fail,sideEffects=None,groups=workspace.jupyter.org,resources=workspaces,verbs=create;update;delete,versions=v1alpha1,name=vworkspace-v1alpha1.kb.io,admissionReviewVersions=v1,serviceName=jupyter-k8s-controller-manager,servicePort=9443

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
	workspace, ok := obj.(*workspacev1alpha1.Workspace)
	if !ok {
		return nil, fmt.Errorf("expected a Workspace object but got %T", obj)
	}
	workspacelog.Info("Validation for Workspace upon creation", "name", workspace.GetName(), "namespace", workspace.GetNamespace())

	// TODO(user): fill in your validation logic upon object creation.

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Workspace.
func (v *WorkspaceCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldWorkspace, ok := oldObj.(*workspacev1alpha1.Workspace)
	if !ok {
		return nil, fmt.Errorf("expected a Workspace object for the oldObj but got %T", oldObj)
	}
	newWorkspace, ok := newObj.(*workspacev1alpha1.Workspace)
	if !ok {
		return nil, fmt.Errorf("expected a Workspace object for the newObj but got %T", newObj)
	}
	workspacelog.Info("Validation for Workspace upon update", "name", newWorkspace.GetName(), "namespace", newWorkspace.GetNamespace())

	// Check if user is admin
	isAdmin := false
	if req, err := admission.RequestFromContext(ctx); err == nil {
		isAdmin = isAdminUser(req.UserInfo.Groups)
	}

	// Validate that ownership annotations are immutable (except for admins)
	if oldWorkspace.Annotations != nil && oldWorkspace.Annotations[controller.AnnotationCreatedBy] != "" {
		oldCreatedBy := oldWorkspace.Annotations[controller.AnnotationCreatedBy]
		// Check if annotations are being cleared
		if newWorkspace.Annotations == nil {
			if !isAdmin {
				return nil, fmt.Errorf("created-by annotation cannot be removed")
			}
		}
		if newCreatedBy := newWorkspace.Annotations[controller.AnnotationCreatedBy]; newCreatedBy != oldCreatedBy {
			if !isAdmin {
				return nil, fmt.Errorf("created-by annotation is immutable")
			}
		}
	}

	originalOwnershipType := getEffectiveOwnershipType(oldWorkspace.Spec.OwnershipType)
	newOwnershipType := getEffectiveOwnershipType(newWorkspace.Spec.OwnershipType)
	workspacelog.Info("Ownership validation check", "originalType", originalOwnershipType, "newType", newOwnershipType)
	// For OwnerOnly workspaces, check if user has permission
	if originalOwnershipType == webhookconst.OwnershipTypeOwnerOnly {
		// Existing OwnerOnly workspace - check against old workspace
		if err := validateOwnershipPermission(ctx, oldWorkspace); err != nil {
			return nil, err
		}
	} else if newOwnershipType == webhookconst.OwnershipTypeOwnerOnly {
		// Changing to OwnerOnly - only allow if user is the original creator
		if err := validateOwnershipPermission(ctx, oldWorkspace); err != nil {
			return nil, err
		}
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Workspace.
func (v *WorkspaceCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	workspace, ok := obj.(*workspacev1alpha1.Workspace)
	if !ok {
		return nil, fmt.Errorf("expected a Workspace object but got %T", obj)
	}
	workspacelog.Info("Validation for Workspace upon deletion", "name", workspace.GetName(), "namespace", workspace.GetNamespace())

	// For OwnerOnly workspaces, check if user has permission
	effectiveOwnershipType := getEffectiveOwnershipType(workspace.Spec.OwnershipType)
	if effectiveOwnershipType == webhookconst.OwnershipTypeOwnerOnly {
		if err := validateOwnershipPermission(ctx, workspace); err != nil {
			return nil, err
		}
	}

	return nil, nil
}
