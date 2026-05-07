# Workspaces

A **Workspace** is the primary custom resource in **Jupyter K8s**. It represents a single compute environment — a pod with dedicated storage and possibly a unique URL.

A workspace maps 1:1 to a Kubernetes deployment with a single replica. When you create a Workspace resource, the controller provisions:

- A **Deployment** running your application image
- A **PersistentVolumeClaim** for durable storage
- A **Service** exposing the application port
- Optionally, **Routing resources** (e.g. IngressRoutes) connecting the workspace to the cluster's reverse proxy

## Minimal example

```yaml
apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: alice-workspace
  namespace: team-alice
spec:
  displayName: Alice Notebook
  image: <your-repo>/<your-image>:<your-tag>
```

If a `workspace.spec` references a **WorkspaceTemplate**, the controller fills in defaults (storage size, mount path, access strategy) that the `workspace.spec` omitted.

## Key fields

| Field | Purpose |
|-------|---------|
| `spec.image` | Application image to run |
| `spec.resources` | CPU/memory requests and limits |
| `spec.storage` | Persistent volume size and mount path in the application container |
| `spec.accessStrategy` | Reference to a **WorkspaceAccessStrategy** for routing configuration |
| `spec.templateRef` | Reference to a **WorkspaceTemplate** for defaults and bounds |
| `spec.desiredStatus` | `Running` or `Stopped` |
| `spec.accessType` | `Public` or `OwnerOnly` — who can connect to the workspace application |
| `spec.ownershipType` | `Public` or `OwnerOnly` — who can modify the workspace configuration |

## Lifecycle states

The workspace reports its state via standard Kubernetes conditions:

- **Available** — pod is running and access probe has passed
- **Progressing** — resources are being created or updated
- **Degraded** — the workspace failed to reach its desired state
- **Stopped** — no pod running, storage persisted

```{toctree}
:hidden:

application-image
access-types
storage
```
