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

package controller

import (
	"fmt"
	"strings"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// ImageResolver handles the resolution of image references
type ImageResolver struct {
	// Registry prefix to use for images (e.g. "example.com/my-registry")
	// If empty, uses the image names directly (for local development)
	Registry string
}

// NewImageResolver creates a new image resolver
func NewImageResolver(registry string) *ImageResolver {
	return &ImageResolver{
		Registry: registry,
	}
}

// ResolveImage resolves an image reference from a Workspace spec
// It handles:
// - Direct image references
// - Built-in image shortcuts
// - Default image when none is specified
// - Adding registry prefix in production environments
func (r *ImageResolver) ResolveImage(workspace *workspacev1alpha1.Workspace) string {
	// Get image from server spec
	image := workspace.Spec.Image

	// If this is a built-in image shortcut, resolve to the base image name
	if IsBuiltInImage(image) {
		image = GetImagePath(image)
	}

	// If no image is specified, use the default
	if image == "" {
		image = DefaultJupyterImage
	}

	// Add registry prefix if it's set and the image doesn't already have one
	if r.Registry != "" {
		// Split image into repository and tag parts
		parts := strings.SplitN(image, ":", 2)
		repository := parts[0]
		tag := DefaultTag
		if len(parts) > 1 {
			tag = parts[1]
		}

		// Check if repository has a slash (which would indicate it already has a registry)
		if !strings.Contains(repository, "/") {
			// Simple image name - prepend the registry
			return fmt.Sprintf("%s/%s:%s", r.Registry, repository, tag)
		}
	}

	return image
}
