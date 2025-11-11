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

	// SSHDocumentContent is the JSON content for the SSH session document
	SSHDocumentContent = `{
 "schemaVersion": "1.0",
 "description": "Document to hold regional settings for Session Manager for SSH connections",
 "sessionType": "Port",
 "parameters": {
  "portNumber": {
   "type": "String",
   "description": "(Optional) Port number of SSH server on the instance",
   "allowedPattern": "^([1-9]|[1-9][0-9]{1,3}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5])$",
   "default": "22"
  }
 },
 "inputs": {
  "idleSessionTimeout": 60,
  "maxSessionDuration": 720
 },
 "properties": {
  "portNumber": "{{ portNumber }}"
 }
}`
)
