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
	"fmt"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/controller"
)

// validateImageAllowed checks if image is in template's allowed list
func validateImageAllowed(image string, template *workspacev1alpha1.WorkspaceTemplate) *controller.TemplateViolation {
	effectiveAllowedImages := template.Spec.AllowedImages
	if len(template.Spec.AllowedImages) == 0 {
		effectiveAllowedImages = []string{template.Spec.DefaultImage}
	}

	for _, allowed := range effectiveAllowedImages {
		if image == allowed {
			return nil
		}
	}

	return &controller.TemplateViolation{
		Type:    controller.ViolationTypeImageNotAllowed,
		Field:   "spec.image",
		Message: fmt.Sprintf("Image '%s' is not allowed by template '%s'. Allowed images: %v", image, template.Name, effectiveAllowedImages),
		Allowed: fmt.Sprintf("%v", effectiveAllowedImages),
		Actual:  image,
	}
}
