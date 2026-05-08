# Routes

**Extension API** exposes three endpoints, all under the aggregated API path:

```
/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/{namespace}/{resource}
```

(extensionapi-create-connection)=
## POST /workspaceconnections

Creates a connection URL for a workspace.

**Request:**

```json
{
  "apiVersion": "connection.workspace.jupyter.org/v1alpha1",
  "kind": "WorkspaceConnection",
  "metadata": {
    "namespace": "team-notebooks"
  },
  "spec": {
    "workspaceName": "my-notebook",
    "workspaceConnectionType": "web-ui"
  }
}
```

Valid connection types:
- `web-ui` — generates a bearer token URL for browser access
- `{ide}-remote` (e.g. `vscode-remote`, `cursor-remote`) — delegates to a plugin handler

**Flow:**

1. Extracts the authenticated user from request headers (set by the K8s API server proxy).
2. Checks RBAC (`SubjectAccessReview`) and `workspace.spec.accessType` to authorize the connection.
3. Verifies the workspace is `Available`.
4. Resolves the connection handler (see below).
5. Returns the connection URL.

**Response:**

```json
{
  "status": {
    "workspaceConnectionType": "web-ui",
    "workspaceConnectionUrl": "https://..."
  }
}
```

### Handler resolution

**Extension API** uses the workspace's access strategy to determine how to generate the connection URL.

#### [Browser-based Connections](../../concepts/connections/web-access.md)

**Extension API** uses the k8s-native path:
1. Checks that `spec.bearerAuthURLTemplate` is defined on the access strategy.
2. Creates a signer from the `signerFactory` (k8s-native HMAC or plugin-delegated).
3. Signs a short-lived bearer token with the user's identity, scoped to the workspace path and domain.
4. Renders the URL from `bearerAuthURLTemplate` and appends `?token=<jwt>`.

#### [Desktop IDE-based Connections](../../concepts/connections/remote-access.md)

**Extension API** delegates to a [plugin](../../integrations/plugins/index.md):
1. Looks up the connection type in the access strategy's `createConnectionHandlerMap`.
2. Falls back to `createConnectionHandler` if no map entry matches.
3. Parses the handler reference (format: `plugin:action`, e.g. `aws:createSession`).
4. Resolves the plugin name to its HTTP endpoint from the configured `PluginEndpoints`.
5. Resolves the `createConnectionContext` (static values + dynamic lookups like pod UID).
6. Calls the plugin's `CreateSession` endpoint with workspace details and resolved context.
7. Returns the plugin's connection URL.

```yaml
# Access strategy example
spec:
  bearerAuthURLTemplate: "https://workspaces.example.com/workspaces/..."
  createConnectionHandlerMap:
    vscode-remote: "aws:createSession"
    cursor-remote: "aws:createSession"
  createConnectionContext:
    ssmDocumentName: "my-ssm-document"
    podUid: "extensionapi::PodUid()"
```

(extensionapi-create-connection-access-review)=
## POST /connectionaccessreviews

Checks whether a user can connect to a specific workspace. Used by **Auth middleware** on session establishment and refresh.

**Request:**

```json
{
  "apiVersion": "connection.workspace.jupyter.org/v1alpha1",
  "kind": "ConnectionAccessReview",
  "metadata": {
    "namespace": "team-notebooks"
  },
  "spec": {
    "workspaceName": "my-notebook",
    "user": "alice",
    "groups": ["team-a"],
    "uid": "alice-uid"
  }
}
```

**Flow:**

1. Performs a [SubjectAccessReview](https://dev-k8sref-io.web.app/docs/authorization/subjectaccessreview-v1/) — checks the user has `create` permission on `workspaceconnections` in the namespace.
2. Fetches the workspace and checks `spec.accessType` — if `OwnerOnly`, only the workspace creator is allowed.
3. Returns `allowed: true/false` with a reason.

**Response:**

```json
{
  "status": {
    "allowed": true,
    "notFound": false,
    "reason": "RBAC allowed and workspace is Public"
  }
}
```

(extensionapi-create-bearer-token-review)=
## POST /bearertokenreviews

Validates a bearer token and returns the authenticated user identity. Used by **Auth middleware** when handling `/bearer-auth` requests.

**Request:**

```json
{
  "apiVersion": "connection.workspace.jupyter.org/v1alpha1",
  "kind": "BearerTokenReview",
  "spec": {
    "token": "<jwt>"
  }
}
```

**Flow:**

1. Validates the JWT signature using the `kid` header to look up the signing key.
2. Checks token expiration.
3. Verifies the token type is `bootstrap` (rejects session tokens).
4. Returns the authenticated identity (username, groups, UID, extra) and the scoped path/domain.

**Response:**

```json
{
  "status": {
    "authenticated": true,
    "user": {
      "username": "alice",
      "uid": "alice-uid",
      "groups": ["team-a"]
    },
    "path": "/workspaces/team-notebooks/my-notebook",
    "domain": "workspaces.example.com"
  }
}
```
