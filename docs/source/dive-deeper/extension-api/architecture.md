# Architecture

**Extension API** server runs as part of the controller manager pod — not as a separate deployment.

## Deployment

The **Jupyter K8s** Helm chart deploys a single manager pod in the `jupyter-k8s-system` namespace. This pod runs:
- The controller (reconciliation loops)
- The **Extension API** server (Connection APIs)

**Extension API** is a [`GenericAPIServer`](https://pkg.go.dev/k8s.io/apiserver/pkg/server#GenericAPIServer) instance added to the controller-runtime manager as a `Runnable`. It starts alongside the controller and shares the same lifecycle.

## TLS and API aggregation

**Extension API** serves over TLS (port `7443` by default) and registers with the Kubernetes API server via an `APIService` resource. This makes its endpoints available through the standard K8s API path:

```
/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/{namespace}/{resource}
```

Clients use their existing kubeconfig credentials — the K8s API server proxies requests and provides authentication context via request headers.

## Configuration

Key settings (set via Helm values under `extensionApi`):

| Setting | Default | Description |
|---------|---------|-------------|
| `enable` | `false` | Enable the Extension API server |
| `serverPort` | `7443` | TLS listen port |
| `jwtSecret.enable` | `false` | Enable k8s-native JWT signing (creates a Secret and rotator CronJob) |
| `jwtSecret.secretName` | `extensionapi-jwt-secrets` | Name of the HMAC signing Secret |

## Plugin endpoints

When the Helm chart configures [plugins](../../integrations/plugins/index.md), **Extension API** creates HTTP clients for each plugin endpoint:

```yaml
controller:
  plugins:
    aws: "http://localhost:8080"
```

These clients are shared across JWT signing and connection creation paths.
