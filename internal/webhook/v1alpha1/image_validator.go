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
