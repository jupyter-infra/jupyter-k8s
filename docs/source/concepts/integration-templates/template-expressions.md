# Template expressions

String fields in a WIT carry Go template expressions, resolved at reconcile time. Three contexts are available:

- `{{ .Workspace.Name }}` and `{{ .Workspace.Namespace }}`: identity of the referencing workspace.
- `{{ .Parameters.<name> }}`: a value the workspace supplied for a declared parameter.
- `{{ resource "<handle>" "<jsonpath>" }}`: a value read from a referenced resource (see [resourceRefs](#resourcerefs) below). The `<handle>` matches a `resourceRefs[].name`; the JSONPath selects a field on that object, for example `{{ resource "rayCluster" "{.status.head.serviceName}" }}`.

Resolution is fail-closed. If any expression cannot resolve (an unknown handle, an invalid or empty JSONPath result), the controller aborts and does not apply a partial overlay to the pod.

## resourceRefs

`spec.resourceRefs` lists the resources the WIT fetches at resolution time. Each entry gives the target a stable handle for use in `{{ resource "<handle>" ... }}` expressions:

```yaml
resourceRefs:
  - name: rayCluster       # the handle used in {{ resource "rayCluster" ... }}
    apiVersion: ray.io/v1
    kind: RayCluster
    metadata:
      name: "{{ .Parameters.rayClusterName }}"   # object name; supports template expressions
```

`apiVersion` and `kind` identify the resource type; `metadata.name` identifies the specific object and may itself be templated (for example `{{ .Workspace.Name }}-ray`). The object is always looked up in the referencing workspace's own namespace; a `resourceRef` cannot target another namespace.

A WIT requires exactly one `resourceRef` today (minimum one, capped at one). A template exists to resolve values from a referenced resource, so at least one ref is required.

## Resolution and drift

The controller resolves an integration only when its input changes: a hash of the template ref plus the supplied parameters, or the template's own generation (after an admin edits the WIT). On any other reconcile, including external drift in the referenced resource or an idle reconcile, the controller replays the frozen values recorded in `status.resolvedIntegrations` rather than re-reading the resource. Drift in a referenced object therefore never rolls a running pod. Re-resolution happens only on an intentional change to the parameters or the template.
