// Package aws provides AWS-related constants for the workspace controller.
package aws

const (
	// AWSSSMDocumentNameEnv is the environment variable for SSM document name
	AWSSSMDocumentNameEnv = "AWS_SSM_DOCUMENT_NAME"

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

	// SSMInstanceNamePrefix is the prefix used for SSM instance names
	SSMInstanceNamePrefix = "workspace"

	// VSCodeScheme is the URL scheme for VSCode remote connections
	VSCodeScheme = "vscode://amazonwebservices.aws-toolkit-vscode/connect/sagemaker"
)
