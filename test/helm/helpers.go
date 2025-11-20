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

// Package helm contains helpers for Helm chart testing
package helm

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"sigs.k8s.io/yaml"
)

// Resource represents a kubernetes resource with identifying fields
type Resource struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace,omitempty"`
	} `json:"metadata"`
}

// ResourceIdentifier is a unique identifier for a k8s resource
type ResourceIdentifier struct {
	Kind       string
	Namespace  string
	Name       string
	SourceFile string // The source file where this resource was defined
}

// String representation of ResourceIdentifier
func (r ResourceIdentifier) String() string {
	if r.Namespace == "" {
		return r.Kind + ":" + r.Name
	}
	return r.Kind + ":" + r.Namespace + ":" + r.Name
}

// GetKey returns a unique identifier string without the source file information
func (r ResourceIdentifier) GetKey() string {
	return r.String()
}

// ParseYAMLFile parses a YAML file and returns a set of resources
func ParseYAMLFile(filePath string) (map[ResourceIdentifier]bool, error) {
	// Read the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// Split the file by YAML document separator
	resources := make(map[ResourceIdentifier]bool)
	docs := strings.Split(string(data), "\n---\n")

	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}

		// Parse the YAML document
		var resource Resource
		if err := yaml.Unmarshal([]byte(doc), &resource); err != nil {
			continue // Skip documents that are not valid Kubernetes resources
		}

		// Skip empty resources (like comments)
		if resource.Kind == "" {
			continue
		}

		// Create a resource identifier
		id := ResourceIdentifier{
			Kind:       resource.Kind,
			Namespace:  resource.Metadata.Namespace,
			Name:       resource.Metadata.Name,
			SourceFile: filePath,
		}

		resources[id] = true
	}

	return resources, nil
}

// ResourceMap is a map from resource keys to source files
type ResourceMap struct {
	Resources map[string][]string // Map from resource key to list of source files
	BySource  map[string][]string // Map from source file to list of resource keys
}

// NewResourceMap creates a new empty ResourceMap
func NewResourceMap() *ResourceMap {
	return &ResourceMap{
		Resources: make(map[string][]string),
		BySource:  make(map[string][]string),
	}
}

// AddResource adds a resource to the ResourceMap
func (rm *ResourceMap) AddResource(id ResourceIdentifier) {
	key := id.GetKey()
	sourceFile := id.SourceFile

	// Add to Resources map
	if _, exists := rm.Resources[key]; !exists {
		rm.Resources[key] = []string{}
	}
	rm.Resources[key] = append(rm.Resources[key], sourceFile)

	// Add to BySource map
	if _, exists := rm.BySource[sourceFile]; !exists {
		rm.BySource[sourceFile] = []string{}
	}
	rm.BySource[sourceFile] = append(rm.BySource[sourceFile], key)
}

// ParseYAMLDirectory parses all YAML files in a directory and its subdirectories
// Returns a ResourceMap that tracks which resources come from which files
func ParseYAMLDirectory(dirPath string) (map[ResourceIdentifier]bool, *ResourceMap, error) {
	resources := make(map[ResourceIdentifier]bool)
	resourceMap := NewResourceMap()

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".yaml") && info.Name() != "kustomization.yaml" {
			fileResources, err := ParseYAMLFile(path)
			if err != nil {
				return err
			}

			// Add resources from this file to the overall set
			for res := range fileResources {
				resources[res] = true
				resourceMap.AddResource(res)
			}
		}
		return nil
	})

	return resources, resourceMap, err
}

// ValuesSchema represents the parsed structure of values.yaml
type ValuesSchema map[string]interface{}

// ExtractValuesSchema parses values.yaml into a structured map
func ExtractValuesSchema(valuesPath string) (ValuesSchema, error) {
	data, err := os.ReadFile(valuesPath)
	if err != nil {
		return nil, err
	}

	var valuesSchema ValuesSchema
	if err := yaml.Unmarshal(data, &valuesSchema); err != nil {
		return nil, err
	}

	return valuesSchema, nil
}

// ExtractTemplateReferences finds all .Values.* references in template files
func ExtractTemplateReferences(templatesDir string) ([]string, error) {
	var references []string
	valuesRegex := regexp.MustCompile(`\.Values\.([a-zA-Z0-9._]+)`)

	err := filepath.Walk(templatesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		matches := valuesRegex.FindAllStringSubmatch(string(content), -1)
		for _, match := range matches {
			if len(match) >= 2 {
				references = append(references, match[1])
			}
		}

		return nil
	})

	return references, err
}

// CheckReferenceExists verifies if a dotted path exists in the values schema
func CheckReferenceExists(reference string, schema ValuesSchema) bool {
	parts := strings.Split(reference, ".")
	current := map[string]interface{}(schema)

	for i, part := range parts {
		value, exists := current[part]
		if !exists {
			return false
		}

		// If we're at the last part, we've found the reference
		if i == len(parts)-1 {
			return true
		}

		// If we need to go deeper but can't
		if nextLevel, ok := value.(map[string]interface{}); ok {
			current = nextLevel
		} else {
			// We have more path components but hit a leaf value
			return false
		}
	}

	return true
}

// FindInvalidReferences returns a list of template references that don't exist in values
func FindInvalidReferences(references []string, schema ValuesSchema, ignorePaths ...string) []string {
	invalidRefs := []string{}

	// Use map to deduplicate references
	uniqueRefs := make(map[string]bool)
	for _, ref := range references {
		uniqueRefs[ref] = true
	}

	// Create a map of paths to ignore
	ignoreMap := make(map[string]bool)
	for _, path := range ignorePaths {
		ignoreMap[path] = true
	}

	for ref := range uniqueRefs {
		if !CheckReferenceExists(ref, schema) {
			// Skip if this path should be ignored
			if _, shouldIgnore := ignoreMap[ref]; !shouldIgnore {
				invalidRefs = append(invalidRefs, ref)
			}
		}
	}

	return invalidRefs
}

// CompareResourceMaps compares source and rendered resources, tracking which files contribute to which resources
// Returns:
// - missingResources: source resources not in rendered output
// - unmatchedResources: rendered resources not from any source file
// - ok: true if all checks pass
func CompareResourceMaps(sourceResources map[ResourceIdentifier]bool, sourceMap *ResourceMap,
	renderedResources map[ResourceIdentifier]bool, renderedMap *ResourceMap) ([]string, []string, bool) {
	missingResources := []string{}
	unmatchedResources := []string{}

	// Check that all source resources exist in rendered resources
	for src := range sourceResources {
		if src.Kind == "" {
			continue // Skip empty resources (like validations.yaml)
		}

		// Check if this resource exists in rendered resources by key (ignoring source file)
		found := false
		srcKey := src.GetKey()
		for rend := range renderedResources {
			if rend.GetKey() == srcKey {
				found = true
				break
			}
		}

		if !found {
			missingResources = append(missingResources, src.String())
		}
	}

	// Check if rendered resources can be traced back to source files
	// A rendered resource is "matched" if there is a source file that contains
	// a resource that matches its Kind + Name + Namespace
	sourceKeyMap := make(map[string]bool)
	for src := range sourceResources {
		sourceKeyMap[src.GetKey()] = true
	}

	for rend := range renderedResources {
		if _, exists := sourceKeyMap[rend.GetKey()]; !exists {
			unmatchedResources = append(unmatchedResources, rend.String())
		}
	}

	return missingResources, unmatchedResources, len(missingResources) == 0
}
