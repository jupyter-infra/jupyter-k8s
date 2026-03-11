/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"fmt"
	"strings"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
)

// validateReservedPrefixOnCreate rejects any workspace.jupyter.org/ prefixed labels or annotations
// that are not in the system-managed allow-list.
func validateReservedPrefixOnCreate(workspace *workspacev1alpha1.Workspace) error {
	if err := checkReservedKeys(workspace.Labels, "label"); err != nil {
		return err
	}
	return checkReservedKeys(workspace.Annotations, "annotation")
}

// validateReservedPrefixOnUpdate rejects user changes to workspace.jupyter.org/ prefixed labels or annotations.
// For SetOnCreateOnly keys: rejects any value change or removal.
// For SetAlways keys: allows changes (system will overwrite).
// For unknown labels/annotations with reserved keys: rejects additions, changes, and removals.
func validateReservedPrefixOnUpdate(oldWorkspace, newWorkspace *workspacev1alpha1.Workspace) error {
	if err := checkReservedKeyChanges(oldWorkspace.Labels, newWorkspace.Labels, "label"); err != nil {
		return err
	}
	return checkReservedKeyChanges(oldWorkspace.Annotations, newWorkspace.Annotations, "annotation")
}

// checkReservedKeys checks if any key with the reserved prefix is not system-managed.
func checkReservedKeys(metadata map[string]string, kind string) error {
	for key := range metadata {
		if strings.HasPrefix(key, controller.ReservedMetadataPrefix) {
			if _, ok := controller.SystemManagedMetadataKeys[key]; !ok {
				return fmt.Errorf("%s '%s' uses reserved prefix %s", kind, key, controller.ReservedMetadataPrefix)
			}
		}
	}
	return nil
}

// checkReservedKeyChanges detects user modifications to reserved-prefix keys between old and new metadata.
func checkReservedKeyChanges(oldMeta, newMeta map[string]string, kind string) error {
	// Check for added or changed keys in new
	for key, newVal := range newMeta {
		if !strings.HasPrefix(key, controller.ReservedMetadataPrefix) {
			continue
		}

		// Reject if newly added reserved key is not allow-listed
		policy, isSystem := controller.SystemManagedMetadataKeys[key]
		if !isSystem {
			// Unknown reserved key added
			return fmt.Errorf("%s '%s' uses reserved prefix %s", kind, key, controller.ReservedMetadataPrefix)
		}

		// Reject if changed reserved key is set on create only
		oldVal, existed := oldMeta[key]
		if existed && oldVal != newVal && policy == controller.SetOnCreateOnly {
			return fmt.Errorf("%s '%s' is immutable", kind, key)
		}
	}

	// Check for removed keys
	for key := range oldMeta {
		if !strings.HasPrefix(key, controller.ReservedMetadataPrefix) {
			continue
		}
		if _, exists := newMeta[key]; !exists {

			// Reject if deleted reserved key is set on create only
			policy, isSystem := controller.SystemManagedMetadataKeys[key]
			if !isSystem || policy == controller.SetOnCreateOnly {
				return fmt.Errorf("%s '%s' cannot be removed", kind, key)
			}
		}
	}

	return nil
}
