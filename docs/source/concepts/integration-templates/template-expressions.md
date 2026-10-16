# Template expressions

String fields in a WorkspaceIntegrationTemplate carry Go template expressions, resolved at reconcile time. Three contexts are available:

- `{{ .Workspace.Name }}` and `{{ .Workspace.Namespace }}`: identity of the referencing workspace.
- `{{ .Parameters.<name> }}`: a value the workspace supplied for a declared parameter.
- `{{ resource "<handle>" "<jsonpath>" }}`: a value read from a referenced resource (see [resourceRefs](#resourcerefs) below). The `<handle>` matches a `resourceRefs[].name`; the JSONPath selects a field on that object, for example `{{ resource "rayCluster" "{.status.head.serviceName}" }}`.

Resolution is fail-closed. If any expression cannot resolve (an unknown handle, an invalid or empty JSONPath result), the controller aborts and does not apply a partial overlay to the pod.

## resourceRefs

`spec.resourceRefs` lists the resources the template fetches at resolution time. Each entry gives the target a stable handle for use in `{{ resource "<handle>" ... }}` expressions:

```yaml
resourceRefs:
  - name: rayCluster       # the handle used in {{ resource "rayCluster" ... }}
    apiVersion: ray.io/v1
    kind: RayCluster
    metadata:
      name: "{{ .Parameters.rayClusterName }}"   # object name; supports template expressions
```

`apiVersion` and `kind` identify the resource type; `metadata.name` identifies the specific object and may itself be templated (for example `{{ .Workspace.Name }}-ray`). The object is always looked up in the referencing workspace's own namespace; a `resourceRef` cannot target another namespace.

An integration template requires exactly one `resourceRef` today (minimum one, capped at one). A template exists to resolve values from a referenced resource, so at least one ref is required.

The workspace user chooses which object to fetch, so the `kind` and JSONPath you write define the entire blast radius of the integration.

The operator must also be granted read access to that kind; see [Ownership and RBAC](index.md#ownership-and-rbac).

## Resolution and drift

The controller resolves an integration only when its input changes: a hash of the template ref plus the supplied parameters, or the template's version. The version is `"<UID>.<Generation>"`, so both an admin edit (Generation) and a delete-and-recreate with an identical spec (UID) force re-resolution.

On any other reconcile, including an idle reconcile or one following drift in the referenced object, the controller replays the values recorded in `status.resolvedIntegrations` rather than re-reading the object. Drift in a referenced object therefore never rolls a running pod.

Only the `{{ resource }}` values are frozen this way. The pod shape is read from the template on every reconcile, so a template edit does roll the pod; see [Deployment modifications](deployment-modifications).
