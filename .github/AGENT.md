# .github — CI Workflows

## Workflows

| Workflow | Trigger | Purpose |
|----------|---------|---------|
| `lint.yml` | push/PR | golangci-lint + helm lint |
| `test.yml` | push/PR | Unit tests |
| `test-chart.yml` | push/PR | Helm install on Kind (operator-only chart) |
| `test-e2e.yml` | push/PR/dispatch | Full E2E on Kind |
| `release.yml` | `workflow_dispatch` | Orchestrator: stage → e2e → promote → release |
| `release-stage-images.yml` | `workflow_call` / `workflow_dispatch` | Build multi-arch images, push to staging GHCR |
| `release-stage-chart.yml` | `workflow_call` / `workflow_dispatch` | Generate, package, push chart to staging GHCR |
| `release-e2e.yml` | `workflow_call` / `workflow_dispatch` | E2E suite against staging artifacts |
| `release-promote-images.yml` | `workflow_call` / `workflow_dispatch` | Promote images: crane copy staging → production |
| `release-promote-chart.yml` | `workflow_call` / `workflow_dispatch` | Promote chart: helm pull staging, push production |

## Testing Workflow Changes

To iterate on workflow changes from a feature branch, create a temporary push-triggered
workflow that calls the reusable sub-workflows:

```yaml
# .github/workflows/test-<name>.yml  — DO NOT merge to main
name: Test workflow (temporary)
on:
  push:
    branches: [your-branch]
permissions:
  contents: read
  packages: write
jobs:
  test:
    uses: ./.github/workflows/release-stage-images.yml
    with:
      version: v0.1.0-rc.1
      short_sha: ""
    permissions:
      contents: read
      packages: write
```

- `workflow_dispatch` only fires on the default branch — use push triggers for feature branches.
- Each release sub-workflow supports both `workflow_call` and `workflow_dispatch`, so after
  merging you can trigger each step individually from the Actions UI.
- Use pre-release versions (e.g. `v0.1.0-rc.1`) to avoid colliding with real releases.
- Remove test workflows before merging to main.

## Release Flow

```
workflow_dispatch (version + dry_run)
  → validate (semver, tag uniqueness, branch check)
  → stage-images (multi-arch build → staging GHCR)
  → stage-chart (helm-generate → package → staging GHCR)
  → e2e (full test suite against staging)
  → promote-images (crane copy → production GHCR)     [skipped if dry_run]
  → promote-chart (helm pull/push → production GHCR)  [skipped if dry_run]
  → release (git tag + GitHub Release)                 [skipped if dry_run]
```

## Registries

| Namespace | Visibility | Purpose |
|-----------|-----------|---------|
| `ghcr.io/jupyter-infra/staging/` | Private | Pre-release validation |
| `ghcr.io/jupyter-infra/` | Public | Production artifacts |
| `ghcr.io/jupyter-infra/staging/charts/` | Private | Chart staging |
| `ghcr.io/jupyter-infra/charts/` | Public | Chart production |
