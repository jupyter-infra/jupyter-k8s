/*
MIT License

Copyright (c) 2025 jupyter-ai-contrib

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.
*/

package controller

// TemplateValidationResult contains the result of template validation
type TemplateValidationResult struct {
	// Valid indicates if the workspace configuration passes all template validations
	Valid bool

	// Violations contains details about any validation failures
	// Empty slice if Valid is true
	Violations []TemplateViolation

	// Template contains the resolved template configuration
	// INVARIANT: Template is never nil when Valid is true
	// May be nil when Valid is false (e.g., template not found)
	// This ensures downstream components can safely use Template when validation passes
	Template *ResolvedTemplate
}

// TemplateViolation describes a specific validation failure
type TemplateViolation struct {
	// Type categorizes the violation (e.g., "ImageNotAllowed", "ResourceExceeded")
	Type string

	// Field identifies the configuration field that failed validation
	// Uses JSONPath-like notation (e.g., "spec.templateOverrides.image")
	Field string

	// Message provides a human-readable description of the violation
	Message string

	// Allowed describes what values/ranges are permitted
	Allowed string

	// Actual shows what the user provided
	Actual string
}

// Common violation types
const (
	ViolationTypeImageNotAllowed                = "ImageNotAllowed"
	ViolationTypeResourceExceeded               = "ResourceExceeded"
	ViolationTypeStorageExceeded                = "StorageExceeded"
	ViolationTypeInvalidTemplate                = "InvalidTemplate"
	ViolationTypeIdleShutdownOverrideNotAllowed = "IdleShutdownOverrideNotAllowed"
	ViolationTypeIdleShutdownTimeoutOutOfBounds = "IdleShutdownTimeoutOutOfBounds"
)
