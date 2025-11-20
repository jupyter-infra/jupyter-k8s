package v1alpha1

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
	ViolationTypeSecondaryStorageNotAllowed     = "SecondaryStorageNotAllowed"
	ViolationTypeVolumeOwnedByAnotherWorkspace  = "VolumeOwnedByAnotherWorkspace"
	ViolationTypeInvalidTemplate                = "InvalidTemplate"
	ViolationTypeIdleShutdownOverrideNotAllowed = "IdleShutdownOverrideNotAllowed"
	ViolationTypeIdleShutdownTimeoutOutOfBounds = "IdleShutdownTimeoutOutOfBounds"
)
