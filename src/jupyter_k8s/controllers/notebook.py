"""Notebook controller handlers for the Jupyter K8s operator."""

import logging
from typing import Any

import kopf
from kubernetes import client  # type: ignore
from kubernetes.client.rest import ApiException  # type: ignore
from pydantic import ValidationError

from ..models import JupyterServerSpec

# Set up logger
logger = logging.getLogger("jupyter-k8s")

# Create module-level client instances
apps_v1 = client.AppsV1Api()
core_v1 = client.CoreV1Api()


def handle_create_notebook(spec: dict[str, Any], name: str, namespace: str, meta: dict[str, Any]) -> dict[str, Any]:
    """Handle creation of a Jupyter notebook server."""

    try:
        # Validate and parse spec
        jupyter_spec = JupyterServerSpec(**spec)
    except ValidationError as e:
        logger.error(f"Invalid spec for {name}: {e}")
        return {
            "status": {
                "phase": "Failed",
                "error": str(e),
                "message": "Invalid spec",
            }
        }

    try:
        # Create Deployment
        deployment = create_jupyter_deployment(name, namespace, jupyter_spec)
        logger.info(f"Creating deployment: jupyter-{name}")

        deployment_result = apps_v1.create_namespaced_deployment(namespace=namespace, body=deployment)
        deployment_name = deployment_result.metadata.name if deployment_result.metadata else f"jupyter-{name}"
        logger.info(f"Deployment created successfully: {deployment_name}")

        # Create Service
        service = create_jupyter_service(name, namespace)
        logger.info(f"Creating service: jupyter-{name}-service")

        service_result = core_v1.create_namespaced_service(namespace=namespace, body=service)
        service_name = service_result.metadata.name if service_result.metadata else f"jupyter-{name}-service"
        logger.info(f"Service created successfully: {service_name}")

        # Return status update
        return {
            "status": {
                "phase": "Running",
                "deploymentName": deployment_name,
                "serviceName": service_name,
                "accessInstructions": {
                    "portForward": f"kubectl port-forward service/{service_name} 8888:8888 -n {namespace}",
                    "url": "http://localhost:8888",
                },
            }
        }

    except ApiException as e:
        logger.error(f"Failed to create Jupyter server: {e}")
        return {
            "status": {
                "phase": "Failed",
                "error": str(e),
                "message": "Failed to create resources",
            }
        }


def handle_delete_notebook(name: str, namespace: str) -> None:
    """Handle deletion of a Jupyter notebook server."""

    try:
        deployment_name = f"jupyter-{name}"
        service_name = f"jupyter-{name}-service"

        # Delete deployment
        try:
            apps_v1.delete_namespaced_deployment(name=deployment_name, namespace=namespace)
            logger.info(f"Deployment {deployment_name} deleted successfully")
        except ApiException as e:
            if e.status != 404:  # Ignore not found errors
                logger.warning(f"Failed to delete deployment {deployment_name}: {e}")

        # Delete service
        try:
            core_v1.delete_namespaced_service(name=service_name, namespace=namespace)
            logger.info(f"Service {service_name} deleted successfully")
        except ApiException as e:
            if e.status != 404:  # Ignore not found errors
                logger.warning(f"Failed to delete service {service_name}: {e}")

    except Exception as e:
        logger.error(f"Failed to delete Jupyter server resources: {e}")


def handle_update_notebook(
    spec: dict[str, Any], name: str, namespace: str, old: dict[str, Any], new: dict[str, Any]
) -> dict[str, Any] | None:
    """Handle update of a Jupyter notebook server."""

    # Check if spec has actually changed
    old_spec = old.get("spec", {})
    new_spec = new.get("spec", {})

    if old_spec == new_spec:
        logger.info(f"No spec changes detected for {name}, skipping update")
        return None

    try:
        # Validate and parse spec
        jupyter_spec = JupyterServerSpec(**spec)
    except ValidationError as e:
        logger.error(f"Invalid spec for {name}: {e}")
        return {
            "status": {
                "phase": "Failed",
                "error": str(e),
                "message": "Invalid spec",
            }
        }

    try:
        deployment_name = f"jupyter-{name}"

        # Update deployment
        deployment = create_jupyter_deployment(name, namespace, jupyter_spec)
        logger.info(f"Updating deployment: {deployment_name}")

        deployment_result = apps_v1.patch_namespaced_deployment(
            name=deployment_name, namespace=namespace, body=deployment
        )
        deployment_name_result = deployment_result.metadata.name if deployment_result.metadata else deployment_name
        logger.info(f"Deployment updated successfully: {deployment_name_result}")

        return {
            "status": {
                "phase": "Running",
                "deploymentName": deployment_name,
                "serviceName": f"jupyter-{name}-service",
            }
        }

    except ApiException as e:
        logger.error(f"Failed to update Jupyter server: {e}")
        return {
            "status": {
                "phase": "Failed",
                "error": str(e),
            }
        }


