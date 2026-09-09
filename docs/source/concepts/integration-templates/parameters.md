# Parameters

`spec.parameters` on a WIT is the single source of truth for what a referencing workspace must supply. The admin declares parameter names (name only, no values or defaults); the workspace user supplies a value for each one.

```yaml
spec:
  parameters:
    - name: rayClusterName
```

All declared parameters are required. There are no optional parameters or defaults:

- A template expression that references an undeclared parameter is rejected when the admin writes the template.
- A workspace that omits a declared parameter, or leaves it empty, is rejected when the user writes the workspace.

This keeps resolution failures out of the reconcile loop. A rendered pod never silently loses an environment variable because a parameter resolved to empty. A WIT declares at most 10 parameters.

A workspace supplies each value under the matching `integrationTemplateRefs[].parameters` entry; see [Usage](index).
