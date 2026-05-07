# AWS Charts

The `jupyter-k8s-aws` [repository](https://github.com/jupyter-infra/jupyter-k8s-aws) publishes guided charts for AWS environments.

(chart-aws-oidc)=
## AWS-OIDC

A full web-access stack for EKS clusters. Users access the applications running in their workspaces from their browser via OIDC authentication.

**What it deploys:**
- Traefik as the reverse proxy (with AWS ALB ingress)
- Dex as the OIDC identity provider
- **Auth middleware** for session management
- Preconfigured access strategies and templates for browser-based workspace access

**Access pattern:** OIDC web access — users authenticate through Dex, **Auth middleware** issues session cookies, and Traefik proxies traffic to workspace pods.

**Package:** `oci://ghcr.io/jupyter-infra/charts/jupyter-k8s-aws-oidc`

**Source:** [charts/aws-oidc](https://github.com/jupyter-infra/jupyter-k8s-aws/tree/main/charts/aws-oidc)

(chart-aws-hyperpod)=
## AWS-HyperPod

Designed for SageMaker HyperPod clusters.

Users connect either a/ from their web browser using bearer token URL or b/ from desktop IDEs (Code Editor) using bearer token VS Code URL.

**What it deploys:**
- **Jupyter K8s** controller with the [AWS plugin](../plugins/aws-plugin) sidecar
- Preconfigured access strategies for SSM-based remote access (VS Code, Cursor)
- Optional web UI with Traefik, ALB, and bearer-token URL

**Access pattern:** Remote access via SSM — the plugin registers workspace pods as SSM managed instances, and users connect through SSM sessions from their desktop IDE.

**Package:** `oci://ghcr.io/jupyter-infra/charts/jupyter-k8s-aws-hyperpod`

**Source:** [charts/aws-hyperpod](https://github.com/jupyter-infra/jupyter-k8s-aws/tree/main/charts/aws-hyperpod)

## Dive deeper

For full configuration options, see the [jupyter-k8s-aws repository](https://github.com/jupyter-infra/jupyter-k8s-aws).
