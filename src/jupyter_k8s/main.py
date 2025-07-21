"""Main entrypoint for the Jupyter K8s controller."""

import asyncio
import logging

import kopf

# Set up logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("jupyter-k8s")


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

    # Run both tasks
    loop.run_until_complete(kopf_task)


if __name__ == "__main__":
    run()
