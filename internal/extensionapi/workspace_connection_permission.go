/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

// Package extensionapi provides extension API server functionality.
package extensionapi

import (
	rlog "github.com/go-logr/logr"
	authorizationv1 "k8s.io/api/authorization/v1"
)

// PermissionCheckResult contains the result of the permission check
type PermissionCheckResult struct {
	Allowed  bool
	NotFound bool
	Reason   string
}

// CheckWorkspaceConnectionPermission checks if a user has permission to connect to a workspace
// by performing the following checks in sequence:
// 1. RBAC check - does the user have permission to create workspace/connection?
// 2. Workspace check - is the workspace public or is the user the owner?
func (s *ExtensionServer) CheckWorkspaceConnectionPermission(
	namespace string,
	workspaceName string,
	username string,
	groups []string,
	uid string,
	extra map[string]authorizationv1.ExtraValue,
	logger *rlog.Logger,
) (*PermissionCheckResult, error) {
	// Step 1: Check RBAC permissions
	rbacResult, err := s.CheckRBACPermission(namespace, username, groups, uid, extra, logger)
	if err != nil {
		logger.Error(err, "RBAC check failed with error")
		return nil, err
	}

	if !rbacResult.Allowed {
		// RBAC check failed, deny access
		logger.Info("RBAC permission denied")
		return &PermissionCheckResult{
			Allowed:  false,
			NotFound: false,
			Reason:   "RBAC permission denied",
		}, nil
	}

	// Step 2: Check workspace access
	workspaceResult, err := s.CheckWorkspaceAccess(namespace, workspaceName, username, logger)
	if err != nil {
		logger.Error(err, "Workspace access check failed with error")
		return nil, err
	}

	if workspaceResult.NotFound {
		return &PermissionCheckResult{
			Allowed:  false,
			NotFound: workspaceResult.NotFound,
			Reason:   "Workspace not found",
		}, nil
	}

	if !workspaceResult.Allowed {
		// Workspace access denied
		logger.Info("Workspace access denied",
			"accessType", workspaceResult.AccessType,
			"owner", workspaceResult.OwnerUsername)

		return &PermissionCheckResult{
			Allowed:  workspaceResult.Allowed,
			NotFound: workspaceResult.NotFound,
			Reason:   "User is not the owner of the private Workspace",
		}, nil
	}

	// All checks passed, grant access
	var reason string
	if workspaceResult.AccessType == AccessTypePublic {
		reason = "Valid RBAC and the subject Workspace is public"
	} else {
		reason = "Valid RBAC and user is the owner of the private Workspace"
	}

	logger.Info("Access granted",
		"accessType", workspaceResult.AccessType,
		"owner", workspaceResult.OwnerUsername,
		"reason", reason)

	return &PermissionCheckResult{
		Allowed:  workspaceResult.Allowed,
		NotFound: workspaceResult.NotFound,
		Reason:   reason,
	}, nil
}
