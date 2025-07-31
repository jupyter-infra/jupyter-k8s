"""Tests for the Jupyter K8s server controller."""

import unittest
from unittest.mock import Mock, patch

from kubernetes.client.rest import ApiException  # type: ignore

from jupyter_k8s.controllers.notebook import (
    create_jupyter_deployment,
    create_jupyter_service,
    handle_create_notebook,
    handle_delete_notebook,
    handle_update_notebook,
)
from jupyter_k8s.models import JupyterServerSpec


class TestCreateNotebook(unittest.TestCase):
    """Test cases for notebook creation."""

    def setUp(self) -> None:
        """Set up test fixtures."""
        self.mock_spec = {
            "name": "comprehensive-notebook-server",
            "desiredStatus": "Running",
            "image": "jupyter/scipy-notebook:latest",
            "serviceAccountName": "default",
            "resources": {
                "requests": {"memory": "512Mi", "cpu": "250m"},
                "limits": {"memory": "1Gi", "cpu": "500m"},
            },
        }
        self.mock_meta = {"name": "test-notebook", "namespace": "default"}

    @patch("jupyter_k8s.controllers.notebook.apps_v1")
    @patch("jupyter_k8s.controllers.notebook.core_v1")
    @patch("jupyter_k8s.controllers.notebook.logger")
    def test_create_notebook_success(self, mock_logger, mock_core_v1, mock_apps_v1):
        """Test successful creation of a Jupyter server."""
        # Arrange
        mock_deployment_result = Mock()
        mock_deployment_result.metadata.name = "jupyter-test-notebook"
        mock_apps_v1.create_namespaced_deployment.return_value = mock_deployment_result

        mock_service_result = Mock()
        mock_service_result.metadata.name = "jupyter-test-notebook-service"
        mock_core_v1.create_namespaced_service.return_value = mock_service_result

        # Act
        result = handle_create_notebook(
            spec=self.mock_spec,
            name="test-notebook",
            namespace="default",
            meta=self.mock_meta,
        )

        # Assert
        self.assertEqual(result["status"]["phase"], "Running")
        self.assertEqual(result["status"]["deploymentName"], "jupyter-test-notebook")
        self.assertEqual(result["status"]["serviceName"], "jupyter-test-notebook-service")
        self.assertEqual(
            result["status"]["accessInstructions"]["portForward"],
            "kubectl port-forward service/jupyter-test-notebook-service 8888:8888 -n default",
        )
        self.assertEqual(result["status"]["accessInstructions"]["url"], "http://localhost:8888")

        # Verify API calls
        mock_apps_v1.create_namespaced_deployment.assert_called_once()
        mock_core_v1.create_namespaced_service.assert_called_once()

        # Verify logging
        mock_logger.info.assert_any_call("Creating deployment: jupyter-test-notebook")
        mock_logger.info.assert_any_call("Creating service: jupyter-test-notebook-service")

    @patch("jupyter_k8s.controllers.notebook.apps_v1")
    @patch("jupyter_k8s.controllers.notebook.core_v1")
    @patch("jupyter_k8s.controllers.notebook.logger")
    def test_create_notebook_deployment_failure(self, mock_logger, mock_core_v1, mock_apps_v1):
        """Test handling of deployment creation failure."""
        # Arrange
        mock_apps_v1.create_namespaced_deployment.side_effect = ApiException(status=400, reason="Bad Request")

        # Act
        result = handle_create_notebook(
            spec=self.mock_spec,
            name="test-notebook",
            namespace="default",
            meta=self.mock_meta,
        )

        # Assert
        self.assertEqual(result["status"]["phase"], "Failed")
        self.assertIn("error", result["status"])
        self.assertEqual(result["status"]["message"], "Failed to create resources")

        # Verify service creation was not attempted
        mock_core_v1.create_namespaced_service.assert_not_called()

        # Verify error logging
        mock_logger.error.assert_called_once()

    @patch("jupyter_k8s.controllers.notebook.apps_v1")
    @patch("jupyter_k8s.controllers.notebook.core_v1")
    @patch("jupyter_k8s.controllers.notebook.logger")
    def test_create_notebook_service_failure(self, mock_logger, mock_core_v1, mock_apps_v1):
        """Test handling of service creation failure."""
        # Arrange
        mock_deployment_result = Mock()
        mock_deployment_result.metadata.name = "jupyter-test-notebook"
        mock_apps_v1.create_namespaced_deployment.return_value = mock_deployment_result

        mock_core_v1.create_namespaced_service.side_effect = ApiException(status=400, reason="Bad Request")

        # Act
        result = handle_create_notebook(
            spec=self.mock_spec,
            name="test-notebook",
            namespace="default",
            meta=self.mock_meta,
        )

        # Assert
        self.assertEqual(result["status"]["phase"], "Failed")
        self.assertIn("error", result["status"])
        self.assertEqual(result["status"]["message"], "Failed to create resources")

        # Verify both API calls were attempted
        mock_apps_v1.create_namespaced_deployment.assert_called_once()
        mock_core_v1.create_namespaced_service.assert_called_once()

    def test_create_jupyter_deployment(self):
        """Test creation of Kubernetes deployment object."""
        jupyter_spec = JupyterServerSpec.from_dict(self.mock_spec)
        # Act
        deployment = create_jupyter_deployment(
            name="test-notebook",
            namespace="default",
            jupyter_spec=jupyter_spec,
        )

        # Assert
        self.assertEqual(deployment.metadata.name, "jupyter-test-notebook")
        self.assertEqual(deployment.metadata.namespace, "default")
        self.assertEqual(
            deployment.metadata.labels,
            {"app": "jupyter-server", "instance": "test-notebook"},
        )

        # Check deployment spec
        self.assertEqual(deployment.spec.replicas, 1)
        self.assertEqual(
            deployment.spec.selector.match_labels,
            {"app": "jupyter-server", "instance": "test-notebook"},
        )

        # Check pod template
        container = deployment.spec.template.spec.containers[0]
        self.assertEqual(container.name, "jupyter")
        self.assertEqual(container.image, self.mock_spec["image"])
        self.assertEqual(container.ports[0].container_port, 8888)
        self.assertEqual(container.env[0].name, "JUPYTER_ENABLE_LAB")
        self.assertEqual(container.env[0].value, "yes")

    def test_create_jupyter_service(self):
        """Test creation of Kubernetes service object."""
        # Act
        service = create_jupyter_service(name="test-notebook", namespace="default")

        # Assert
        self.assertEqual(service.metadata.name, "jupyter-test-notebook-service")
        self.assertEqual(service.metadata.namespace, "default")
        self.assertEqual(
            service.metadata.labels,
            {"app": "jupyter-server", "instance": "test-notebook"},
        )

        # Check service spec
        self.assertEqual(service.spec.type, "ClusterIP")
        self.assertEqual(
            service.spec.selector,
            {"app": "jupyter-server", "instance": "test-notebook"},
        )
        self.assertEqual(service.spec.ports[0].port, 8888)
        self.assertEqual(service.spec.ports[0].target_port, 8888)
        self.assertEqual(service.spec.ports[0].name, "jupyter")


