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
