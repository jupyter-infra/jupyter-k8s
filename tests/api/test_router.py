"""Tests for the Jupyter K8s API."""

import unittest
from datetime import datetime
from unittest.mock import Mock, patch

from fastapi.testclient import TestClient

from jupyter_k8s.main import app


class TestFastApiApp(unittest.TestCase):
    def setUp(self) -> None:
        self.client = TestClient(app)

    @patch("jupyter_k8s.api.router.logger")
    def test_list_notebooks(self, mock_logger: Mock) -> None:
        """Test that list_notebooks endpoint returns expected data."""
        # Act
        response = self.client.get("/api/v1/notebooks")

        # Assert
        self.assertEqual(response.status_code, 200)
        notebooks = response.json()
        self.assertEqual(len(notebooks), 1)
        self.assertEqual(notebooks[0]["name"], "sample-notebook")
        self.assertEqual(notebooks[0]["status"], "Running")
        mock_logger.info.assert_called()

    @patch("jupyter_k8s.api.router.logger")
    def test_get_notebook_found(self, mock_logger: Mock) -> None:
        """Test that get_notebook endpoint returns expected data for existing notebook."""
        # Act
        response = self.client.get("/api/v1/notebooks/sample-notebook")

        # Assert
        self.assertEqual(response.status_code, 200)
        notebook = response.json()
        self.assertEqual(notebook["name"], "sample-notebook")
        self.assertEqual(notebook["status"], "Running")
        mock_logger.info.assert_called()

    @patch("jupyter_k8s.api.router.logger")
    def test_get_notebook_not_found(self, mock_logger: Mock) -> None:
        """Test that get_notebook endpoint returns 404 for non-existent notebook."""
        # Act
        response = self.client.get("/api/v1/notebooks/non-existent")

        # Assert
        self.assertEqual(response.status_code, 404)
        error = response.json()
        self.assertEqual(error["detail"], "Notebook not found")
        mock_logger.info.assert_called()

    @patch("jupyter_k8s.api.router.logger")
    @patch("jupyter_k8s.api.router.datetime")
    def test_create_notebook(self, mock_datetime: Mock, mock_logger: Mock) -> None:
        """Test that create_notebook endpoint returns expected data."""
        # Arrange
        mock_now = datetime(2023, 1, 1, 12, 0)
        mock_datetime.now.return_value = mock_now

        notebook_data = {"name": "new-notebook"}

        # Act
        response = self.client.post("/api/v1/notebooks", json=notebook_data)

        # Assert
        self.assertEqual(response.status_code, 201)
        created = response.json()
        self.assertEqual(created["name"], "new-notebook")
        self.assertEqual(created["status"], "Creating")
        self.assertEqual(created["created_at"], mock_now.isoformat())
        mock_logger.info.assert_called()

    @patch("jupyter_k8s.api.router.logger")
    def test_delete_notebook_found(self, mock_logger: Mock) -> None:
        """Test that delete_notebook endpoint returns 204 for existing notebook."""
        # Act
        response = self.client.delete("/api/v1/notebooks/sample-notebook")

        # Assert
        self.assertEqual(response.status_code, 204)
        self.assertEqual(response.content, b"")  # No content for 204
        mock_logger.info.assert_called()

    @patch("jupyter_k8s.api.router.logger")
    def test_delete_notebook_not_found(self, mock_logger: Mock) -> None:
        """Test that delete_notebook endpoint returns 404 for non-existent notebook."""
        # Act
        response = self.client.delete("/api/v1/notebooks/non-existent")

        # Assert
        self.assertEqual(response.status_code, 404)
        error = response.json()
        self.assertEqual(error["detail"], "Notebook not found")
        mock_logger.info.assert_called()
