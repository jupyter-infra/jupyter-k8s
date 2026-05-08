# AWS Plugin

The AWS plugin implements remote access via AWS Systems Manager (SSM).

## What it does

1. **Pod start** — registers the workspace pod as an SSM managed instance.
2. **Connection creation** — creates an SSM session when a user requests a `vscode-remote` or `cursor-remote` connection.
3. **Pod stop** — deregisters the SSM managed instance.

**Extension API** returns a URL that opens the user's desktop IDE, which connects through the SSM tunnel.

## Installation

Add the plugin as a sidecar in the operator Helm chart's `controller.plugins` list:

```yaml
controller:
  plugins:
    - name: aws
      image:
        repository: ghcr.io/jupyter-infra/jupyter-k8s-aws-plugin
        tag: latest
      port: 8080
      imagePullPolicy: Always
      healthcheckCommand: ["/aws-plugin", "--healthcheck"]
      env:
        PLUGIN_PORT: "8080"
        AWS_REGION: "us-west-2"
```

When the operator chart is configured this way, it creates a sidecar container in the controller pod and registers `http://localhost:<port>` as the plugin endpoint under the given name. Access strategies can then reference the plugin using the `aws:` prefix in handler fields.

For reference, the {ref}`aws-hyperpod <chart-aws-hyperpod>` guided chart configures the **JupyterK8s** chart to use this plugin.

## Requirements

The plugin calls AWS APIs (SSM, STS, etc.) from the controller pod. The pod's service account needs AWS credentials — typically granted via:

- **[EKS Pod Identity](https://docs.aws.amazon.com/eks/latest/userguide/pod-identities.html)** (recommended) — create a pod identity association for the controller's service account.
- **[IRSA](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html)** (IAM Roles for Service Accounts) — annotate the service account with an IAM role ARN.

Both mechanisms share credentials with all containers in the pod, including the plugin sidecar.

## Source and packages

| | |
|---|---|
| Repository | [jupyter-k8s-aws](https://github.com/jupyter-infra/jupyter-k8s-aws) |
| Image | `ghcr.io/jupyter-infra/jupyter-k8s-aws-plugin` |
