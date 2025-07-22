# Claude Configuration

## Code Quality Commands

When writing or modifying Python files, always run these commands before committing:

1. **Linting with auto-fix:**
```bash
uv run ruff check --fix
```

2. **Formatting:**
```bash
uv run ruff format
```

3. **Type checking:**
```bash
uv run mypy
```

## Project Structure

- `src/jupyter_k8s/` - Main package source code
  - `controllers/` - Kopf-based Kubernetes controllers
  - `api/` - FastAPI endpoints
  - `models/` - Pydantic models
- `helm/` - Helm chart for deployment
- `tests/` - Test files

## Development Workflow

1. Implement changes
2. Run quality checks (lint, format, type-check)
3. IMPORTANT: Never commit or stage files - let the user handle all git operations

## Git Usage

- DO NOT use `git add`, `git commit`, or `git push` commands
- DO NOT stage any files
- Let the user handle all git operations manually
- Focus only on code implementation and quality