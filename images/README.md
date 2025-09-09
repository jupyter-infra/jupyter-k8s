# JupyterServer Images

This directory contains Dockerfile definitions for the built-in JupyterServer images used by the Jupyter K8s operator.

## Available Images

- **jupyter-uv**: Jupyter with the [UV package manager](https://github.com/astral-sh/uv)

## Building Images

You can build all images using the provided Makefile:

```bash
# Build all images
make all

# Build a specific image
make jupyter-uv

# Push all images to the registry
make push-all

# Clean up local images
make clean
```

## Using Images in JupyterServer CRDs

The JupyterServer operator supports using these images either by their shortcut name or by full image path:

```yaml
# Using image shortcut
apiVersion: servers.jupyter.org/v1alpha1
kind: JupyterServer
metadata:
  name: jupyter-sample
spec:
  name: jupyter-sample
  image: uv  # Uses the built-in UV image
```

```yaml
# Using full image path
apiVersion: servers.jupyter.org/v1alpha1
kind: JupyterServer
metadata:
  name: jupyter-sample
spec:
  name: jupyter-sample
  image: localhost:5000/jupyter-uv:latest
```

## Adding New Images

To add a new image variant:

1. Create a new directory for your image (e.g., `images/jupyter-newvariant/`)
2. Create a Dockerfile for your image
3. Add the image to the `IMAGES` list in the Makefile
4. Add the image to the `images.go` constants file

## Customizing Images

Each image can be customized by modifying its Dockerfile to include the packages and configuration you need.

## Registry Configuration

By default, images are built for the `localhost:5000` registry (for local development with kind). You can override this by setting the `REGISTRY` environment variable:

```bash
REGISTRY=my-registry.example.com make all
```