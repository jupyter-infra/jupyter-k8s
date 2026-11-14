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

The connection types a workspace accepts depend on its access strategy, which an administrator creates and a workspace user is not necessarily allowed to read. The controller derives the types the strategy configures and writes them to the workspace as `status.connectionTypes`: `web-ui` when the strategy sets `bearerAuthURLTemplate`, `ssh-over-websocket` when it sets `webSocketURLTemplate`, and the `{ide}-remote` keys of `createConnectionHandlerMap`. A remote type that only the strategy's default `createConnectionHandler` serves is not listed, since that handler accepts any `{ide}-remote` name. A client picks a type from this list without reading the access strategy:

```bash
kubectl get workspace alice-workspace -n alice-team -o jsonpath='{.status.connectionTypes}'
```

The controller writes the list once the workspace's compute and Service are ready, rewrites it whenever it reconciles the running workspace against its access strategy, and clears it when the workspace stops or drops its access strategy. Whether the workspace is ready to connect is reported by `status.conditions[Available]`, not by this list.

Unlike a normal Kubernetes resource, a WorkspaceConnection does not persist in ETCD, and a user cannot list connections with `kubectl get WorkspaceConnections`.

```{toctree}
:hidden:

web-access
remote-access
access-review
token-review
```
