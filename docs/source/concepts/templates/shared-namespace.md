# Shared Namespace

The **shared namespace** is a cluster-wide namespace where administrators place templates and access strategies that should be available to workspaces in any namespace.

## Configuration

Set the `--default-template-namespace` flag on **Jupyter K8s** (default: `jupyter-k8s-shared`) to identify the shared namespace. In the Helm chart, use `workspaceTemplates.defaultNamespace`.

(default-template-resolution)=
## Default template resolution

When a user creates a workspace without a `spec.templateRef`, the [**workspace mutating webhook**](../../dive-deeper/webhooks/workspace-defaults.md) looks for a default template — a template labeled with:

```yaml
metadata:
  labels:
    workspace.jupyter.org/default-template: "true"
```

The lookup order is:

1. **Workspace namespace** — search for a default-labeled template in the workspace's own namespace.
2. **Shared namespace** — if no local default is found, fall back to the shared namespace.

A template in the workspace's namespace takes priority over a template in the shared namespace. When the webhook finds a default template, it sets the workspace's `spec.templateRef` attribute to point to that template.

If neither namespace contains a default template, the webhook leaves `spec.templateRef` unset.

## Cross-namespace references

Normally, a `workspace.spec.templateRef` and `workspace.spec.accessStrategy` can only reference resources in the workspace's own namespace. The shared namespace is the one exception — workspaces in any namespace may reference templates and access strategies that live in the shared namespace.

## Example setup

```yaml
apiVersion: workspace.jupyter.org/v1alpha1
kind: WorkspaceTemplate
metadata:
  name: org-default
  namespace: jupyter-k8s-shared
  labels:
    workspace.jupyter.org/default-template: "true"
spec:
  defaultImage: <repo>/<application-image-name>:<tag>
  defaultAccessStrategy:
    name: web-access
  primaryStorage:
    defaultSize: 10Gi
    defaultMountPath: /home/jovyan
```

With this in place, any workspace in any namespace automatically inherits this template's defaults — unless it specifies its own `templateRef`.

## When to use

- **Organization-wide defaults** — a single template that applies to all new workspaces across the cluster.
- **Shared access strategies** — one access strategy that all namespaces can reference, rather than duplicating it per namespace.
- **Team overrides** — teams can define their own default-labeled template in their namespace to override the shared default.
