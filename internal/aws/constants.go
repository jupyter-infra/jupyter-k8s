// Package aws provides AWS-related constants for the workspace controller.
package aws

import (
	_ "embed"
)

const (
	// EKSClusterARNEnv is the environment variable key for EKS cluster ARN
	EKSClusterARNEnv = "CLUSTER_ID"

	// WorkspacePodUIDTagKey is the tag key used to identify workspace pods in SSM
	WorkspacePodUIDTagKey = "tag:workspace-pod-uid"

	// SageMakerManagedByTagKey is the tag key for SageMaker managed-by identification
	SageMakerManagedByTagKey = "sagemaker.amazonaws.com/managed-by"
	// SageMakerManagedByTagValue is the required value for SageMaker managed-by tag
	SageMakerManagedByTagValue = "amazon-sagemaker-spaces"
	// SageMakerEKSClusterTagKey is the tag key for SageMaker EKS cluster ARN
	SageMakerEKSClusterTagKey = "sagemaker.amazonaws.com/eks-cluster-arn"
	// SageMakerPurposeTagKey is the tag key for SageMaker purpose identification
	SageMakerPurposeTagKey = "sagemaker.amazonaws.com/purpose"
	// SageMakerJWTSigningTagValue is the tag value for JWT signing purpose
	SageMakerJWTSigningTagValue = "JWT-Signing"

	// KMSJWTKeyAliasPattern is the pattern for the JWT signing KMS key
	KMSJWTKeyAliasPattern = "alias/sagemaker-devspace-key-%s"

	// SSMInstanceNamePrefix is the prefix used for SSM instance names
	SSMInstanceNamePrefix = "workspace"

	// VSCodeScheme is the URL scheme for VSCode remote connections
	VSCodeScheme = "vscode://amazonwebservices.aws-toolkit-vscode/connect/workspace"

	// CustomSSHDocumentName is the name of the SSM document for SSH sessions
	CustomSSHDocumentName = "SageMaker-SpaceSSHSessionDocument"
)

// SageMakerSpaceSSHSessionDocumentContent is the JSON content for the SSH session document
//
//go:embed sagemaker_space_ssh_document_content.json
var SageMakerSpaceSSHSessionDocumentContent string
