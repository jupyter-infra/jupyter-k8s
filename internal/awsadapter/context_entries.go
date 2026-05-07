/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

// Package awsadapter provides the SSM-based pod event adapter implementation.
// It reads all configuration from the podEventsContext map defined in the AccessStrategy.
package awsadapter

import "github.com/jupyter-infra/jupyter-k8s-plugin/plugin"

// Pod event context keys resolved from the AccessStrategy's podEventsContext map.
// These keys have no defaults — values must be provided in the AccessStrategy.
var (
	// ContextKeyPodUID is the context key for the pod UID (resolved from controller::PodUid()).
	ContextKeyPodUID = plugin.ContextEntry{Key: "podUid"}
	// ContextKeySsmManagedNodeRole is the context key for the IAM role used for SSM activation.
	ContextKeySsmManagedNodeRole = plugin.ContextEntry{Key: "ssmManagedNodeRole"}
	// ContextKeySidecarContainerName is the context key for the SSM agent sidecar container name.
	ContextKeySidecarContainerName = plugin.ContextEntry{Key: "sidecarContainerName"}
	// ContextKeyWorkspaceContainerName is the context key for the main workspace container name.
	ContextKeyWorkspaceContainerName = plugin.ContextEntry{Key: "workspaceContainerName"}
	// ContextKeyRegistrationStateFile is the context key for the state file path in the shared volume.
	ContextKeyRegistrationStateFile = plugin.ContextEntry{Key: "registrationStateFile"}
	// ContextKeyRegistrationMarkerFile is the context key for the legacy marker file path.
	ContextKeyRegistrationMarkerFile = plugin.ContextEntry{Key: "registrationMarkerFile"}
	// ContextKeyRegistrationScript is the context key for the SSM registration script path.
	ContextKeyRegistrationScript = plugin.ContextEntry{Key: "registrationScript"}
	// ContextKeyRemoteAccessServerScript is the context key for the remote access server startup script.
	ContextKeyRemoteAccessServerScript = plugin.ContextEntry{Key: "remoteAccessServerScript"}
	// ContextKeyRemoteAccessServerPort is the context key for the remote access server port.
	ContextKeyRemoteAccessServerPort = plugin.ContextEntry{Key: "remoteAccessServerPort"}
	// ContextKeyRegion is the context key for the AWS region used in SSM registration.
	ContextKeyRegion = plugin.ContextEntry{Key: "region"}
)
