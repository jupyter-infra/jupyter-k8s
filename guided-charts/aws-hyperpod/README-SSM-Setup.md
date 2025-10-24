# AWS SSM Remote Access Strategy Setup

This guide helps you set up the required AWS IAM roles and EKS pod identity association for the `aws-ssm-remote-access` strategy.

## Prerequisites

- AWS CLI configured with appropriate permissions
- EKS cluster running
- `kubectl` configured to access your EKS cluster
- Workspace operator deployed

## Setup Overview

You need to create two IAM roles:
1. **SSM Managed Node Role** - Used by SSM agents in workspace pods
2. **Operator Role** - Used by the workspace controller to manage SSM resources

## Step 1: Create SSM Managed Node Role

### 1.1 Create Trust Policy for SSM Managed Node Role

```bash
# Set your AWS account ID
export AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)

# Create trust policy file
cat > ssm-managed-node-trust-policy.json << EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Service": "ssm.amazonaws.com"
      },
      "Action": [
        "sts:AssumeRole",
        "sts:TagSession"
      ]
    },
    {
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::${AWS_ACCOUNT_ID}:root"
      },
      "Action": [
        "sts:AssumeRole",
        "sts:TagSession"
      ]
    }
  ]
}
EOF
```

### 1.2 Create SSM Managed Node Role

```bash
# Create the role
aws iam create-role \
  --role-name workspace-ssm-managed-node-role \
  --assume-role-policy-document file://ssm-managed-node-trust-policy.json \
  --description "Role for SSM managed instances in workspace pods"

# Attach the required policy
aws iam attach-role-policy \
  --role-name workspace-ssm-managed-node-role \
  --policy-arn arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore

# Get the role ARN (save this for later)
aws iam get-role --role-name workspace-ssm-managed-node-role --query 'Role.Arn' --output text
```

## Step 2: Create Operator Role

The operator role uses **minimal scoped permissions** following the principle of least privilege:

- **`ssm:CreateActivation`** - Create SSM activations for workspace pods
- **`ssm:AddTagsToResource`** - Tag SSM activations with workspace metadata
- **`ssm:DescribeInstanceInformation`** - Find SSM instances (needed for cleanup)
- **`ssm:DeregisterManagedInstance`** - Clean up SSM instances when pods are deleted (scoped to workspace-tagged instances only)
- **`ssm:*Document*`** - Document operations for SSM functionality
- **`iam:PassRole`** - Pass the SSM managed node role (scoped to ssm.amazonaws.com only)

### 2.1 Create Trust Policy for Operator Role

```bash
# Create trust policy for EKS pod identity
cat > operator-trust-policy.json << EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Service": "pods.eks.amazonaws.com"
      },
      "Action": [
        "sts:AssumeRole",
        "sts:TagSession"
      ]
    }
  ]
}
EOF
```

### 2.2 Create Operator Role Permissions Policy

```bash
# Create minimal scoped permissions policy
cat > operator-permissions-policy.json << EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "SSMActivationManagement",
      "Effect": "Allow",
      "Action": [
        "ssm:CreateActivation",
        "ssm:AddTagsToResource"
      ],
      "Resource": "*"
    },
    {
      "Sid": "SSMDescribeInstances",
      "Effect": "Allow",
      "Action": [
        "ssm:DescribeInstanceInformation"
      ],
      "Resource": "*"
    },
    {
      "Sid": "SSMDeregisterWorkspaceInstances",
      "Effect": "Allow",
      "Action": [
        "ssm:DeregisterManagedInstance"
      ],
      "Resource": "*",
      "Condition": {
        "Null": {
          "ssm:resourceTag/workspace-pod-uid": "false"
        }
      }
    },
    {
      "Sid": "SSMDocumentAccess",
      "Effect": "Allow",
      "Action": [
        "ssm:CreateDocument",
        "ssm:UpdateDocument",
        "ssm:GetDocument",
        "ssm:DescribeDocument"
      ],
      "Resource": "*"
    },
    {
      "Sid": "PassRoleForSSMManagedNode",
      "Effect": "Allow",
      "Action": "iam:PassRole",
      "Resource": "arn:aws:iam::${AWS_ACCOUNT_ID}:role/workspace-ssm-managed-node-role",
      "Condition": {
        "StringEquals": {
          "iam:PassedToService": "ssm.amazonaws.com"
        }
      }
    }
  ]
}
EOF
```

### 2.3 Create Operator Role

```bash
# Create the role
aws iam create-role \
  --role-name workspace-operator-role \
  --assume-role-policy-document file://operator-trust-policy.json \
  --description "Role for workspace operator to manage SSM resources"

# Create and attach minimal scoped permissions policy
aws iam put-role-policy \
  --role-name workspace-operator-role \
  --policy-name WorkspaceOperatorSSMPermissions \
  --policy-document file://operator-permissions-policy.json

# Get the role ARN (save this for later)
aws iam get-role --role-name workspace-operator-role --query 'Role.Arn' --output text
```

## Step 3: Create EKS Pod Identity Association

