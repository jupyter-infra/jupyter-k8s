// Package extensionapi provides extension API server functionality.
package extensionapi

import (
	"context"
	"fmt"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlog "github.com/go-logr/logr"
)

const (
	// AccessTypePrivate indicates a private workspace with restricted access
	AccessTypePrivate string = "Private"

	// AccessTypePublic indicates a public workspace with broader access
	AccessTypePublic string = "Public"

	// DefaultAccessType is the fallback ownership type if none is specified
	DefaultAccessType = AccessTypePrivate

	// OwnerAnnotation is the annotation key for workspace owner
	OwnerAnnotation = "workspace.jupyter.org/created-by"
)

// WorkspaceAdmissionResult contains the result of a workspace access check
type WorkspaceAdmissionResult struct {
	Allowed       bool
	NotFound      bool
	Reason        string
	AccessType    string
	OwnerUsername string
}

// CheckWorkspaceAccess checks if a user has access to a workspace based on:
// 1. If workspace is public, grant access
// 2. If workspace is private, check if user is the owner
func (s *ExtensionServer) CheckWorkspaceAccess(
	namespace string,
	workspaceName string,
	username string,
	logger *rlog.Logger,
) (*WorkspaceAdmissionResult, error) {
	k8sClient := s.k8sClient

	// Get the workspace
	var workspace workspacev1alpha1.Workspace
	if err := k8sClient.Get(
		context.Background(),
		client.ObjectKey{Namespace: namespace, Name: workspaceName},
		&workspace,
	); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Workspace was not found")
			return &WorkspaceAdmissionResult{
				Allowed:       false,
				NotFound:      true,
				Reason:        "Workspace not found",
				AccessType:    "unknown",
				OwnerUsername: getWorkspaceOwner(&workspace),
			}, nil
		}

		logger.Error(err, "Failed to get workspace")
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	// Check access type
	accessType := getWorkspaceAccessType(&workspace)

	// If public, grant access
	if accessType == AccessTypePublic {
		logger.Info("Granting access to public workspace")
		return &WorkspaceAdmissionResult{
			Allowed:       true,
			NotFound:      false,
			Reason:        "Workspace is public",
			AccessType:    accessType,
			OwnerUsername: getWorkspaceOwner(&workspace),
		}, nil
	}

	// If private, check owner
	owner := getWorkspaceOwner(&workspace)

	// Owner check - simple string match for now
	if owner == username {
		logger.Info("Granting access to workspace owner")
		return &WorkspaceAdmissionResult{
			Allowed:       true,
			NotFound:      false,
			Reason:        "User is the workspace owner",
			AccessType:    accessType,
			OwnerUsername: owner,
		}, nil
	}

	// Access denied - not public and not the owner
	logger.Info("Denying access to private workspace")
	return &WorkspaceAdmissionResult{
		Allowed:       false,
		NotFound:      false,
		Reason:        "User is not the workspace owner",
		AccessType:    accessType,
		OwnerUsername: owner,
	}, nil
}

// getWorkspaceAccessType determines the ownership type of a workspace
func getWorkspaceAccessType(workspace *workspacev1alpha1.Workspace) string {
	if workspace != nil {
		accessType := workspace.Spec.OwnershipType
		if accessType == "" {
			return AccessTypePrivate
		}
		switch accessType {
		case "Public":
			{
				return AccessTypePublic
			}
		}
	}
	// For now, always assume private
	return AccessTypePrivate
}

// getWorkspaceOwner gets the username of the workspace owner
func getWorkspaceOwner(workspace *workspacev1alpha1.Workspace) string {
	// Look for owner annotation
	if workspace != nil && workspace.Annotations != nil {
		if owner, exists := workspace.Annotations[OwnerAnnotation]; exists && owner != "" {
			return owner
		}
	}
	return ""
}
