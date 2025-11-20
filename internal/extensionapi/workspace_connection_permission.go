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