### 3.1 Get Required Information

```bash
# Get your EKS cluster name
export CLUSTER_NAME="your-eks-cluster-name"

# Get the operator namespace (usually jupyter-k8s-system)
export OPERATOR_NAMESPACE="jupyter-k8s-system"

# Get the service account name
export SERVICE_ACCOUNT_NAME="jupyter-k8s-controller-manager"

# Get the operator role ARN from Step 2
export OPERATOR_ROLE_ARN=$(aws iam get-role --role-name workspace-operator-role --query 'Role.Arn' --output text)
```

### 3.2 Create Pod Identity Association

```bash
# Create the pod identity association
aws eks create-pod-identity-association \
  --cluster-name $CLUSTER_NAME \
  --namespace $OPERATOR_NAMESPACE \
  --service-account $SERVICE_ACCOUNT_NAME \
  --role-arn $OPERATOR_ROLE_ARN

# Verify the association was created
aws eks list-pod-identity-associations --cluster-name $CLUSTER_NAME
```

## Step 4: Deploy the Access Strategy

### 4.1 Install the Helm Chart

```bash
# Get the SSM managed node role name (without ARN prefix)
export SSM_ROLE_NAME="workspace-ssm-managed-node-role"

# Install the access strategy
helm install aws-hyperpod-strategy . \
  --set remoteAccess.enabled=true \
  --set remoteAccess.ssmManagedNodeRole="$SSM_ROLE_NAME" \
  --set remoteAccess.ssmSidecarImage="your-ecr-registry/sidecar-image:tag"
```

### 4.2 Verify Installation

```bash
# Check if the access strategy was created
kubectl get workspaceaccessstrategy aws-ssm-remote-access -o yaml

# Check operator logs
kubectl logs -f deployment/jupyter-k8s-controller-manager -n jupyter-k8s-system
```

## Step 5: Test with a Workspace

### 5.1 Create Test Workspace

```bash
cat > test-workspace.yaml << EOF
apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: test-ssm-workspace
  namespace: default
spec:
  displayName: "Test SSM Workspace"
  image: "public.ecr.aws/sagemaker/sagemaker-distribution:3.2.0-cpu"
  containerConfig:
    command: ["jupyter", "lab", "--core-mode", "--ip", "0.0.0.0", "--port", "8888"]
  resources:
    requests:
      memory: "2Gi"
      cpu: "1000m"
    limits:
      memory: "2Gi"
      cpu: "1000m"
  desiredStatus: "Running"
  accessStrategy:
    name: aws-ssm-remote-access
EOF

kubectl apply -f test-workspace.yaml
```

### 5.2 Monitor SSM Registration

```bash
# Watch pod creation
kubectl get pods -l workspace.jupyter.org/workspaceName=test-ssm-workspace -w

# Monitor operator logs for SSM workflow
kubectl logs -f deployment/jupyter-k8s-controller-manager -n jupyter-k8s-system | grep -i ssm
```

## Troubleshooting

### Common Issues

1. **SSM Client Not Available**
   - Check EKS pod identity association is created
   - Verify operator role has correct trust policy
   - Check operator pod has restarted after creating association

2. **Permission Denied Errors**
   - Verify operator role has the minimal scoped SSM permissions policy
   - Check `iam:PassRole` permission for SSM managed node role (scoped to ssm.amazonaws.com)
   - Ensure SSM managed node role has `AmazonSSMManagedInstanceCore` policy

3. **Pod Identity Association Issues**
   - Verify cluster name, namespace, and service account name are correct
   - Check association exists: `aws eks list-pod-identity-associations --cluster-name $CLUSTER_NAME`
   - Restart operator deployment after creating association

### Verification Commands

```bash
# Check operator role permissions
aws iam list-role-policies --role-name workspace-operator-role
aws iam get-role-policy --role-name workspace-operator-role --policy-name WorkspaceOperatorSSMPermissions

# Check SSM managed node role permissions  
aws iam list-attached-role-policies --role-name workspace-ssm-managed-node-role

# Check pod identity associations
aws eks describe-pod-identity-association \
  --cluster-name $CLUSTER_NAME \
  --association-id $(aws eks list-pod-identity-associations --cluster-name $CLUSTER_NAME --query 'associations[0].associationId' --output text)
```

## Cleanup

To remove the setup:

```bash
# Delete pod identity association
aws eks delete-pod-identity-association \
  --cluster-name $CLUSTER_NAME \
  --association-id $(aws eks list-pod-identity-associations --cluster-name $CLUSTER_NAME --query 'associations[0].associationId' --output text)

# Delete operator role
aws iam delete-role-policy --role-name workspace-operator-role --policy-name WorkspaceOperatorSSMPermissions
aws iam delete-role --role-name workspace-operator-role

# Delete SSM managed node role
aws iam detach-role-policy --role-name workspace-ssm-managed-node-role --policy-arn arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore
aws iam delete-role --role-name workspace-ssm-managed-node-role

# Uninstall Helm chart
helm uninstall aws-hyperpod-strategy
```