# Workspace

## Workspace



Workspace is the Schema for the workspaces API

| Field | Value or Description |
| --- | --- |
| `apiVersion` _string_ | `workspace.jupyter.org/v1alpha1` |
| `kind` _string_ | `Workspace` |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[WorkspaceSpec](#workspacespec)_ | spec defines the desired state of Workspace |
| `status` _[WorkspaceStatus](#workspacestatus)_ | status defines the observed state of Workspace |



## AccessResourceStatus



AccessResourceStatus defines the status of a resource created from a template

_Appears in:_
- [WorkspaceStatus](#workspacestatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `kind` _string_ | Kind of the Kubernetes resource |  |  |
| `apiVersion` _string_ | APIVersion of the Kubernetes resource |  |  |
| `name` _string_ | Name of the resource |  |  |
| `namespace` _string_ | Namespace of the resource |  |  |



## AccessStrategyRef



AccessStrategyRef defines a reference to a WorkspaceAccessStrategy

_Appears in:_
- [WorkspaceSpec](#workspacespec)
- [WorkspaceTemplateSpec](#workspacetemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the WorkspaceAccessStrategy |  |  |
| `namespace` _string_ | Namespace where the WorkspaceAccessStrategy is located |  | Optional: \{\} <br /> |



## ContainerConfig



ContainerConfig defines container command and args configuration

_Appears in:_
- [WorkspaceSpec](#workspacespec)
- [WorkspaceTemplateSpec](#workspacetemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `command` _string array_ | Command specifies the container command |  |  |
| `args` _string array_ | Args specifies the container arguments |  |  |



## IdleDetectionSpec



IdleDetectionSpec defines idle detection methods

_Appears in:_
- [IdleShutdownSpec](#idleshutdownspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `httpGet` _[IdleHTTPGetAction](#idlehttpgetaction)_ | HTTPGet specifies the HTTP request to perform for idle detection |  | Optional: \{\} <br /> |



## IdleHTTPGetAction



IdleHTTPGetAction extends corev1.HTTPGetAction with transport and response parsing options.

_Appears in:_
- [IdleDetectionSpec](#idledetectionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `path` _string_ | Path to access on the HTTP server. |  | Optional: \{\} <br /> |
| `port` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#intorstring-intstr-util)_ | Name or number of the port to access on the container.<br />Number must be in the range 1 to 65535.<br />Name must be an IANA_SVC_NAME. |  |  |
| `host` _string_ | Host name to connect to, defaults to the pod IP. You probably want to set<br />"Host" in httpHeaders instead. |  | Optional: \{\} <br /> |
| `scheme` _[URIScheme](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#urischeme-v1-core)_ | Scheme to use for connecting to the host.<br />Defaults to HTTP. |  | Optional: \{\} <br /> |
| `httpHeaders` _[HTTPHeader](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#httpheader-v1-core) array_ | Custom headers to set in the request. HTTP allows repeated headers. |  | Optional: \{\} <br /> |
| `transport` _string_ | Transport selects how the operator reaches the endpoint.<br />"podExec" executes curl inside the workspace container (legacy).<br />"network" makes a direct HTTP call from the operator to the workspace Service's ClusterIP. | podExec | Enum: [podExec network] <br />Optional: \{\} <br /> |
| `lastActivityTimestamp` _[IdleLastActivityTimestampSpec](#idlelastactivitytimestampspec)_ | LastActivityTimestamp describes how to extract and parse the last-activity<br />timestamp from the JSON response body. |  | Optional: \{\} <br /> |



## IdleLastActivityTimestampSpec



IdleLastActivityTimestampSpec configures extraction and parsing of a last-activity
timestamp value from an idle-detection HTTP response body.

_Appears in:_
- [IdleHTTPGetAction](#idlehttpgetaction)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `responseBodyPath` _string_ | ResponseBodyPath is a dot-separated path to the timestamp value in the<br />JSON response body (e.g. "last_activity" or "status.lastActive").<br />Default: "lastActiveTimestamp" |  | Optional: \{\} <br /> |
| `format` _string_ | Format specifies how to parse the extracted value.<br />"RFC3339" expects an RFC 3339 timestamp string.<br />"unix" expects epoch seconds (numeric or string). | RFC3339 | Enum: [RFC3339 unix] <br />Optional: \{\} <br /> |



## IdleShutdownSpec



IdleShutdownSpec defines idle shutdown configuration

_Appears in:_
- [WorkspaceSpec](#workspacespec)
- [WorkspaceTemplateSpec](#workspacetemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled indicates if idle shutdown is enabled |  |  |
| `idleTimeoutInMinutes` _integer_ | IdleTimeoutInMinutes specifies idle timeout in minutes |  | Minimum: 1 <br /> |
| `detection` _[IdleDetectionSpec](#idledetectionspec)_ | Detection specifies how to detect idle state |  |  |



## IntegrationParameter



IntegrationParameter is a single user-provided input for template expression resolution.

_Appears in:_
- [IntegrationTemplateRef](#integrationtemplateref)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the parameter, referenced in template expressions as \{\{ .Parameters.<Name> \}\} |  |  |
| `value` _string_ | Value of the parameter (substituted into template expression fields at resolution time) |  | Optional: \{\} <br /> |



## IntegrationStatus



IntegrationStatus reports the operator-observed readiness of a single integration. It follows the
KRO instance-status precedent (https://kro.run/docs/concepts/instances#understanding-status):
a name, a coarse state, and standard Kubernetes conditions carrying the machine-readable detail.

_Appears in:_
- [WorkspaceStatus](#workspacestatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the WorkspaceIntegrationTemplate this status reports on. |  |  |
| `state` _string_ | State is a coarse, human-facing rollup of the conditions below: "Ready" when the integration's<br />last probe succeeded, "Degraded" when it failed. Empty means "not yet probed". |  | Optional: \{\} <br /> |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#condition-v1-meta) array_ | Conditions carry the detailed, machine-readable observation. The operator sets a single<br />"Ready" condition whose Reason is one of Ready/ProbeFailed/PodNotFound/ProbeError and whose<br />Message holds any human-readable detail (e.g. the probe's stderr on failure). |  | Optional: \{\} <br /> |



## IntegrationTemplateRef



IntegrationTemplateRef defines a reference to a WorkspaceIntegrationTemplate.

_Appears in:_
- [WorkspaceSpec](#workspacespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the WorkspaceIntegrationTemplate |  |  |
| `namespace` _string_ | Namespace where the WorkspaceIntegrationTemplate is located |  | Optional: \{\} <br /> |
| `parameters` _[IntegrationParameter](#integrationparameter) array_ | Parameters provided by the user for template expression resolution.<br />Each parameter is referenced in template expressions as \{\{ .Parameters.<Name> \}\}.<br />Names must be unique. |  | MaxItems: 10 <br />Optional: \{\} <br /> |



## ResolvedIntegration



ResolvedIntegration is the frozen resolution record for a single attached integration. It is the
source of truth the controller replays from on unchanged-token reconciles.

_Appears in:_
- [WorkspaceStatus](#workspacestatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the WorkspaceIntegrationTemplate name this record resolves (matches the<br />spec.integrationTemplateRefs[].name that produced it). |  |  |
| `parametersVersion` _string_ | ParametersVersion is a hash of the integration ref's identity and user-supplied parameters<br />(templateRef namespace+name + the sorted parameter map) captured when these values were<br />resolved. A change means the user switched clusters or edited a parameter. |  |  |
| `observedIntegrationTemplateVersion` _string_ | ObservedIntegrationTemplateVersion is "<template.UID>.<template.Generation>" captured when these<br />values were resolved. A change means the admin edited (or replaced) the referenced<br />WorkspaceIntegrationTemplate. Mirrors status.observedAccessStrategyVersion. |  |  |
| `values` _object (keys:string, values:string)_ | Values is the frozen resolution map: each key is a "<resourceRefID>\|<jsonPath>" capture key<br />and each value is the literal string the \{\{ resource ... \}\} expression resolved to at capture<br />time. On replay, the resolver serves these instead of reading the referenced resource. Storing<br />only these substitutions (not the rendered pod spec) keeps the status payload small. |  | Optional: \{\} <br /> |



## StorageSpec



StorageSpec defines the storage configuration for Workspace

_Appears in:_
- [WorkspaceSpec](#workspacespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `storageClassName` _string_ | StorageClassName specifies the storage class to use for persistent storage |  |  |
| `size` _[Quantity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#quantity-resource-api)_ | Size specifies the size of the persistent volume<br />Supports standard Kubernetes resource quantities (e.g., "10Gi", "500Mi", "1Ti")<br />Integer values without units are interpreted as bytes |  |  |
| `mountPath` _string_ | MountPath specifies where to mount the persistent volume in the container<br />Default is /home/jovyan (jovyan is the standard user in Jupyter images) |  |  |



## TemplateRef



TemplateRef defines a reference to a WorkspaceTemplate

_Appears in:_
- [WorkspaceSpec](#workspacespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the WorkspaceTemplate |  |  |
| `namespace` _string_ | Namespace where the WorkspaceTemplate is located<br />When omitted, defaults to the workspace's namespace |  | Optional: \{\} <br /> |



## VolumeSpec



VolumeSpec defines a volume to mount from an existing PVC

_Appears in:_
- [WorkspaceSpec](#workspacespec)
- [WorkspaceTemplateSpec](#workspacetemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is a unique identifier for this volume within the pod (maps to pod.spec.volumes[].name) |  |  |
| `persistentVolumeClaimName` _string_ | PersistentVolumeClaimName is the name of the existing PVC to mount |  |  |
| `mountPath` _string_ | MountPath is the path where the volume should be mounted (Unix-style path, e.g. /data) |  |  |



## WorkspaceSpec



WorkspaceSpec defines the desired state of Workspace

_Appears in:_
- [Workspace](#workspace)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `displayName` _string_ | Display Name of the server |  |  |
| `image` _string_ | Image specifies the container image to use |  |  |
| `desiredStatus` _string_ | DesiredStatus specifies the desired operational status |  | Enum: [Running Stopped] <br /> |
| `ownershipType` _string_ | OwnershipType specifies who can modify the workspace.<br />Public means anyone with RBAC permissions can update/delete the workspace.<br />OwnerOnly means only the creator can update/delete the workspace. |  | Enum: [Public OwnerOnly] <br />Optional: \{\} <br /> |
| `accessType` _string_ | AccessType specifies who can connect to the workspace.<br />Public means anyone with RBAC permissions can connect to workspace.<br />OwnerOnly means only the creator can connect to the workspace. |  | Enum: [Public OwnerOnly] <br />Optional: \{\} <br /> |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#resourcerequirements-v1-core)_ | Resources specifies the resource requirements |  |  |
| `storage` _[StorageSpec](#storagespec)_ | Storage specifies the storage configuration |  |  |
| `volumes` _[VolumeSpec](#volumespec) array_ | Volumes specifies additional volumes to mount from existing PersistantVolumeClaims |  |  |
| `containerConfig` _[ContainerConfig](#containerconfig)_ | ContainerConfig specifies container command and args configuration |  |  |
| `env` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#envvar-v1-core) array_ | Env specifies environment variables for the workspace container<br />When a template is used, template's BaseEnv vars are merged (workspace vars take precedence by name) |  | Optional: \{\} <br /> |
| `nodeSelector` _object (keys:string, values:string)_ | NodeSelector specifies node selection constraints for the workspace pod |  |  |
| `affinity` _[Affinity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#affinity-v1-core)_ | Affinity specifies node affinity and anti-affinity rules for the workspace pod |  |  |
| `tolerations` _[Toleration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#toleration-v1-core) array_ | Tolerations specifies tolerations for the workspace pod to schedule on nodes with matching taints |  |  |
| `lifecycle` _[Lifecycle](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#lifecycle-v1-core)_ | Lifecycle specifies actions that the management system should take<br />in response to container lifecycle events (for instance, lifecycle hooks) |  |  |
| `readinessProbe` _[Probe](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#probe-v1-core)_ | ReadinessProbe specifies the readiness probe for the main workspace container. |  | Optional: \{\} <br /> |
| `accessStrategy` _[AccessStrategyRef](#accessstrategyref)_ | AccessStrategy specifies the WorkspaceAccessStrategy to use |  | Optional: \{\} <br /> |
| `templateRef` _[TemplateRef](#templateref)_ | TemplateRef references a WorkspaceTemplate to use as base configuration<br />When set, template provides defaults and workspace spec fields act as overrides |  | Optional: \{\} <br /> |
| `idleShutdown` _[IdleShutdownSpec](#idleshutdownspec)_ | IdleShutdown specifies idle shutdown configuration |  | Optional: \{\} <br /> |
| `appType` _string_ | AppType specifies the application type for this workspace |  | Optional: \{\} <br /> |
| `serviceAccountName` _string_ | ServiceAccountName specifies the name of the ServiceAccount to use for the workspace pod |  | Optional: \{\} <br /> |
| `podSecurityContext` _[PodSecurityContext](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#podsecuritycontext-v1-core)_ | PodSecurityContext specifies pod-level security context<br />Overrides template defaults when specified |  | Optional: \{\} <br /> |
| `containerSecurityContext` _[SecurityContext](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#securitycontext-v1-core)_ | ContainerSecurityContext specifies container-level security context for the main workspace container<br />Takes precedence over PodSecurityContext for the main container<br />Overrides template defaults when specified |  | Optional: \{\} <br /> |
| `initContainers` _[Container](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#container-v1-core) array_ | InitContainers specifies init containers to run before the workspace container starts<br />When a template is used, template's DefaultInitContainers are applied if workspace has none<br />Requires AllowCustomInitContainers=true on the template to specify custom init containers |  | MaxItems: 10 <br />Optional: \{\} <br /> |
| `integrationTemplateRefs` _[IntegrationTemplateRef](#integrationtemplateref) array_ | IntegrationTemplateRefs attaches one or more WorkspaceIntegrationTemplates that inject runtime<br />capabilities (sidecars, volumes, env vars) into the workspace pod with template-based<br />dynamic resolution. Each entry is the user's REQUEST only -- a reference to a<br />WorkspaceIntegrationTemplate plus parameters -- never the resolved sidecars.<br />The workspace controller resolves each integration against its referenced resources (e.g. a<br />RayCluster) only when the input token -- hash(templateRef + parameters) -- changes. On an<br />unchanged token (external drift in a referenced resource, an idle reconcile), the controller<br />rebuilds the pod template from the frozen values recorded in status.resolvedIntegrations<br />instead of re-reading the referenced resource, so drift never rolls the running pod. No<br />WorkspaceIntegration child object is created. |  | MaxItems: 1 <br />Optional: \{\} <br /> |



## WorkspaceStatus



WorkspaceStatus defines the observed state of Workspace.

_Appears in:_
- [Workspace](#workspace)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `deploymentName` _string_ | DeploymentName is the name of the deployment managing the Workspace pods |  | Optional: \{\} <br /> |
| `serviceName` _string_ | ServiceName is the name of the service exposing the Workspace |  | Optional: \{\} <br /> |
| `accessURL` _string_ | AccessURL is the URL at which the workspace can be accessed |  | Optional: \{\} <br /> |
| `applicationBasePath` _string_ | ApplicationBasePath is the resolved routing prefix for the workspace application.<br />Set during access-resources reconciliation; used by idle detection to construct<br />the full endpoint path. |  | Optional: \{\} <br /> |
| `accessResourceSelector` _string_ | AccessResourceSelector is a label selector that can be used to find all resources<br />created from the workspace's AccessStrategy templates |  | Optional: \{\} <br /> |
| `accessResources` _[AccessResourceStatus](#accessresourcestatus) array_ | AccessResources provides status details of individual resources created from<br />the workspace's AccessStrategy templates |  | Optional: \{\} <br /> |
| `observedAccessStrategyVersion` _string_ | ObservedAccessStrategyVersion is a token capturing the identity and<br />version of the AccessStrategy last evaluated during workspace<br />reconciliation. The controller resets probe state when this value changes. |  | Optional: \{\} <br /> |
| `accessStartupProbeSucceeded` _boolean_ | AccessStartupProbeSucceeded indicates whether the access startup probe<br />has passed. Set to true when the probe succeeds; reset to false when<br />the workspace stops. |  | Optional: \{\} <br /> |
| `accessStartupProbeFailures` _integer_ | AccessStartupProbeFailures tracks the number of consecutive failed access<br />startup probe attempts. Set by the controller during the probing phase;<br />cleared (nil) on success or when the workspace stops. |  | Optional: \{\} <br /> |
| `earliestNextProbeTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#time-v1-meta)_ | EarliestNextProbeTime is the earliest wall-clock time at which the next<br />access startup probe may fire. Set by the controller after each probe<br />attempt to enforce spacing; survives watch-triggered re-reconciliations. |  | Optional: \{\} <br /> |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#condition-v1-meta) array_ | Conditions represent the current state of the Workspace resource.<br />Each condition has a unique type and reflects the status of a specific aspect of the resource.<br />Standard condition types include:<br />- "Available": the resource is fully functional and ready to use<br />- "Progressing": the resource is being created, updated, or stopped<br />- "Degraded": the resource failed to reach or maintain its desired state<br />- "Stopped": the workspace has been stopped and resources scaled down<br />The status of each condition is one of True, False, or Unknown. |  | Optional: \{\} <br /> |
| `integrationStatuses` _[IntegrationStatus](#integrationstatus) array_ | IntegrationStatuses reports the operator-observed readiness of each integration applied to the<br />workspace, as observed by the periodic status probe. One entry per integration that defines<br />a statusProbe. Absence of an entry means "not yet probed"; a present entry always reflects<br />an actual observation (Ready + Reason). Named after the k8s pod.status.containerStatuses<br />convention (<thing>Statuses []<Thing>Status), matching the IntegrationStatus element type. |  | Optional: \{\} <br /> |
| `resolvedIntegrations` _[ResolvedIntegration](#resolvedintegration) array_ | ResolvedIntegrations records the frozen output of the last successful integration resolution,<br />one entry per attached integration. It is the operator's private freeze store: on an unchanged<br />input token the controller rebuilds the pod template from these values WITHOUT re-reading the<br />referenced resource, so external drift never re-resolves and never rolls the running pod. Each<br />entry stores only the resolved template substitutions (a small string map), never the fully<br />rendered pod spec -- the pod SHAPE still comes from the live WorkspaceIntegrationTemplate; only<br />the resolved VALUES are frozen here. Written by the controller via the status subresource; it<br />is never user-authored. |  | Optional: \{\} <br /> |


