# Contributing

This project contains tools for setting up a local development environment for the Jupyter Kubernetes controller.

## Local development

We use the following tools:
- [uv](https://docs.astral.sh/uv/) for package and dependency management (instead of `conda` or `pip`)
- [ruff](https://docs.astral.sh/ruff/) for linting and formatting Python files (replace `flake8`, `black` and `isort`)
- [mypy](https://mypy.readthedocs.io/en/stable/) for Python type checking
- [pytest](https://docs.pytest.org/en/stable/contents.html) for Python unit testing

### Getting started

First, install `uv`, see [guide](https://docs.astral.sh/uv/getting-started/installation/)

Quick setup on MacOs/Linux: `pipx install uv`

Then setup your virtual env and download dependencies
```bash
uv sync
```

### Before you raise a PR

To apply Python formatter and linter, run:
```bash
make fix-all
```

Run type-checking with:
```bash
uv run mypy
```

Run unit tests with:
```bash
uv run pytest
```

Or run all at once with:
```bash
make run-all
```


## End-to-end testing with a local Kubernetes cluster

The goal is to test the custom operator on a local Kubernetes cluster.

We use `kind` to setup and run the local Kubernetes cluster. `kind` enables to run local containers as nodes.
See [documentation](https://kind.sigs.k8s.io/).

Given `docker` is not open source for all users, we use `finch` to run `docker build` commands.
See [documentation](https://github.com/runfinch/finch).

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
3. Generate a kubeconfig file at `.kubeconfig`

### Usage

After setting up the environment, you can:

1. Build and push the controller image:
   ```bash
   make build
   ```

2. Deploy the controller to the cluster:
   ```bash
   make local-deploy
   ```

3. Create a Jupyter notebook instance:
   ```bash
   kubectl --kubeconfig=.kubeconfig apply -f examples/sample-notebook.yaml
   ```

4. Check the status:
   ```bash
   kubectl --kubeconfig=.kubeconfig get JupyterServer
   kubectl --kubeconfig=.kubeconfig get JupyterServer sample-notebook
   ```

5. Delete the notebook instance
   ```bash
   kubectl --kubeconfig=.kubeconfig delete JupyterServer sample-notebook
   kubectl --kubeconfig=.kubeconfig get JupyterServer
   ```

### Apply local changes

To sync local changes to the helm chart running in your local cluster, run again:
```bash
make local-deploy
```

### Teardown

To delete the local Kind cluster and remove the local registry, run:

```bash
make local-dev-teardown
```

## Trouble-shooting

### Local testing

If you tore down your local cluster and have trouble setting it back up, try restarting your laptop or host.