class TestDeleteNotebook(unittest.TestCase):
    """Test cases for notebook deletion."""

    @patch("jupyter_k8s.controllers.notebook.apps_v1")
    @patch("jupyter_k8s.controllers.notebook.core_v1")
    @patch("jupyter_k8s.controllers.notebook.logger")
    def test_delete_notebook_success(self, mock_logger, mock_core_v1, mock_apps_v1):
        """Test successful deletion of a Jupyter server."""
        # Act
        handle_delete_notebook(name="test-notebook", namespace="default")

        # Assert
        mock_apps_v1.delete_namespaced_deployment.assert_called_once_with(
            name="jupyter-test-notebook", namespace="default"
        )
        mock_core_v1.delete_namespaced_service.assert_called_once_with(
            name="jupyter-test-notebook-service", namespace="default"
        )

        # Verify logging
        mock_logger.info.assert_any_call("Deployment jupyter-test-notebook deleted successfully")
        mock_logger.info.assert_any_call("Service jupyter-test-notebook-service deleted successfully")

    @patch("jupyter_k8s.controllers.notebook.apps_v1")
    @patch("jupyter_k8s.controllers.notebook.core_v1")
    @patch("jupyter_k8s.controllers.notebook.logger")
    def test_delete_notebook_not_found(self, mock_logger, mock_core_v1, mock_apps_v1):
        """Test deletion when resources don't exist."""
        # Arrange
        mock_apps_v1.delete_namespaced_deployment.side_effect = ApiException(status=404)
        mock_core_v1.delete_namespaced_service.side_effect = ApiException(status=404)

        # Act
        handle_delete_notebook(name="test-notebook", namespace="default")

        # Assert - should not raise exception for 404s
        mock_apps_v1.delete_namespaced_deployment.assert_called_once()
        mock_core_v1.delete_namespaced_service.assert_called_once()

    @patch("jupyter_k8s.controllers.notebook.apps_v1")
    @patch("jupyter_k8s.controllers.notebook.core_v1")
    @patch("jupyter_k8s.controllers.notebook.logger")
    def test_delete_notebook_error(self, mock_logger, mock_core_v1, mock_apps_v1):
        """Test deletion with API error."""
        # Arrange
        mock_apps_v1.delete_namespaced_deployment.side_effect = ApiException(status=500, reason="Internal Server Error")

        # Act
        handle_delete_notebook(name="test-notebook", namespace="default")

        # Assert
        mock_logger.warning.assert_called_once()
        self.assertIn(
            "Failed to delete deployment",
            mock_logger.warning.call_args[0][0],
        )


