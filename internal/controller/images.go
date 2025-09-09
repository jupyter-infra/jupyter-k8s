package controller

// Direct image references for built-in images
// No registry prefix needed for local development with Kind
// For production, update these with your registry prefix

// Image constants
const (
	// Standard images
	ImageJupyterUV = "docker.io/library/jupyter-uv:latest"

	// Default image
	DefaultJupyterImage = ImageJupyterUV
)

// Map of image shortcuts to full paths
var JupyterImages = map[string]string{
	"uv": ImageJupyterUV,
}

// IsBuiltInImage checks if an image name is a built-in shortcut
func IsBuiltInImage(imageName string) bool {
	_, exists := JupyterImages[imageName]
	return exists
}

// GetImagePath returns the full image path for a built-in image name
// If the image is not a built-in, it returns the original image name
func GetImagePath(imageName string) string {
	if fullPath, exists := JupyterImages[imageName]; exists {
		return fullPath
	}
	return imageName
}
