# Single stage build that tries to use either direct network access or pre-built dependencies
FROM ghcr.io/astral-sh/uv:python3.12-bookworm-slim

WORKDIR /app

# Copy the application code
COPY pyproject.toml /app/
COPY README.md /app/
COPY src/ /app/src/

RUN uv sync --no-dev --no-cache

# Expose API port
EXPOSE 8000

# Run the application using the entrypoint defined in pyproject.toml
CMD ["uv", "run", "jupyter-k8s"]