@kopf.on.create("servers.jupyter.org", "v1alpha1", "jupyterservers")
def create_notebook(spec, name, namespace, meta, **kwargs):
    """Kopf handler for creating Jupyter notebook servers."""
    return handle_create_notebook(spec, name, namespace, meta)


@kopf.on.delete("servers.jupyter.org", "v1alpha1", "jupyterservers")
def delete_notebook(name, namespace, **kwargs):
    """Kopf handler for deleting Jupyter notebook servers."""
    handle_delete_notebook(name, namespace)


@kopf.on.update("servers.jupyter.org", "v1alpha1", "jupyterservers")
def update_notebook(spec, name, namespace, old, new, **kwargs):
    """Kopf handler for updating Jupyter notebook servers."""
    return handle_update_notebook(spec, name, namespace, old, new)


def create_jupyter_deployment(name: str, namespace: str, jupyter_spec: JupyterServerSpec) -> client.V1Deployment:
    """Create a Kubernetes deployment for Jupyter server."""

    image = jupyter_spec.image
    resources = jupyter_spec.resources.dict() if jupyter_spec.resources else {}

    # Create container
    container = client.V1Container(
        name="jupyter",
        image=image,
        ports=[client.V1ContainerPort(container_port=8888, name="jupyter")],
        env=[
            client.V1EnvVar(name="JUPYTER_ENABLE_LAB", value="yes"),
            client.V1EnvVar(name="JUPYTER_TOKEN", value=""),
        ],
        liveness_probe=client.V1Probe(
            http_get=client.V1HTTPGetAction(path="/api", port=8888),
            initial_delay_seconds=30,
            period_seconds=10,
        ),
        readiness_probe=client.V1Probe(
            http_get=client.V1HTTPGetAction(path="/api", port=8888),
            initial_delay_seconds=5,
            period_seconds=5,
        ),
    )

    # Add resources if specified
    if resources:
        container.resources = client.V1ResourceRequirements(**resources)

    # Create pod template
    template = client.V1PodTemplateSpec(
        metadata=client.V1ObjectMeta(
            labels={
                "app": "jupyter-server",
                "instance": name,
            }
        ),
        spec=client.V1PodSpec(containers=[container]),
    )

    # Create deployment spec
    deployment_spec = client.V1DeploymentSpec(
        replicas=1,
        selector=client.V1LabelSelector(
            match_labels={
                "app": "jupyter-server",
                "instance": name,
            }
        ),
        template=template,
    )

    # Create deployment
    deployment = client.V1Deployment(
        api_version="apps/v1",
        kind="Deployment",
        metadata=client.V1ObjectMeta(
            name=f"jupyter-{name}",
            namespace=namespace,
            labels={
                "app": "jupyter-server",
                "instance": name,
            },
        ),
        spec=deployment_spec,
    )

    return deployment


def create_jupyter_service(name: str, namespace: str) -> client.V1Service:
    """Create a Kubernetes service for Jupyter server."""
    return client.V1Service(
        api_version="v1",
        kind="Service",
        metadata=client.V1ObjectMeta(
            name=f"jupyter-{name}-service",
            namespace=namespace,
            labels={
                "app": "jupyter-server",
                "instance": name,
            },
        ),
        spec=client.V1ServiceSpec(
            selector={
                "app": "jupyter-server",
                "instance": name,
            },
            ports=[
                client.V1ServicePort(
                    port=8888,
                    target_port=8888,
                    name="jupyter",
                )
            ],
            type="ClusterIP",
        ),
    )
