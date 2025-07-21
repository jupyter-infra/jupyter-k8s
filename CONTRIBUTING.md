# Local Development Setup

This directory contains tools for setting up a local development environment for the Jupyter Kubernetes controller.

## Kind Setup

The `kind` directory contains configuration for a local Kubernetes cluster using [Kind](https://kind.sigs.k8s.io/).

### Prerequisites

- [Kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
- [Finch](https://github.com/runfinch/finch) (or Docker)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [Helm](https://helm.sh/docs/intro/install/)

### Setup

To set up the local development environment:

```bash
make local-dev-setup
```

This will:
1. Create a Kind cluster named `jupyter-k8s`
2. Set up a local Docker registry at `localhost:5000`
3. Generate a kubeconfig file at `local-dev/kind/.kubeconfig`

### Usage

After setting up the environment, you can:

1. Build and push the controller image:
   ```bash
   make build
   ```

2. Deploy the controller to the cluster:
   ```bash
   make deploy-local
   ```

3. Try creating a Jupyter notebook instance:
   ```bash
   kubectl --kubeconfig=.kubeconfig apply -f examples/sample-notebook.yaml
   ```

4. Check the status:
   ```bash
   kubectl --kubeconfig=.kubeconfig get JupyterNotebook
   kubectl --kubeconfig=.kubeconfig get JupyterNotebook sample-notebook
   ```

### Teardown

To tear down the local development environment:

```bash
make local-dev-teardown
```

This will delete the Kind cluster and remove the local registry.