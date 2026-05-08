# Access URL

The access strategy controls how users discover the URL to reach their workspace.

## `accessURLTemplate`

A Go template that resolves to the workspace's access URL, stored in `workspace.status.accessURL`:

```yaml
spec:
  accessURLTemplate: "https://jupyter.example.com/workspaces/{{ .Workspace.Namespace }}/{{ .Workspace.Name }}/"
```

The fully resolved URL appears in the `workspace.status`, and points to the router's public endpoint with the workspace's unique path prefix.

To find their workspace's access URL, a workspace user can run:

```bash
kubectl get Workspace <workspace-name> -o yaml
```

## `bearerAuthURLTemplate`

For bearer-token access strategies, this template constructs the initial authentication URL. The Extension API uses it when generating connection URLs:

```yaml
spec:
  bearerAuthURLTemplate: "https://jupyter.example.com/workspaces/{{ .Workspace.Namespace }}/{{ .Workspace.Name }}/auth?token={{ .Token }}"
```

**Extension API** populates the `{{ .Token }}` variable with a signed bearer token at connection creation time.

To generate a bearer token URL, a workspace user can run:

```bash
kubectl create -f - <<EOF
apiVersion: connection.workspace.jupyter.org/v1alpha1
kind: WorkspaceConnection
metadata:
  namespace: <workspace-namespace>
spec:
  workspaceName: <workspace-name>
  workspaceConnectionType: web-ui
EOF
```

## Connection handlers

For more advanced connection types (remote IDE access, tunneling), the access strategy maps connection types to [plugin](../../integrations/plugins/index.md) handlers:

```yaml
spec:
  createConnectionHandler: "k8s-native"
  createConnectionHandlerMap:
    vscode-remote: "aws:createSession"
```

In this example, when a workspace user requests a connection of type `vscode-remote`, **Extension API** dispatches to the `aws` plugin's `createSession` action.
