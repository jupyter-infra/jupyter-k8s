# Connection

## WorkspaceConnectionRequest



WorkspaceConnectionRequest represents the request body for creating a workspace connection

| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `connection.workspace.jupyter.org/v1alpha1` |
| `kind` _string_ | `WorkspaceConnectionRequest` |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[WorkspaceConnectionRequestSpec](#workspaceconnectionrequestspec)_ |  |



## WorkspaceConnectionRequestSpec



WorkspaceConnectionRequestSpec represents the spec of a workspace connection request

_Appears in:_
- [WorkspaceConnectionRequest](#workspaceconnectionrequest)
- [WorkspaceConnectionResponse](#workspaceconnectionresponse)

| Field | Description |
| --- | --- |
| `workspaceName` _string_ |  |
| `workspaceConnectionType` _string_ |  |



## WorkspaceConnectionResponse



WorkspaceConnectionResponse represents the response for a workspace connection

| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `connection.workspace.jupyter.org/v1alpha1` |
| `kind` _string_ | `WorkspaceConnectionResponse` |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[WorkspaceConnectionRequestSpec](#workspaceconnectionrequestspec)_ |  |
| `status` _[WorkspaceConnectionResponseStatus](#workspaceconnectionresponsestatus)_ |  |



## WorkspaceConnectionResponseStatus



WorkspaceConnectionResponseStatus represents the status of a workspace connection response

_Appears in:_
- [WorkspaceConnectionResponse](#workspaceconnectionresponse)

| Field | Description |
| --- | --- |
| `workspaceConnectionType` _string_ |  |
| `workspaceConnectionUrl` _string_ |  |



