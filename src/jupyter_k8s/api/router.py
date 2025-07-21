"""API router for the Jupyter K8s controller."""

import logging
from datetime import datetime

from fastapi import APIRouter, HTTPException

from jupyter_k8s.models.notebook import NotebookCreate, NotebookResponse

# Set up logger
logger = logging.getLogger("jupyter-k8s")

# Create router
api_router = APIRouter(prefix="/api/v1")


@api_router.get("/notebooks", response_model=list[NotebookResponse])
async def list_notebooks() -> list[NotebookResponse]:
    """List all Jupyter notebooks."""
    logger.info("Received request to list notebooks")
    # Return a single sample notebook
    return [
        NotebookResponse(
            name="sample-notebook",
            namespace="default",
            created_at=datetime.now(),
            status="Running",
            image="jupyter/minimal-notebook:latest",
        )
    ]


@api_router.get("/notebooks/{name}", response_model=NotebookResponse)
async def get_notebook(name: str) -> NotebookResponse:
    """Get details for a specific Jupyter notebook."""
    logger.info(f"Received request to get notebook: {name}")

    if name != "sample-notebook":
        raise HTTPException(status_code=404, detail="Notebook not found")

    return NotebookResponse(
        name=name,
        namespace="default",
        created_at=datetime.now(),
        status="Running",
        image="jupyter/minimal-notebook:latest",
    )


@api_router.post("/notebooks", response_model=NotebookResponse, status_code=201)
async def create_notebook(notebook: NotebookCreate) -> NotebookResponse:
    """Create a new Jupyter notebook."""
    logger.info(f"Received request to create notebook: {notebook.name}")

    # Log the request but don't actually create anything
    return NotebookResponse(
        name=notebook.name,
        namespace="default",
        created_at=datetime.now(),
        status="Creating",
        image="jupyter/minimal-notebook:latest",
    )


@api_router.delete("/notebooks/{name}", status_code=204)
async def delete_notebook(name: str) -> None:
    """Delete a Jupyter notebook."""
    logger.info(f"Received request to delete notebook: {name}")

    if name != "sample-notebook":
        raise HTTPException(status_code=404, detail="Notebook not found")
