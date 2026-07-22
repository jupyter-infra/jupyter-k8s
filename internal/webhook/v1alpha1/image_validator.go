/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"fmt"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// validateImageAllowed checks if image is in template's allowed list
func validateImageAllowed(image string, template *workspacev1alpha1.WorkspaceTemplate) *TemplateViolation {
	// Skip validation if custom images are allowed
	if template.Spec.AllowCustomImages != nil && *template.Spec.AllowCustomImages {
		return nil
	}

	effectiveAllowedImages := template.Spec.AllowedImages
	if len(template.Spec.AllowedImages) == 0 {
		effectiveAllowedImages = []string{template.Spec.DefaultImage}
	}

	for _, allowed := range effectiveAllowedImages {
		if image == allowed {
			return nil
		}
	}

	return &TemplateViolation{
		Type:    ViolationTypeImageNotAllowed,
		Field:   "spec.image",
		Message: fmt.Sprintf("Image '%s' is not allowed by template '%s'. Allowed images: %v", image, template.Name, effectiveAllowedImages),
		Allowed: fmt.Sprintf("%v", effectiveAllowedImages),
		Actual:  image,
	}
}

// validateTemplateImageConsistency rejects a template whose own defaultImage would be
// un-creatable under its own image policy. When allowedImages is non-empty and custom images
// are not allowed, defaultImage must be a member of allowedImages: otherwise the workspace
// defaulter fills image=defaultImage for a workspace that omits one, and validateImageAllowed
// then rejects that same image, making the template's default impossible to use (see #440).
//
// When allowedImages is empty, validateImageAllowed falls back to [defaultImage], so the
// default is always creatable and no consistency check is needed.
func validateTemplateImageConsistency(template *workspacev1alpha1.WorkspaceTemplate) error {
	if template.Spec.AllowCustomImages != nil && *template.Spec.AllowCustomImages {
		return nil
	}

	if len(template.Spec.AllowedImages) == 0 {
		return nil
	}

	for _, allowed := range template.Spec.AllowedImages {
		if template.Spec.DefaultImage == allowed {
			return nil
		}
	}

	return fmt.Errorf(
		"defaultImage %q is not in allowedImages %v: the template default would be rejected by its own "+
			"image policy, making it un-creatable (template %q)",
		template.Spec.DefaultImage, template.Spec.AllowedImages, template.GetName(),
	)
}
