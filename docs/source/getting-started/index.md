# Getting Started

Install **Jupyter K8s** into an existing Kubernetes cluster using Helm.

## Prerequisites

- Kubernetes cluster (v1.28+)
- Helm (v3.12+)
- `kubectl` configured to access the cluster

## Install the chart

The **Jupyter K8s** chart is published as an OCI artifact on GitHub Container Registry.

```bash
helm install jupyter-k8s oci://ghcr.io/jupyter-infra/charts/jupyter-k8s \
  --namespace jupyter-k8s-system \
  --create-namespace
```

This installs the controller, extension API server, and CRDs into the `jupyter-k8s-system` namespace.

## Verify the installation

```bash
kubectl get pods -n jupyter-k8s-system
```

You should see the controller pod running:

```text
NAME                                    READY   STATUS    RESTARTS   AGE
jupyter-k8s-controller-manager-xxxxx    1/1     Running   0          30s
```

Confirm the CRDs are registered:

```bash
kubectl get crds | grep workspace.jupyter.org
```

```text
workspaces.workspace.jupyter.org                 2025-01-01T00:00:00Z
workspacetemplates.workspace.jupyter.org         2025-01-01T00:00:00Z
workspaceaccessstrategies.workspace.jupyter.org  2025-01-01T00:00:00Z
```

## Configuration

The chart exposes configuration via `values.yaml`. Common options:

```bash
helm install jupyter-k8s oci://ghcr.io/jupyter-infra/charts/jupyter-k8s \
  --namespace jupyter-k8s-system \
  --create-namespace \
  --set manager.resources.limits.memory=512Mi
```

See the [Helm Chart Values](../reference/helm-charts/operator) reference for all available options.

## Bring your applications

**Jupyter K8s** orchestrates compute, storage, networking, and access control — but does not ship application images. You bring your own container images (JupyterLab, VS Code, or any HTTP-serving application) and reference them in `workspace.spec.image`.

See [Applications](../applications/index) for image requirements and configuration guides.

## Uninstall

```bash
helm uninstall jupyter-k8s -n jupyter-k8s-system
```

By default, Helm retains CRDs. To fully remove them from your cluster, run:

```bash
kubectl delete crds \
  workspaces.workspace.jupyter.org \
  workspacetemplates.workspace.jupyter.org \
  workspaceaccessstrategies.workspace.jupyter.org
```

```{toctree}
:hidden:

run-workspaces
```
