"""Data models for Jupyter K8s operator."""

from typing import Literal

from pydantic import BaseModel


class ResourceRequirements(BaseModel):
    """Kubernetes resource requirements."""

    requests: dict[str, str] | None = None
    limits: dict[str, str] | None = None


class JupyterServerSpec(BaseModel):
    """JupyterServer spec model."""

    name: str
    image: str
    desiredStatus: Literal["Running", "Stopped"] | None = "Running"
    serviceAccountName: str | None = None
    resources: ResourceRequirements | None = None

    @classmethod
    def from_dict(cls, data: dict) -> "JupyterServerSpec":
        """Create instance from dictionary with nested resource conversion."""
        if "resources" in data and data["resources"] is not None:
            data = data.copy()
            data["resources"] = ResourceRequirements(**data["resources"])
        return cls(**data)