class TestUpdateNotebook(unittest.TestCase):
    """Test cases for notebook updates."""

    def setUp(self):
        """Set up test fixtures."""
        self.old = {
            "spec": {
                "image": "jupyter/scipy-notebook:old",
                "resources": {"requests": {"memory": "256Mi"}},
            }
        }
        self.new = {
            "spec": {
                "image": "jupyter/scipy-notebook:new",
                "resources": {"requests": {"memory": "512Mi"}},
            }
        }

    @patch("jupyter_k8s.controllers.notebook.apps_v1")
    @patch("jupyter_k8s.controllers.notebook.logger")
    def test_update_notebook_no_changes(self, mock_logger, mock_apps_v1):
        """Test update when no spec changes detected."""
        # Act
        result = handle_update_notebook(
            spec=self.old["spec"],
            name="test-notebook",
            namespace="default",
            old=self.old,
            new=self.old,  # Same spec
        )

        # Assert
        self.assertIsNone(result)
        mock_apps_v1.patch_namespaced_deployment.assert_not_called()
        mock_logger.info.assert_called_with("No spec changes detected for test-notebook, skipping update")

    @patch("jupyter_k8s.controllers.notebook.apps_v1")
    @patch("jupyter_k8s.controllers.notebook.logger")
    def test_update_notebook_failure(self, mock_logger, mock_apps_v1):
        """Test update with API error."""
        # Arrange
        mock_apps_v1.patch_namespaced_deployment.side_effect = ApiException(status=400, reason="Bad Request")

        # Act
        result = handle_update_notebook(
            spec=self.new["spec"],
            name="test-notebook",
            namespace="default",
            old=self.old,
            new=self.new,
        )

        # Assert
        self.assertIsNotNone(result)
        assert result is not None  # For type checking
        self.assertEqual(result["status"]["phase"], "Failed")
        self.assertIn("error", result["status"])

        mock_logger.error.assert_called_once()


if __name__ == "__main__":
    unittest.main()
