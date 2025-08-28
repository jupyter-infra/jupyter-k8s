# jupyter-k8s
CRD and controller to deploy Jupyter server app to Kubernetes pod

## Quick Start

### Development
```bash
# Build and deploy locally
make local-deploy

# Quick rebuild during development
make dev-restart

# Check status and logs
make status
make logs
```

### Usage
```yaml
apiVersion: servers.jupyter.org/v1alpha1
kind: JupyterServer
metadata:
  name: my-notebook
spec:
  desiredStatus: Running  # or "Stopped"
  image: jupyter/base-notebook:latest
  resources:
    requests:
      cpu: 200m
      memory: 512Mi
```
