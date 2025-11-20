/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

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

	// MaxConcurrentSSMSessionsPerInstance is the maximum number of concurrent SSM sessions allowed per managed instance
	MaxConcurrentSSMSessionsPerInstance = 10
)

// SageMakerSpaceSSHSessionDocumentContent is the JSON content for the SSH session document
//
//go:embed sagemaker_space_ssh_document_content.json
var SageMakerSpaceSSHSessionDocumentContent string
