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
	"reflect"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/controller"
)

// isOnlyStoppingWorkspace checks if the only change is setting desiredStatus to Stopped
func isOnlyStoppingWorkspace(old, new *workspacev1alpha1.Workspace) bool {
	if new.Spec.DesiredStatus != controller.PhaseStopped {
		return false
	}

	// Create copies and normalize desiredStatus to compare other fields
	oldCopy := old.DeepCopy()
	newCopy := new.DeepCopy()
	newCopy.Spec.DesiredStatus = oldCopy.Spec.DesiredStatus

	return reflect.DeepEqual(oldCopy.Spec, newCopy.Spec)
}
