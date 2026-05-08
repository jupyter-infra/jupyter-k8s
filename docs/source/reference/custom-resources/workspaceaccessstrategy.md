# WorkspaceAccessStrategy

## WorkspaceAccessStrategy



WorkspaceAccessStrategy is the Schema for the workspaceaccessstrategies API

| Field | Value or Description |
| --- | --- |
| `apiVersion` _string_ | `workspace.jupyter.org/v1alpha1` |
| `kind` _string_ | `WorkspaceAccessStrategy` |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[WorkspaceAccessStrategySpec](#workspaceaccessstrategyspec)_ | Spec defines the desired state of WorkspaceAccessStrategy |
| `status` _[WorkspaceAccessStrategyStatus](#workspaceaccessstrategystatus)_ | Status defines the observed state of WorkspaceAccessStrategy |



## AccessEnvTemplate



AccessEnvTemplate defines a template for environment variables

_Appears in:_
- [PrimaryContainerModifications](#primarycontainermodifications)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the environment variable |  |  |
| `valueTemplate` _string_ | ValueTemplate is a template string for the value<br />Can use variables from the Workspace or AccessStrategy objects<br />but not the Service object |  |  |



## AccessHTTPGetProbe



AccessHTTPGetProbe defines the HTTP GET action for access startup probing.

_Appears in:_
- [AccessStartupProbe](#accessstartupprobe)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `urlTemplate` _string_ | URLTemplate is a Go text/template resolving to the URL to probe.<br />Available variables: .Workspace, .AccessStrategy, .Service<br />(same as accessURLTemplate and accessResourceTemplates). |  |  |
| `additionalSuccessStatusCodes` _integer array_ | AdditionalSuccessStatusCodes extends the default success range (200–399)<br />with extra HTTP status codes that indicate the route is live.<br />Example: [401] for bearer-token auth flows where the auth middleware<br />returns 401 on unauthenticated requests. |  | Optional: \{\} <br /> |



## AccessResourceTemplate



AccessResourceTemplate defines a template for creating Kubernetes resources

_Appears in:_
- [WorkspaceAccessStrategySpec](#workspaceaccessstrategyspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `kind` _string_ | Kind of the Kubernetes resource to create |  |  |
| `apiVersion` _string_ | ApiVersion of the Kubernetes resource |  |  |
| `namePrefix` _string_ | NamePrefix is a prefix for the resource name<br />The name will be constructed as \{NamePrefix\}-\{workspace.metadata.name\} |  |  |
| `template` _string_ | Template is a YAML template string for the resource<br />Template variables include Workspace, AccessStrategy and Service objects |  |  |



## AccessStartupProbe



AccessStartupProbe defines how the controller verifies that access resources
are serving traffic before marking the workspace as Available. Modeled after
corev1.startupProbe — a one-shot gate that passes on the first successful
response and is never checked again.

_Appears in:_
- [WorkspaceAccessStrategySpec](#workspaceaccessstrategyspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `httpGet` _[AccessHTTPGetProbe](#accesshttpgetprobe)_ | HTTPGet specifies an HTTP GET to perform against the access path. |  | Optional: \{\} <br /> |
| `initialDelaySeconds` _integer_ | Number of seconds after access resources are created before probes are initiated.<br />Default: 0. |  | Optional: \{\} <br /> |
| `periodSeconds` _integer_ | How often (in seconds) to perform the probe. Default: 2. Minimum: 1. |  | Optional: \{\} <br /> |
| `timeoutSeconds` _integer_ | Number of seconds after which the probe times out. Default: 5. Minimum: 1. |  | Optional: \{\} <br /> |
| `failureThreshold` _integer_ | Minimum consecutive failures before giving up and marking the workspace as Degraded.<br />Once degraded, the workspace must be stopped and restarted to retry the probe.<br />Default: 30. Minimum: 1. |  | Optional: \{\} <br /> |



## DeploymentModifications



DeploymentModifications defines modifications to apply to deployment spec

_Appears in:_
- [WorkspaceAccessStrategySpec](#workspaceaccessstrategyspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `podModifications` _[PodModifications](#podmodifications)_ | PodModifications describes modifications to apply to the pod template |  | Optional: \{\} <br /> |



## PodModifications



PodModifications defines pod-level modifications

_Appears in:_
- [DeploymentModifications](#deploymentmodifications)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `additionalContainers` _[Container](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#container-v1-core) array_ | AdditionalContainers to add to the pod (sidecars) |  | Optional: \{\} <br /> |
| `volumes` _[Volume](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#volume-v1-core) array_ | Volumes to add to the pod |  | Optional: \{\} <br /> |
| `initContainers` _[Container](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#container-v1-core) array_ | InitContainers to add to the pod |  | Optional: \{\} <br /> |
| `primaryContainerModifications` _[PrimaryContainerModifications](#primarycontainermodifications)_ | PrimaryContainerModifications to apply to the primary container |  | Optional: \{\} <br /> |



## PrimaryContainerModifications



PrimaryContainerModifications defines modifications for the primary container

_Appears in:_
- [PodModifications](#podmodifications)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `volumeMounts` _[VolumeMount](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#volumemount-v1-core) array_ | VolumeMounts to add to the primary container |  | Optional: \{\} <br /> |
| `mergeEnv` _[AccessEnvTemplate](#accessenvtemplate) array_ | MergeEnv defines environment variables to be added to the main container<br />These will be merged with any existing env vars in the Workspace's container |  | Optional: \{\} <br /> |



## WorkspaceAccessStrategySpec



WorkspaceAccessStrategySpec defines the desired state of WorkspaceAccessStrategy

_Appears in:_
- [WorkspaceAccessStrategy](#workspaceaccessstrategy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `displayName` _string_ | DisplayName is a human-readable name for this access strategy |  |  |
| `accessResourceTemplates` _[AccessResourceTemplate](#accessresourcetemplate) array_ | AccessResourceTemplates defines templates for resources created in the routes namespace |  |  |
| `accessURLTemplate` _string_ | AccessURLTemplate is a template string for constructing the workspace access URL<br />Template variables include .Workspace and .AccessStrategy objects<br />If not provided, the AccessURL will not be set in the workspace status<br />Example: "https://example.com/workspace-path/" |  | Optional: \{\} <br /> |
| `bearerAuthURLTemplate` _string_ | BearerAuthURLTemplate is a template string for constructing the bearer auth URL<br />Template variables include .Workspace and .AccessStrategy objects<br />Used by the extension API to generate initial authentication URLs |  | Optional: \{\} <br /> |
| `createConnectionHandler` _string_ | CreateConnectionHandler specifies the default handler for connection creation (e.g., "k8s-native").<br />Used as fallback when CreateConnectionHandlerMap does not contain the requested connection type. |  | Optional: \{\} <br /> |
| `createConnectionHandlerMap` _object (keys:string, values:string)_ | CreateConnectionHandlerMap maps connection types to handler references in "plugin:action" format.<br />Example: \{"vscode-remote": "aws:createSession"\}<br />Falls back to CreateConnectionHandler if the requested connection type is not in this map. |  | Optional: \{\} <br /> |
| `podEventsHandler` _string_ | PodEventsHandler specifies the handler for pod lifecycle events in "plugin:action" format.<br />Example: "aws:ssm-remote-access" |  | Optional: \{\} <br /> |
| `createConnectionContext` _object (keys:string, values:string)_ | CreateConnectionContext contains configuration for the connection handler |  | Optional: \{\} <br /> |
| `podEventsContext` _object (keys:string, values:string)_ | PodEventsContext contains configuration for the pod events handler |  | Optional: \{\} <br /> |
| `deploymentModifications` _[DeploymentModifications](#deploymentmodifications)_ | DeploymentModifications defines modifications to apply to workspace deployments |  | Optional: \{\} <br /> |
| `accessStartupProbe` _[AccessStartupProbe](#accessstartupprobe)_ | AccessStartupProbe defines how the controller verifies that access resources are<br />serving traffic. If not set, access resources are considered ready as soon as they<br />exist in the API server. |  | Optional: \{\} <br /> |



## WorkspaceAccessStrategyStatus



WorkspaceAccessStrategyStatus defines the observed state of WorkspaceAccessStrategy

_Appears in:_
- [WorkspaceAccessStrategy](#workspaceaccessstrategy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#condition-v1-meta) array_ | Conditions represent the latest available observations of the resource's state |  | Optional: \{\} <br /> |


