# Extension API

**Extension API** server runs in the same pod as the controller and serves the Connection APIs under the `connection.workspace.jupyter.org` API group. It registers as an aggregated API server with the Kubernetes API server, so clients can call it via `kubectl` or any K8s client.

## Endpoints

| Resource | Verb | Purpose |
|----------|------|---------|
| {ref}`workspaceconnections <extensionapi-create-connection>` | `POST` | Create a connection URL (bearer token or plugin-delegated) |
| {ref}`connectionaccessreviews <extensionapi-create-connection-access-review>` | `POST` | Check whether a user can connect to a workspace |
| {ref}`bearertokenreviews <extensionapi-create-bearer-token-review>` | `POST` | Validate a bearer token and return the associated user identity |

```{toctree}
:hidden:

architecture
routes
bearer-token
key-rotation
```
