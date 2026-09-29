# Deployment modifications

`spec.deploymentModifications.podModifications` declares what the integration adds to the workspace pod: `additionalContainers`, `initContainers`, `volumes`, `primaryContainerModifications.volumeMounts`, and `primaryContainerModifications.mergeEnv`. These fields take the same shape as the block of the same name on an access strategy; see [Access Strategies: Deployment Modifications](../access-strategies/deployment-modifications) for the field-by-field reference.

`exposedPorts` is part of that shared shape but is not supported on an integration template. Publishing a sidecar port on the workspace Service is an access strategy's job. The schema currently admits the field here and the port is never published, so do not set it.

String values within these fields may carry template expressions — `{{ resource "<handle>" "<jsonpath>" }}`, `{{ .Parameters.<name> }}`, and `{{ .Workspace.* }}`. See [Template expressions](template-expressions).

## What re-renders, and when

The pod shape and the resolved resource values are not refreshed on the same schedule, and the difference determines whether an edit rolls the workspace pod.

The pod shape — which sidecars and init containers, which volumes and mounts, which environment variable names, the container commands — is read from the template on every reconcile. An edit to the template therefore takes effect on the next reconcile and rolls the pod.

The values that `{{ resource }}` expressions read out of the referenced object are resolved once and then frozen. They are recorded in `status.resolvedIntegrations[].values` as a map keyed `"<resourceRefID>|<jsonPath>"`, and later reconciles replay that map rather than re-reading the object. Editing the referenced object does not re-render the pod. Only the substitutions are stored, not the rendered pod spec, which keeps the status payload small.

The values are captured again when the supplied parameters change or the template version changes. See [Resolution and drift](template-expressions.md#resolution-and-drift).

```{note}
This is the one behaviour that does not carry over from access strategies, which re-resolve `mergeEnv` on every reconcile so that a change to their inputs flows straight through to the pod.
```

## shareProcessNamespace

`spec.shareProcessNamespace` places every container in the workspace pod in a single process namespace, so that the workspace container can see and signal processes belonging to an injected sidecar.

Sharing weakens the isolation between the containers in the pod, in two degrees.

Every container sees the others' processes and their full command lines, because `/proc/<pid>/cmdline` is world-readable. A value passed to a sidecar as a command-line argument is therefore readable by the workspace container, and by any other sidecar, whatever user they run as. Pass secrets by environment variable or mounted file instead of on the command line.

A container's environment and filesystem, read through `/proc/<pid>/environ` and `/proc/<pid>/root`, remain subject to normal file permissions: they are readable by another container only when both run as the same user, or when the reader runs as root or holds `CAP_SYS_PTRACE`. Running injected containers as non-root, with a UID distinct from the workspace container's, keeps that boundary intact.

Even so, a pod that shares its process namespace is best treated as one trust domain rather than as a set of isolated containers.
