# AWS Charts

The `jupyter-k8s-aws` [GitHub repository](https://github.com/jupyter-infra/jupyter-k8s-aws) publishes guided charts for AWS environments.

(chart-aws-oidc)=
## AWS-OIDC

A full web-access stack for EKS clusters. Users access the applications running in their workspaces from their browser via OIDC authentication.

**What it deploys:**
- [Traefik](https://doc.traefik.io/traefik/) with [Let's Encrypt](https://letsencrypt.org/) issued TLS certificates
- [Dex](https://dexidp.io/docs/getting-started/) as the OIDC identity provider
- [OAuth2-Proxy](https://oauth2-proxy.github.io/oauth2-proxy/) to manage workspace-wide cookies
- **Auth middleware** configured to verify the OIDC token from `dex`
- Preconfigured access strategies and templates for browser-based workspace access

**Access pattern:**
- {ref}`OIDC web access<web-access-oidc-flow>` — users authenticate through Dex, **Auth middleware** issues session cookies, and Traefik proxies traffic to workspace pods.

| Type | URL |
|---|---|
| Package | `oci://ghcr.io/jupyter-infra/charts/jupyter-k8s-aws-oidc` |
| Source | [charts/aws-oidc](https://github.com/jupyter-infra/jupyter-k8s-aws/tree/main/charts/aws-oidc) |

(chart-aws-hyperpod)=
## AWS-HyperPod

Designed for SageMaker HyperPod clusters.

Users connect either a/ from their web browser using bearer token URL or b/ from desktop IDEs (Code Editor) using bearer token VS Code URL.

**What it deploys:**
- **Jupyter K8s** controller with the [AWS Plugin](../plugins/aws-plugin) sidecar
- Preconfigured access strategies for SSM-based remote access (VS Code, Cursor)

Optionally, components for Web UI with:
- [Traefik](https://doc.traefik.io/traefik/)
- An AWS ALB with ACM-issued TLS certificates
- **Auth middleware** configured to accept bearer token URLs
    
**Access patterns:**
- [Desktop IDE remote access](../../concepts/connections/remote-access.md) — the [AWS Plugin](../plugins/aws-plugin.md) registers workspace pods as SSM managed instances, and users connect through SSM sessions from their desktop IDE.
- {ref}`Bearer token web access<web-access-bearer-token-flow>` - users generate a bearer token URL with [connection](../../concepts/connections/index.md), **Auth middleware** issues session cookies, and Traefik proxies traffic to workspace pods.

| Type | URL |
|---|---|
| Package | `oci://ghcr.io/jupyter-infra/charts/jupyter-k8s-aws-hyperpod` |
| Source | [charts/aws-hyperpod](https://github.com/jupyter-infra/jupyter-k8s-aws/tree/main/charts/aws-hyperpod) |

## Dive deeper

For full configuration options, see the `jupyter-k8s-aws` [GitHub repository](https://github.com/jupyter-infra/jupyter-k8s-aws).
