package controller

import (
	"fmt"
	"strings"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
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
