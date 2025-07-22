"""Notebook controller handlers for the Jupyter K8s operator."""

import logging

import kopf

# Set up logger
logger = logging.getLogger("jupyter-k8s")


@kopf.on.create("servers.jupyter.org", "v1alpha1", "jupyterservers")
def create_notebook(*_, **kwargs) -> dict[str, str]:
    """Handle creation of a Jupyter server resource."""
    name = kwargs.get("name") or kwargs.get("meta", {}).get("name", "unknown")
    logger.info(f"Received create event for JupyterServer: {name}")
    logger.info("This is a minimal implementation that does not create any resources.")
    return {"status": "Acknowledged", "message": "Create request logged (no action taken)"}


@kopf.on.delete("servers.jupyter.org", "v1alpha1", "jupyterservers")
def delete_notebook(*_, **kwargs) -> None:
    """Handle deletion of a Jupyter server resource."""
    name = kwargs.get("name") or kwargs.get("meta", {}).get("name", "unknown")
    logger.info(f"Received delete event for JupyterServer: {name}")
    logger.info("This is a minimal implementation that does not delete any resources.")
