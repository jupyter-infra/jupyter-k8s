# Jupyter K8s

[![Documentation](https://readthedocs.org/projects/jupyter-k8s/badge/?version=latest)](https://jupyter-k8s.readthedocs.io/en/latest/)
[![Tests](https://github.com/jupyter-infra/jupyter-k8s/actions/workflows/test.yml/badge.svg)](https://github.com/jupyter-infra/jupyter-k8s/actions/workflows/test.yml)
[![E2E Tests](https://github.com/jupyter-infra/jupyter-k8s/actions/workflows/test-e2e.yml/badge.svg)](https://github.com/jupyter-infra/jupyter-k8s/actions/workflows/test-e2e.yml)

A Kubernetes operator for Jupyter notebooks and interactive IDEs — managing compute, storage, networking, and access control for multiple users.

- **Kubernetes Native** — Workspaces are native Kubernetes resources. Your users manage and access them using their Kubernetes identities and RBAC policies.
- **Multi-Application Support** — Run JupyterLab, VS Code, or bring your own apps. Persistent per-user storage with optional shared volumes for team collaboration.
- **Secure by Default** — Scope a workspace access to a single user or a team. Namespace-scoped RBAC, JWT-based authentication with automatic key rotation.
- **Flexible Access** — Connect to your workspaces from your web browser with OAuth 2 or bearer token URL, or directly from your desktop IDE.
- **Fine-Grained Control** — Provide default configurations to your users and enforce bounds with templates. Automatically shutdown idle workspaces.
- **Vendor Neutral** — Compatible with any cloud provider via an HTTP sidecar plugin pattern. Bring your own integration.

## Documentation

https://jupyter-k8s.readthedocs.io

## Installation

```bash
helm install jupyter-k8s oci://ghcr.io/jupyter-infra/charts/jupyter-k8s \
  --namespace jupyter-k8s-system \
  --create-namespace
```

See the [Getting Started](https://jupyter-k8s.readthedocs.io/en/latest/getting-started/) guide for prerequisites, configuration, and next steps.

## Packages

| Artifact | Description |
|----------|-------------|
| [`jupyter-k8s`](https://github.com/jupyter-infra/jupyter-k8s/pkgs/container/charts%2Fjupyter-k8s) Helm chart | Operator chart — CRDs, controller, webhooks, RBAC, extension API server. |
| [`jupyter-k8s-controller`](https://github.com/jupyter-infra/jupyter-k8s/pkgs/container/jupyter-k8s-controller) image | Controller and Extension API server. |
| [`jupyter-k8s-authmiddleware`](https://github.com/jupyter-infra/jupyter-k8s/pkgs/container/jupyter-k8s-authmiddleware) image | JWT auth middleware for reverse-proxy deployments. |
| [`jupyter-k8s-rotator`](https://github.com/jupyter-infra/jupyter-k8s/pkgs/container/jupyter-k8s-rotator) image | HMAC key rotation CronJob. |

For cloud-specific guided charts, see [jupyter-k8s-aws](https://github.com/jupyter-infra/jupyter-k8s-aws).

## Contributing

Refer to the [Contributing guide](./CONTRIBUTING.md).

## License

This project is licensed under the [MIT License](LICENSE).

