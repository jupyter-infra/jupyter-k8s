/*
MIT License

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
	"k8s.io/apimachinery/pkg/api/resource"
)

// ResourceBounds defines minimum and maximum resource limits
type ResourceBounds struct {
	// CPU bounds
	// +optional
	CPU *ResourceRange `json:"cpu,omitempty"`

	// Memory bounds
	// +optional
	Memory *ResourceRange `json:"memory,omitempty"`

	// GPU bounds
	// +optional
	GPU *ResourceRange `json:"gpu,omitempty"`
}

// ResourceRange defines min and max for a resource
// NOTE: CEL validation for min <= max is not possible due to resource.Quantity type limitations
// Validation is enforced at runtime in the template resolver
type ResourceRange struct {
	// Min is the minimum allowed value
	// +kubebuilder:validation:Required
	Min resource.Quantity `json:"min"`

	// Max is the maximum allowed value
	// +kubebuilder:validation:Required
	Max resource.Quantity `json:"max"`
}
