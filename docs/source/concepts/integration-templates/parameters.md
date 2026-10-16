# Parameters

`spec.parameters` on a WorkspaceIntegrationTemplate is the single source of truth for what a referencing workspace must supply. The admin declares parameter names (name only, no values or defaults); the workspace user supplies a value for each one.

```yaml
spec:
  parameters:
    - name: rayClusterName
```

All declared parameters are required. There are no optional parameters and no defaults. A template declares at most 10.

Both sides of the contract are checked at admission:

- An integration template expression that references an undeclared parameter is rejected when the administrator writes the template.
- A workspace that omits a declared parameter, or supplies an empty value for one, is rejected when the user writes the workspace. Only presence and non-emptiness are checked, not what the value names.

Catching these at admission keeps resolution failures out of the reconcile loop, so a rendered pod never silently loses an environment variable because a parameter resolved to empty.

## Where expressions are checked

Admission validates a template by rendering it with each declared parameter seeded, so it reports on the expressions that this rendering evaluates. That is the `{{ .Parameters.<name> }}` form outside a conditional, which is therefore the form to prefer.

Two forms resolve at reconcile time instead:

- **Dynamic key access.** `{{ index .Parameters "name" }}` looks the parameter map up by key. Go's `index` returns an empty string for a key the map does not hold, so the expression resolves to a value rather than raising an error.
- **Conditional branches.** Seeding sets every declared parameter to the empty string, so the body of an `{{ if }}` or `{{ range }}` that is false under those values is not evaluated during validation. It is evaluated on reconcile, once real parameter values decide the branch.

A workspace supplies each value under the matching `integrationTemplateRefs[].parameters` entry; see [Usage](index).
