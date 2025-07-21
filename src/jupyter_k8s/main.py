"""Main entrypoint for the Jupyter K8s controller."""

import asyncio
import logging

import kopf
import uvicorn
from fastapi import FastAPI

from jupyter_k8s.api.router import api_router

# Set up logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("jupyter-k8s")


# Initialize FastAPI app
app = FastAPI(
    title="Jupyter K8s Controller",
    description="Kubernetes controller for managing Jupyter notebooks",
    version="0.1.0",
)

# Add API routes
app.include_router(api_router)


@kopf.on.startup()
def configure(*_, **__) -> None:
    """Configure kopf operator settings."""
    logger.info("Starting Jupyter K8s Controller")
    logger.info("This is a minimal implementation that only logs events")


# Define the entry point
def run() -> None:
    """Run both kopf operator and FastAPI server in a single process."""
    loop = asyncio.get_event_loop()

    # Run kopf operator
    kopf_task = loop.create_task(
        kopf.operator(
            clusterwide=True,
            standalone=True,
        )
    )

    # Run FastAPI server
    config = uvicorn.Config(app=app, host="0.0.0.0", port=8000)
    server = uvicorn.Server(config)
    api_task = loop.create_task(server.serve())

    # Run both tasks
    loop.run_until_complete(asyncio.gather(kopf_task, api_task))


if __name__ == "__main__":
    run()
