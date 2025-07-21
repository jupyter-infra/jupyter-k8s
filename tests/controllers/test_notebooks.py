"""Tests for the Jupyter K8s controller."""

import unittest
from typing import Any
from unittest.mock import Mock, patch

from jupyter_k8s.controllers.notebook import create_notebook, delete_notebook


class TestCreateNotebook(unittest.TestCase):
    @patch("jupyter_k8s.controllers.notebook.logger")
    def test_create_notebook_happy_case(self, mock_logger: Mock) -> None:
        """Test that create_notebook logs messages and returns expected status."""
        # Arrange
        mock_kwargs = {"meta": {"name": "test-notebook"}}

        # Act
        result = create_notebook(**mock_kwargs)  # type: ignore

        # Assert
        self.assertEqual({"status": "Acknowledged", "message": "Create request logged (no action taken)"}, result)
        mock_logger.info.assert_called()
        self.assertEqual(mock_logger.info.call_count, 2)

    @patch("jupyter_k8s.controllers.notebook.logger")
    def test_create_notebook_no_meta(self, mock_logger: Mock) -> None:
        """Test that create_notebook handles missing metadata."""
        # Arrange
        mock_kwargs = {"name": "test-notebook"}

        # Act
        result = create_notebook(**mock_kwargs)  # type: ignore

        # Assert
        self.assertEqual({"status": "Acknowledged", "message": "Create request logged (no action taken)"}, result)
        mock_logger.info.assert_called()
        self.assertEqual(mock_logger.info.call_count, 2)

    @patch("jupyter_k8s.controllers.notebook.logger")
    def test_create_notebook_no_name(self, mock_logger: Mock) -> None:
        """Test that create_notebook handles missing name."""
        # Arrange
        mock_kwargs: dict[str, Any] = {}

        # Act
        result = create_notebook(**mock_kwargs)  # type: ignore

        # Assert
        self.assertEqual({"status": "Acknowledged", "message": "Create request logged (no action taken)"}, result)
        self.assertEqual(mock_logger.info.call_count, 2)


class TestDeleteNotebook(unittest.TestCase):
    @patch("jupyter_k8s.controllers.notebook.logger")
    def test_delete_notebook_happy_case(self, mock_logger: Mock) -> None:
        """Test that delete_notebook logs messages."""
        # Arrange
        mock_kwargs = {"meta": {"name": "test-notebook"}}

        # Act
        delete_notebook(**mock_kwargs)  # type: ignore

        # Assert
        self.assertEqual(mock_logger.info.call_count, 2)

    @patch("jupyter_k8s.controllers.notebook.logger")
    def test_delete_notebook_no_meta(self, mock_logger: Mock) -> None:
        """Test that delete_notebook handles missing metadata."""
        # Arrange
        mock_kwargs = {"name": "test-notebook"}

        # Act
        delete_notebook(**mock_kwargs)  # type: ignore

        # Assert
        mock_logger.info.assert_called()
        self.assertEqual(mock_logger.info.call_count, 2)

    @patch("jupyter_k8s.controllers.notebook.logger")
    def test_delete_notebook_no_name(self, mock_logger: Mock) -> None:
        """Test that delete_notebook handles missing name."""
        # Arrange
        mock_kwargs: dict[str, Any] = {}

        # Act
        delete_notebook(**mock_kwargs)  # type: ignore

        # Assert
        mock_logger.info.assert_called()
        self.assertEqual(mock_logger.info.call_count, 2)
