/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

// Package awsplugin provides AWS SDK implementations for the plugin sidecar.
package awsplugin

import _ "embed"

// --- Init-only constants (not overridable via context) ---

const (
	// CustomSSHDocumentName is the name of the SSM document for SSH sessions
	CustomSSHDocumentName = "SageMaker-SpaceSSHSessionDocument"
)

// SageMakerSpaceSSHSessionDocumentContent is the JSON content for the SSH session document
//
//go:embed sagemaker_space_ssh_document_content.json
var SageMakerSpaceSSHSessionDocumentContent string
