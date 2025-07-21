"""Models for Jupyter notebook resources."""

from datetime import datetime

from pydantic import BaseModel, Field


class NotebookCreate(BaseModel):
    """Model for creating a new Jupyter notebook."""

    name: str = Field(..., description="Name of the notebook")


class NotebookResponse(BaseModel):
    """Model for Jupyter notebook API responses."""

    name: str = Field(..., description="Name of the notebook")
    namespace: str = Field(default="default", description="Namespace of the notebook")
    created_at: datetime | None = Field(None, description="Creation timestamp")
    status: str = Field(default="Unknown", description="Current status of the notebook")
    image: str = Field(..., description="Docker image for the notebook")
