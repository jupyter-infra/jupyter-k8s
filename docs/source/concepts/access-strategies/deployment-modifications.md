# Deployment Modifications

An access strategy can modify workspace deployments to support its access method — for example, adding a sidecar container or setting environment variables into the primary container of the workspace.

## What can be modified

The `spec.deploymentModifications.podModifications` attribute of an access strategy supports:

| Modification | Purpose |
|-------------|---------|
| `additionalContainers` | Sidecar containers (e.g. SSM agent, tunnel client) |
| `initContainers` | Containers that run before the workspace starts |
| `volumes` | Extra volumes needed by sidecars |
| `primaryContainerModifications.volumeMounts` | Mount volumes into the main container |
| `primaryContainerModifications.mergeEnv` | Inject environment variables into the main container |

## Example: SSM sidecar for remote access

```yaml
spec:
  deploymentModifications:
    podModifications:
      additionalContainers:
        - name: ssm-agent
          image: amazon/ssm-agent:latest
          env:
            - name: ACTIVATION_ID
              valueFrom:
                fieldRef:
                  fieldPath: metadata.annotations['ssm-activation-id']
      volumes:
        - name: ssm-state
          emptyDir: {}
```

## Merge behavior

Deployment modifications are **injections** — they add attributes to the deployment.
In case of conflict, configurations the access strategy provides take precedence over the `workspace.spec`.

The controller applies the access strategy configuration to the workspace's resources during reconciliation:
- **Appends** additional containers (sidecars) to the workspace pod.
- **Creates** init containers for the pod.
- **Resolves** Environment variables in `mergeEnv` from Go templates (with access to `.Workspace` and `.AccessStrategy`); then
- **Merges** them by name. In case of conflict, the access strategy's environment variables take precedence over `workspace.spec.env`.
