# Packages

**Jupyter K8s** publishes the following artifacts to GitHub Container Registry (`ghcr.io/jupyter-infra`).

## Container images

| Image | Description |
|-------|-------------|
| `ghcr.io/jupyter-infra/jupyter-k8s-controller` | Controller and Extension API server. Manages workspace resources and serves the Connection APIs. |
| `ghcr.io/jupyter-infra/jupyter-k8s-authmiddleware` | JWT auth middleware. Deployed alongside a reverse proxy to authorize workspace requests. |
| `ghcr.io/jupyter-infra/jupyter-k8s-rotator` | HMAC key rotation to run in a CronJob. Rotates the signing keys used by the controller and auth middleware. |

## Helm chart

| Chart | OCI URI | Description |
|-------|---------|-------------|
| `jupyter-k8s` | `oci://ghcr.io/jupyter-infra/charts/jupyter-k8s` | Operator chart — deploys CRDs, controller, webhooks, RBAC, extension API server, JWT secret and CronJob to rotate it. |

Install with:

```bash
helm install jupyter-k8s oci://ghcr.io/jupyter-infra/charts/jupyter-k8s \
  --namespace jupyter-k8s-system \
  --create-namespace
```

## Versioning

All packages in a release share the same version tag (e.g. `v0.2.0`). The chart version uses the semver number without the `v` prefix (e.g. `0.2.0`).

Images are also tagged with:
- `sha-<short-sha>` — for pinning to a specific commit
- `latest` — the most recent release
