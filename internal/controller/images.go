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
