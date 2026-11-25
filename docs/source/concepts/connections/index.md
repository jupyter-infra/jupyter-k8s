# Connections

A **Connection** is a short-lived resource that provides a user with a URL (or session) to access their workspace.

A workspace user can generate a Connection to a workspace with:

```bash
kubectl create -f - <<EOF
apiVersion: connection.workspace.jupyter.org/v1alpha1
kind: WorkspaceConnection
metadata:
  namespace: alice-team
spec:
  workspaceName: alice-workspace
  workspaceConnectionType: web-ui
EOF
```

The Kubernetes API server authorizes this request against the RBAC permissions of the caller. The following RBAC rule is sufficient:
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
rules:
  - apiGroups: ["connection.workspace.jupyter.org"]
    resources: ["workspaceconnections"]
    verbs: ["create"]
```

After authorizing the request, the API server forwards it to the **Extension API**.

The final access decision depends on the `workspace.spec.accessType` attribute.
- `Public` allows any Kubernetes user to access
- `OwnerOnly` only allows the user who created the workspace to access

When access should be granted, the **Extension API** responds with a URL. Otherwise it returns an error.

If the workspace does not exist, or if its `status.conditions[Available]` is not `True`, **Extension API** also returns an error.

The connection types a workspace accepts depend on its [access strategy](../access-strategies/index.md), which an administrator creates and a workspace user is not necessarily allowed to read. The controller derives the types **Extension API** serves for the strategy and writes them to the workspace as `status.connectionTypes`. `web-ui` is listed when the strategy sets `bearerAuthURLTemplate`, and `ssh-over-websocket` when it sets `webSocketURLTemplate`. The strategy's `createConnectionHandlerMap` maps each `{ide}-remote` type to the handler that creates its connection, either `k8s-native` or a [plugin](../../integrations/plugins/index.md) action such as `aws:createSession`. A mapped `{ide}-remote` type is listed when **Extension API** can serve its handler: `k8s-native` needs `webSocketURLTemplate`, and a plugin needs to be configured in `PluginEndpoints` with `createSession` as the action or no action at all, the only forms **Extension API** serves. A type that only the strategy's default `createConnectionHandler` serves is not listed, since that handler accepts any `{ide}-remote` name. Nothing is listed while **Extension API** is disabled. `web-ui`, `ssh-over-websocket` and the map keys handled by `k8s-native` are listed only when the operator is started with a JWT secret name, `extensionApi.jwtSecret.enable` in the operator chart, and the strategy's default `createConnectionHandler` is empty or `k8s-native`. The list follows that configuration, not whether a signing key has been written yet.

`ssh-over-websocket` means any desktop editor that speaks SSH can connect over the generic WebSocket route. A listed `{ide}-remote` type means that editor connects through its own handler, typically a plugin. A client checks for `ssh-over-websocket` first and only then looks up a specific `{ide}-remote` type. It picks from this list without reading the access strategy:

```bash
kubectl get workspace alice-workspace -n alice-team -o jsonpath='{.status.connectionTypes}'
```

The controller writes the list once the workspace's Deployment and Service are ready and keeps it current while they stay ready, so an edit to the access strategy shows up in the list. It clears the list when the workspace stops or no longer references an access strategy. `status.conditions[Available]`, not this list, reports whether the workspace is ready to connect.

Unlike a normal Kubernetes resource, a WorkspaceConnection does not persist in ETCD, and a user cannot list connections with `kubectl get WorkspaceConnections`.

```{toctree}
:hidden:

web-access
remote-access
access-review
token-review
```
