/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package awsplugin

import "github.com/jupyter-infra/jupyter-k8s/internal/plugin"

// --- Pod event context keys ---
// These are resolved from podEventsContext (RegisterNodeAgent / DeregisterNodeAgent).

var (
	// WorkspacePodUIDTagKey is the SSM tag filter key used to identify workspace pods.
	WorkspacePodUIDTagKey = plugin.ContextEntry{
		Key:     "workspacePodUIDTagKey",
		Default: "tag:workspace-pod-uid",
	}

	// SageMakerManagedByTagKey is the tag key for SageMaker managed-by identification.
	SageMakerManagedByTagKey = plugin.ContextEntry{
		Key:     "sagemakerManagedByTagKey",
		Default: "sagemaker.amazonaws.com/managed-by",
	}

	// SageMakerManagedByTagValue is the required value for the SageMaker managed-by tag.
	SageMakerManagedByTagValue = plugin.ContextEntry{
		Key:     "sagemakerManagedByTagValue",
		Default: "amazon-sagemaker-spaces",
	}

	// SageMakerEKSClusterTagKey is the tag key for SageMaker EKS cluster ARN.
	SageMakerEKSClusterTagKey = plugin.ContextEntry{
		Key:     "sagemakerEKSClusterTagKey",
		Default: "sagemaker.amazonaws.com/eks-cluster-arn",
	}

	// SSMInstanceNamePrefix is the prefix used for SSM instance names.
	SSMInstanceNamePrefix = plugin.ContextEntry{
		Key:     "ssmInstanceNamePrefix",
		Default: "workspace",
	}
)

// --- Connection context keys ---
// These are resolved from createConnectionContext (CreateSession).

var (
	// VSCodeScheme is the URL scheme for VSCode remote connections.
	VSCodeScheme = plugin.ContextEntry{
		Key:     "vscodeScheme",
		Default: "vscode://amazonwebservices.aws-toolkit-vscode/connect/workspace",
	}

	// MaxConcurrentSSMSessions is the maximum concurrent SSM sessions per managed instance.
	MaxConcurrentSSMSessions = plugin.ContextEntry{
		Key:     "maxConcurrentSSMSessions",
		Default: "10",
	}
)
