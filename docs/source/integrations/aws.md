# AWS

The `jupyter-k8s-aws` [repository](https://github.com/jupyter-infra/jupyter-k8s-aws) provides the AWS plugin and opinionated guided charts for AWS environments.

## Published artifacts

| Package | Registry | Description |
|---------|----------|-------------|
| `jupyter-k8s-aws-plugin` | `ghcr.io/jupyter-infra/jupyter-k8s-aws-plugin` | AWS plugin sidecar — handles SSM session creation, pod registration/deregistration, and JWT operations via AWS APIs |
| `jupyter-k8s-aws-hyperpod` (chart) | `oci://ghcr.io/jupyter-infra/charts/jupyter-k8s-aws-hyperpod` | Guided chart for AWS HyperPod clusters — bundles **Jupyter K8s**, the plugin, and access strategy for SSM-based remote access |
| `jupyter-k8s-aws-oidc` (chart) | `oci://ghcr.io/jupyter-infra/charts/jupyter-k8s-aws-oidc` | Guided chart with Traefik reverse proxy and Dex identity provider — provides full OIDC web access with the auth middleware |

## What the plugin does

The AWS plugin implements remote access via AWS Systems Manager (SSM):

1. On pod start, the plugin registers the workspace pod as an SSM managed instance.
2. When a user creates a `vscode-remote` connection, the plugin creates an SSM session.
3. The Extension API returns a URL that opens VS Code desktop, which connects via the SSM tunnel.
4. On pod stop, the plugin deregisters the instance.

## Guided charts

Guided charts are opinionated Helm charts that bundle everything needed for a specific deployment scenario:

- **aws-hyperpod** — designed for SageMaker HyperPod clusters. Deploys **Jupyter K8s** with the AWS plugin sidecar and preconfigured access strategies for SSM remote access.
- **aws-oidc** — deploys the full stack including Traefik as a reverse proxy, Dex as the identity provider, the auth middleware, and templates for web-based workspace access.

For installation and configuration, see `jupyter-k8s-aws` [repository](https://github.com/jupyter-infra/jupyter-k8s-aws).
