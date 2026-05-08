# Access Resources

The `spec.accessResourceTemplates` attribute of an access strategy defines Kubernetes resources that the controller creates in the workspace's namespace to wire up network access.

## Template rendering

Each template is a Go `text/template` string with access to three variables:

| Variable | Content |
|----------|---------|
| `.Workspace` | The full Workspace object |
| `.AccessStrategy` | The full WorkspaceAccessStrategy object |
| `.Service` | The workspace's Service object (name, port, namespace) |

## Example: Traefik IngressRoute

```yaml
spec:
  accessResourceTemplates:
    - kind: IngressRoute
      apiVersion: traefik.io/v1alpha1
      namePrefix: web
      template: |
        spec:
          entryPoints: [websecure]
          routes:
            - match: PathPrefix(`/workspaces/{{ .Workspace.Namespace }}/{{ .Workspace.Name }}/`)
              kind: Rule
              middlewares:
                - name: auth-middleware
                  namespace: jupyter-k8s-router
              services:
                - name: {{ .Service.Name }}
                  port: 8888
```

## Lifecycle

During the reconciliation loop of a workspace, the controller:

- **Creates** access resources before setting the `workspace.status` to `Available`, and sets the workspace as their `owner.reference`
- **Updates** access resources when the access strategy or workspace spec changes.
- **Deletes** access resources when setting the `workspace.status` to `Stopping`.

The workspace's `workspace.status.accessResources` field lists all resources created from these templates, and `workspace.status.accessResourceSelector` provides a label selector to find them.

On deletion of the workspace, Kubernetes' garbage collector detects and deletes the access resources using their `owner.reference`.

## Access startup probe

After creating access resources but before marking the `workspace.status` as `Available`, the controller can probe the resulting route to confirm it's fully wired up. To enable this behavior, configure the `spec.accessStartupProbe` attribute of your access strategy.

See [Dive Deeper: Access Probe](../../dive-deeper/workspace-lifecycle/access-probes.md) for more details.
