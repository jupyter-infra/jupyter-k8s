#!/bin/bash
set -e

BASE_URL="${JUPYTER_BASE_URL:-/}"
export UV_PROJECT_ENVIRONMENT="/home/jovyan/.venv"
# Keep copied entry points valid after moving the image environment into home.
export UV_PREVIEW_FEATURES="relocatable-envs-default"

echo "Setting up uv environment..."

if [ ! -f "/home/jovyan/pyproject.toml" ] || [ ! -f "/home/jovyan/uv.lock" ]; then
    echo "Did not find uv environment files in /home/jovyan."
    cp -n /opt/uv/jupyter/pyproject.toml /home/jovyan/
    cp -n /opt/uv/jupyter/uv.lock /home/jovyan/
else
    echo "Found existing uv environment files, syncing..."
fi

if [ ! -d "$UV_PROJECT_ENVIRONMENT" ]; then
    echo "Seeding persistent uv environment from the image..."
    # Copy as jovyan so the persistent environment stays writable at runtime.
    cp -R /opt/uv/jupyter/.venv "$UV_PROJECT_ENVIRONMENT"
fi

uv sync --locked

# Disable exit on error for the jupyter lab attempt
set +e
uv run jupyter lab \
    --no-browser \
    --ip=0.0.0.0 \
    --IdentityProvider.token= \
    --ServerApp.base_url="$BASE_URL"

# captures jupyterlab exit code
jupyter_exit_code=$?
set -e

# Check if jupyter lab failed
if [ $jupyter_exit_code -ne 0 ]; then
    echo "Jupyter lab failed to start, calling reset script..."
    /usr/local/bin/jupyter-reset.sh
fi
