# Integration Templates

A **WorkspaceIntegrationTemplate** injects runtime capabilities into a workspace pod: sidecar containers, volumes, and environment variables. Unlike a **WorkspaceTemplate**, which sets static defaults and bounds, an integration template resolves its values dynamically at reconcile time by reading a live Kubernetes resource and substituting template expressions.

A WorkspaceIntegrationTemplate has a 1:many relationship with workspaces. Multiple workspaces may attach the same template, each supplying its own parameters.

## When to use an integration template

Reach for a WorkspaceIntegrationTemplate when a workspace needs to connect to another resource that exists in the Kubernetes cluster, and the connection details are only known at runtime. The canonical case is wiring a workspace to a `RayCluster` or similar object: the template fetches that object, reads fields from it (host, port, name), and renders them into the workspace pod as sidecars or environment variables.

Use a plain **WorkspaceTemplate** instead when the configuration is static (a fixed image, resource bounds, a default access strategy). Use a WorkspaceIntegrationTemplate when the configuration must be computed from another object's live state.

## Ownership and RBAC

A WorkspaceIntegrationTemplate is intended to be an administrator-owned resource. An administrator installs a small set of vetted integrations; workspace users attach them by reference and supply parameter values.

The API does not enforce that division. No field is reserved for administrators: whoever can write the resource writes all of it, `shareProcessNamespace` and the pod modifications included. RBAC is what establishes the division.

### Permissions for workspace users

Workspace users need `get` and `list` on `workspaceintegrationtemplates` to discover which integrations exist and which parameters each one declares. They must not have `create`, `update`, `patch`, or `delete`.

Write access on this resource is equivalent to read access on any object the operator can reach in the namespace. The template names the `kind` and the JSONPath to read, and the operator performs that read.

### Permissions for the operator

The operator reads each object named in a template's `resourceRefs`, and a default install does not permit it. The kinds an integration references depend on the deployment rather than on the operator, so they are not part of the operator's generated ClusterRole. Granting the read is part of installing an integration.

Grant `get` on the referenced kind to the operator's ServiceAccount. For a template that references a `RayCluster`, for example:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: jupyter-k8s-integration-resource-reader
rules:
  - apiGroups: ["ray.io"]
    resources: ["rayclusters"]
    verbs: ["get"]
```

Bind it to the operator's ServiceAccount with a `ClusterRoleBinding`, alongside the operator's own role. `get` is sufficient on its own: the operator reads the object directly from the API server rather than through a cache, so it needs no `list` or `watch`.

The grant is cluster-wide, but the read is not. A `resourceRef` resolves only in the referencing workspace's own namespace, and carries no namespace field to point elsewhere — the controller supplies the workspace's namespace itself. That bound comes from the controller, not from the shape of the grant, so scoping the role more narrowly is not what keeps an integration inside its namespace.

```{note}
Resolution is fail-closed. Without this grant the read fails, no partial overlay is applied to the pod, and the workspace reports the integration as degraded.
```

### Vetting a template

An administrator chooses the `kind` a template reads; the workspace user chooses which object of that kind it reads. `resourceRefs[].metadata.name` is rendered from a parameter the workspace supplies, and admission checks only that the parameter is present and non-empty, not what it names.

An integration also does more than read the object it resolves. In the Ray integration the injected sidecar runs `ray start --address=...`, so the workspace joins the cluster it named.

Vet a template on the assumption that any object of the referenced kind in the workspace's own namespace is reachable through it.

## Usage

A workspace user attaches an integration through `spec.integrationTemplateRefs`. Each entry names a template and supplies the parameter values that template declares:

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
      namespace: jupyter-k8s-system   # where the template lives; omit when it is in the workspace's own namespace
      parameters:
        - name: rayClusterName
          value: team-ray
```

`spec.integrationTemplateRefs` is capped at one entry for now.

`namespace` selects where the template is read from, and admission accepts exactly three values: unset, the workspace's own namespace, or the [shared namespace](../templates/shared-namespace) configured at the **Jupyter K8s** operator level. An omitted `namespace` means the workspace's own namespace. Anything else is rejected before the template is read.

An `integrationTemplateRefs` entry only declares which template to attach and the parameter values to use; it does not contain the injected sidecars, volumes, or environment variables. The controller computes those during reconciliation and records the resolved result in the workspace status. The workspace user does not author the template or write template expressions; they only choose a template and supply its parameter values.

## Authoring an integration template

Administrators define the template itself. These pages cover each part of the spec:

- [Parameters](parameters) — the contract of values a workspace must supply.
- [Template expressions](template-expressions) — reading live resource fields with `{{ resource }}`, `{{ .Parameters }}`, and `{{ .Workspace }}`, and when the controller re-resolves.
- [Deployment modifications](deployment-modifications) — the sidecars, volumes, and environment variables injected into the pod.
- [Status probe](status-probe) — the report-only reachability check.
- [Example setup](example) — a complete, maintained Ray integration.

```{toctree}
:hidden:

parameters
template-expressions
deployment-modifications
status-probe
example
```
