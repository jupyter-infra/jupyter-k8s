# JupyterLab

[JupyterLab](https://jupyterlab.readthedocs.io/) is an extensible web-based IDE for notebooks, code, and data. See the project source code on [GitHub](https://github.com/jupyterlab/jupyterlab).

You can run JupyterLab applications inside a **Jupyter K8s** workspace.

**Jupyter K8s** handles the lifecycle of the workspace — it's the compute, storage, and routing — but you are responsible for the JupyterLab image you use.

## Image requirements

Your JupyterLab image must:

1. **Listen on `0.0.0.0`** — the pod service routes traffic to the container port, so the server cannot bind to `localhost` only.
2. **Expose a known port** — the default is `8888`.

For web access through a reverse proxy, also:

3. **Respect `JUPYTER_BASE_URL`** — when routing uses path-based routing, each workspace gets a unique path prefix. Pass it as `--ServerApp.base_url`.
4. **Disable token authentication** — the routing components handle admission externally. Set `--IdentityProvider.token=` (empty value).

A minimal start command:

```bash
jupyter lab \
  --no-browser \
  --ip=0.0.0.0 \
  --ServerApp.base_url="${JUPYTER_BASE_URL:-/}" \
  --IdentityProvider.token=
```

## Reference image

The repository includes a reference image at `images/jupyter-uv/` that demonstrates these requirements. It uses [uv](https://docs.astral.sh/uv/) for dependency management and installs JupyterLab with the `jupyter-server-documents` extension.

## Workspace manifest

```yaml
apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: my-jupyterlab-app
spec:
  displayName: My JupyterLab App
  image: <your-repository>/<your-jupyterlab-image>:<your-tag>
  desiredStatus: Running
  resources:
    requests:
      cpu: "200m"
      memory: "256Mi"
    limits:
      cpu: "1"
      memory: "1Gi"
  storage:
    size: "10Gi"
    mountPath: "/home/jovyan"
```

## Access methods

### Port forward

The simplest method — no routing infrastructure required:

```bash
kubectl port-forward svc/my-jupyterlab-app 8888:8888
```

Open `http://localhost:8888` in your browser.

### Web access (OIDC)

When an access strategy with a reverse proxy and identity provider is configured, users navigate directly to the workspace URL.

See {ref}`Concepts: Web Access OIDC<web-access-oidc-flow>` for details.

### Web access (Bearer token)

For environments without an IdP, or for sharing a workspace URL, **Jupyter K8s** can generate bearer-token URLs.

See {ref}`Concepts: Web Access Bearer Token<web-access-bearer-token-flow>` for details.
