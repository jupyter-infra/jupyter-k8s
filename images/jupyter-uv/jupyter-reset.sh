#!/bin/bash
set -e

echo "Resetting Jupyter environment (uv)..."
export UV_PROJECT_ENVIRONMENT="/home/jovyan/.venv"

if [ -d "$UV_PROJECT_ENVIRONMENT" ]; then
    echo "Removing existing virtual environment..."
    rm -rf "$UV_PROJECT_ENVIRONMENT"
fi

if [ -f "/home/jovyan/pyproject.toml" ]; then
    echo "Removing existing pyproject.toml..."
    rm -f /home/jovyan/pyproject.toml
fi

if [ -f "/home/jovyan/uv.lock" ]; then
    echo "Removing existing uv.lock..."
    rm -f /home/jovyan/uv.lock
fi

if [ -d "/home/jovyan/.jupyter" ]; then
    echo "Cleaning Jupyter config..."
    rm -rf /home/jovyan/.jupyter
fi

cp /opt/uv/jupyter/pyproject.toml /home/jovyan/
cp /opt/uv/jupyter/uv.lock /home/jovyan/

echo "Restoring persistent uv environment from the image..."
# Copy as jovyan so the persistent environment stays writable at runtime.
cp -R /opt/uv/jupyter/.venv "$UV_PROJECT_ENVIRONMENT"

echo "Syncing uv environment..."
uv sync --locked

uv run jupyter lab \
    --no-browser \
    --ip=0.0.0.0 \
    --IdentityProvider.token=
