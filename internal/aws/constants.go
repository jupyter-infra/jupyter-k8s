// Package aws provides AWS-related constants for the workspace controller.
package aws

const (
	// AWSSSMDocumentNameEnv is the environment variable for SSM document name
	AWSSSMDocumentNameEnv = "AWS_SSM_DOCUMENT_NAME"

	// EKSClusterARNEnv is the environment variable key for EKS cluster ARN
	EKSClusterARNEnv = "CLUSTER_ID"

	// SSHDocumentContentEnv is the environment variable for SSH document content
	SSHDocumentContentEnv = "SSH_DOCUMENT_CONTENT"

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

	// KMSJWTKeyAlias is the alias for the JWT signing KMS key
	KMSJWTKeyAlias = "alias/sagemaker-devspace-jwt-key"

	// SSMInstanceNamePrefix is the prefix used for SSM instance names
	SSMInstanceNamePrefix = "workspace"

	// VSCodeScheme is the URL scheme for VSCode remote connections
	VSCodeScheme = "vscode://amazonwebservices.aws-toolkit-vscode/connect/workspace"

	// CustomSSHDocumentName is the name of the SSM document for SSH sessions
	CustomSSHDocumentName = "SageMaker-SpaceSSHSessionDocument"
)
