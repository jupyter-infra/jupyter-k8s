# Integration Templates

A **WorkspaceIntegrationTemplate** (WIT) injects runtime capabilities into a workspace pod: sidecar containers, volumes, and environment variables. Unlike a **WorkspaceTemplate**, which sets static defaults and bounds, a WIT resolves its values dynamically at reconcile time by reading a live Kubernetes resource and substituting template expressions.

A WIT has a 1:many relationship with workspaces. Multiple workspaces may attach the same template, each supplying its own parameters.

## When to use a WIT

Reach for a WIT when a workspace needs to connect to another resource that exists in the Kubernetes cluster, and the connection details are only known at runtime. The canonical case is wiring a workspace to a `RayCluster` or similar object: the WIT fetches that object, reads fields from it (host, port, name), and renders them into the workspace pod as sidecars or environment variables.

Use a plain **WorkspaceTemplate** instead when the configuration is static (a fixed image, resource bounds, a default access strategy). Use a WIT when the configuration must be computed from another object's live state.

Like access strategies, a WIT is an administrator-owned resource. Admins install a small set of vetted integrations; workspace users attach them by reference and supply parameters. In an enterprise cluster, workspace users should not have permission to create or edit integration templates directly.

## Usage

A workspace user attaches an integration through `spec.integrationTemplateRefs`. Each entry names a WIT and supplies the parameter values the template declares:

```yaml
apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: alice-workspace
  namespace: alice-team
spec:
  displayName: Alice's Workspace
  image: my-repository/my-image:my-tag
  integrationTemplateRefs:
    - name: ray-integration
      parameters:
        - name: rayClusterName
          value: team-ray
```

`spec.integrationTemplateRefs` is capped at one entry for now. The reference resolves within the workspace's own namespace. As with templates and access strategies, a workspace may also reference a WIT in the [shared namespace](../templates/shared-namespace), a special namespace identified at the **Jupyter K8s** operator level.

An `integrationTemplateRefs` entry only declares which WIT to attach and the parameter values to use; it does not contain the injected sidecars, volumes, or environment variables. The controller computes those during reconciliation and records the resolved result in the workspace status. The workspace user does not author the template or write template expressions; they only choose a WIT and supply its parameter values.

## Authoring a WIT

Administrators define the template itself. These pages cover each part of a WIT spec:

- [Parameters](parameters) — the contract of values a workspace must supply.
- [Template expressions](template-expressions) — reading live resource fields with `{{ resource }}`, `{{ .Parameters }}`, and `{{ .Workspace }}`, and when the controller re-resolves.
- [Deployment modifications](deployment-modifications) — the sidecars, volumes, and environment variables injected into the pod.
- [Status probe](status-probe) — the report-only reachability check.
- [Example setup](example) — a complete, working Ray integration.

```{toctree}
:hidden:

parameters
template-expressions
deployment-modifications
status-probe
example
```
