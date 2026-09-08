# Deployment modifications

`spec.deploymentModifications.podModifications` is where a WIT actually changes the workspace pod. It uses the same shape as an access strategy's deployment modifications: `additionalContainers`, `initContainers`, `volumes`, `primaryContainerModifications.volumeMounts`, and `primaryContainerModifications.mergeEnv`. See [Access Strategies: Deployment Modifications](../access-strategies/deployment-modifications) for the field-by-field reference; the fields behave the same here.

Two differences matter:

- **Template context.** A WIT resolves `{{ resource "<handle>" "<jsonpath>" }}`, `{{ .Parameters.<name> }}`, and `{{ .Workspace.* }}`. An access strategy instead exposes `.Workspace` and `.AccessStrategy`, and has no `resource` function. See [Template expressions](template-expressions).
- **The result is frozen, not re-applied live.** This is the key distinction: an access strategy re-resolves its `mergeEnv` on every reconcile, so a change to its inputs flows straight through. A WIT does not work this way. It resolves once, records the rendered sidecars, volumes, and environment variables in `status.resolvedIntegrations`, and replays that frozen result on later reconciles. Editing the referenced resource does not re-render the workspace pod; the injection changes only when the parameters or the template itself change. See [Resolution and drift](template-expressions.md#resolution-and-drift).

## shareProcessNamespace

`spec.shareProcessNamespace` is an admin-only, pod-level toggle that shares the PID namespace across all containers in the workspace pod, so the workspace container can see and signal processes running in an injected sidecar (for example, to attach to a sidecar's local Ray session). Containers sharing a PID namespace can read each other's `/proc` (filesystem and environment), so injected containers should run as non-root.
