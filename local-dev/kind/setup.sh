#!/bin/bash

set -e

# Use Finch as the container provider for KinD
export KIND_EXPERIMENTAL_PROVIDER=finch

# Create kind cluster if it doesn't exist
if ! kind get clusters | grep -q jupyter-k8s; then
  echo "Creating kind cluster..."
  kind create cluster --config=local-dev/kind/cluster.yaml --name=jupyter-k8s
else
  echo "Kind cluster already exists."
fi

# Set up local registry if it's not running
if ! finch ps | grep -q "registry:2"; then
  echo "Setting up local Docker registry..."
  finch run -d --restart=always -p 5000:5000 --name registry registry:2
else
  echo "Local registry is already running."
fi

echo "Environment setup complete."