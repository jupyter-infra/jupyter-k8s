/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

// Direct image references for built-in images without registry prefix
// The registry will be added by the image resolver based on the environment

// Default tag
const (
	DefaultTag = "latest"
)

// Image constants - simple names without registry prefix
const (
	// Standard images - registry will be added at runtime
	ImageJupyterUV = "jk8s-application-jupyter-uv:latest"

	// Default image
	DefaultJupyterImage = ImageJupyterUV
)

// Map of image shortcuts to image names
var jupyterImages = map[string]string{
	"uv": ImageJupyterUV,
}

// IsBuiltInImage checks if an image name is a built-in shortcut
func IsBuiltInImage(imageName string) bool {
	_, exists := jupyterImages[imageName]
	return exists
}

// GetImagePath returns the full image path for a built-in image name
// If the image is not a built-in, it returns the original image name
func GetImagePath(imageName string) string {
	if fullPath, exists := jupyterImages[imageName]; exists {
		return fullPath
	}
	return imageName
}
