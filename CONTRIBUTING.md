# Contributor Guide

Development guide for **Jupyter K8s** — the Kubernetes operator for Jupyter notebooks and interactive IDEs.

## Prerequisites

- Go (version from `go.mod`)
- Docker or Finch (container runtime)
- Helm (v3.12+)
- Kind (for local clusters)
- kubectl

Install all toolchain dependencies:

```bash
make deps
```

## Project setup

Fork and clone the repository to your local workspace, then run:
```bash
make deps
make build
```

## Build

```bash
make build
```

## Lint

```bash
make lint-fix   # auto-fix where possible
make lint       # check only
```

## Unit tests

Run all unit tests:

```bash
make test
```

### Run specific tests

Target a single package:

```bash
go test ./internal/controller/... -v
```

Target a single test function:

```bash
go test ./internal/controller/... -run TestNewWorkspaceIdleChecker_Success -v
```

### Get targeted coverage

Generate a coverage report for a specific package:

```bash
go test ./internal/controller/... -coverprofile=cover.out
go tool cover -html=cover.out
```

## Local cluster

### Container runtime

The Makefile defaults to Finch (`CONTAINER_TOOL=finch`). To use Docker instead, pass the override:

```bash
make setup-kind CONTAINER_TOOL=docker
make deploy-kind CONTAINER_TOOL=docker
```

Or export it for the session:

```bash
export CONTAINER_TOOL=docker
```

### Setup

Create a Kind cluster and deploy the operator:

```bash
make setup-kind
make deploy-kind
```

### What gets deployed

`make deploy-kind` installs the Helm chart into the `jupyter-k8s-system` namespace with:

- **CRDs** — Workspace, WorkspaceTemplate, WorkspaceAccessStrategy
- **Manager** — a single pod running both the controller and the Extension API server
- **JWT Secret** — HMAC signing key for the Extension API
- **JWT Rotator** — a CronJob that rotates the signing key

### Use kubectl

After `deploy-kind`, your kubectl context points to the Kind cluster. Use it directly:

```bash
make kubectl-kind    # switch context if needed
kubectl get pods -n jupyter-k8s-system
kubectl logs -n jupyter-k8s-system deployment/jupyter-k8s-controller-manager
```

### Apply samples

Deploy sample workspaces from `config/samples/`:

```bash
make apply-samples
kubectl get workspaces
```

Remove them:

```bash
make delete-samples
```

### Port forward to a workspace

Connect to a running workspace in your browser:

```bash
make port-forward
```

This lists available workspaces, prompts you for one, and opens a port-forward on `localhost:8888` (macOS) or `hostname:9888` (Linux).

### Teardown

```bash
make teardown-kind
```

## End-to-end tests

### Run

E2E tests spin up a separate Kind cluster:

```bash
make test-e2e
```

### Run focused tests

```bash
make test-e2e-focus FOCUS="Workspace Access Strategy"
```

### Lint

```bash
make lint-e2e
```

## Helm chart

When modifying the Helm chart, edit files under `/api`, `/config`, and `/hack`, then regenerate:

```bash
make helm-generate   # outputs to dist/chart/
make helm-lint
make helm-test       # results in dist/test-output-crd-only/
```

## Documentation

### Build

```bash
make docs            # render diagrams + build HTML
make docs-serve      # live-reload dev server on :8080
```

### Diagrams

Architecture diagrams live as D2 source files in `diagrams/`. The build renders them to SVG:

```bash
make docs-diagrams   # renders diagrams/*.d2 → docs/source/_static/img/diagrams/*.svg
```

Edit `.d2` files directly, then run `make docs` to see the result. See `diagrams/AGENT.md` for conventions.

### Structure

Source files live in `docs/source/`. The site uses Sphinx + MyST Markdown with the Shibuya theme. See `docs/AGENT.md` for formatting rules.

## AWS development (ECR)

Push images to an AWS ECR registry:

```bash
make setup-aws EKS_CLUSTER_NAME=<cluster> AWS_REGION=<region>
make load-images-aws
make kubectl-aws
```

Guided chart deployment and testing lives in [jupyter-k8s-aws](https://github.com/jupyter-infra/jupyter-k8s-aws).

## Release process

Releases run via GitHub Actions workflow dispatch:

1. A maintainer triggers the `Release` workflow with a version (e.g. `v0.2.0`).
2. The pipeline builds images and pushes them to the staging registry (`ghcr.io/jupyter-infra/staging`).
3. It packages the Helm chart and pushes it to the staging OCI registry.
4. E2E tests run against the staged artifacts.
5. On success, the pipeline promotes images and chart to the production registry (`ghcr.io/jupyter-infra`).
6. A GitHub Release appears with auto-generated release notes.

## Before submitting a PR

```bash
make build
make lint
make test
make helm-test
```

## Project structure

| Directory | Contents |
|-----------|----------|
| `api/` | CRD Go types and markers |
| `internal/controller/` | Reconciliation loops |
| `internal/webhook/` | Mutating and admission webhooks |
| `internal/extensionapi/` | Extension API server (Connection APIs) |
| `internal/authmiddleware/` | Auth middleware handling workspace access |
| `internal/rotator/` | JWT key rotation image for CronJob |
| `internal/pluginadapters/` | Controller-side plugin adapter interfaces |
| `internal/awsadapter/` | AWS-specific adapter (SSM orchestration) |
| `config/` | Kubebuilder kustomize overlays |
| `dist/chart/` | Generated Helm chart output |
| `images/` | Container images (auth middleware, rotator, reference apps) |
| `docs/` | Sphinx documentation source |
| `diagrams/` | D2 architecture diagram sources |
