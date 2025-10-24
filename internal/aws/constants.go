// Package aws provides AWS-related constants for the workspace controller.
package aws

const (
	// AWSSSMDocumentNameEnv is the environment variable for SSM document name
	AWSSSMDocumentNameEnv = "AWS_SSM_DOCUMENT_NAME"

	// ClusterARNEnv is the environment variable for the cluster ARN
	ClusterARNEnv = "CLUSTER_ARN"

	// WorkspacePodUIDTagKey is the tag key used to identify workspace pods in SSM
	WorkspacePodUIDTagKey = "tag:workspace-pod-uid"
)
