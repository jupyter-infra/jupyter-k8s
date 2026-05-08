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
| `httpGet` _[HTTPGetAction](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#httpgetaction-v1-core)_ | HTTPGet specifies the HTTP request to perform for idle detection |  | Optional: \{\} <br /> |



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
| `accessStrategy` _[AccessStrategyRef](#accessstrategyref)_ | AccessStrategy specifies the WorkspaceAccessStrategy to use |  | Optional: \{\} <br /> |
| `templateRef` _[TemplateRef](#templateref)_ | TemplateRef references a WorkspaceTemplate to use as base configuration<br />When set, template provides defaults and workspace spec fields act as overrides |  | Optional: \{\} <br /> |
| `idleShutdown` _[IdleShutdownSpec](#idleshutdownspec)_ | IdleShutdown specifies idle shutdown configuration |  | Optional: \{\} <br /> |
| `appType` _string_ | AppType specifies the application type for this workspace |  | Optional: \{\} <br /> |
| `serviceAccountName` _string_ | ServiceAccountName specifies the name of the ServiceAccount to use for the workspace pod |  | Optional: \{\} <br /> |
| `podSecurityContext` _[PodSecurityContext](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#podsecuritycontext-v1-core)_ | PodSecurityContext specifies pod-level security context<br />Overrides template defaults when specified |  | Optional: \{\} <br /> |
| `containerSecurityContext` _[SecurityContext](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#securitycontext-v1-core)_ | ContainerSecurityContext specifies container-level security context for the main workspace container<br />Takes precedence over PodSecurityContext for the main container<br />Overrides template defaults when specified |  | Optional: \{\} <br /> |
| `initContainers` _[Container](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#container-v1-core) array_ | InitContainers specifies init containers to run before the workspace container starts<br />When a template is used, template's DefaultInitContainers are applied if workspace has none<br />Requires AllowCustomInitContainers=true on the template to specify custom init containers |  | MaxItems: 10 <br />Optional: \{\} <br /> |



## WorkspaceStatus



WorkspaceStatus defines the observed state of Workspace.

_Appears in:_
- [Workspace](#workspace)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `deploymentName` _string_ | DeploymentName is the name of the deployment managing the Workspace pods |  | Optional: \{\} <br /> |
| `serviceName` _string_ | ServiceName is the name of the service exposing the Workspace |  | Optional: \{\} <br /> |
| `accessURL` _string_ | AccessURL is the URL at which the workspace can be accessed |  | Optional: \{\} <br /> |
| `accessResourceSelector` _string_ | AccessResourceSelector is a label selector that can be used to find all resources<br />created from the workspace's AccessStrategy templates |  | Optional: \{\} <br /> |
| `accessResources` _[AccessResourceStatus](#accessresourcestatus) array_ | AccessResources provides status details of individual resources created from<br />the workspace's AccessStrategy templates |  | Optional: \{\} <br /> |
| `observedAccessStrategyVersion` _string_ | ObservedAccessStrategyVersion is a token capturing the identity and<br />version of the AccessStrategy last evaluated during workspace<br />reconciliation. The controller resets probe state when this value changes. |  | Optional: \{\} <br /> |
| `accessStartupProbeSucceeded` _boolean_ | AccessStartupProbeSucceeded indicates whether the access startup probe<br />has passed. Set to true when the probe succeeds; reset to false when<br />the workspace stops. |  | Optional: \{\} <br /> |
| `accessStartupProbeFailures` _integer_ | AccessStartupProbeFailures tracks the number of consecutive failed access<br />startup probe attempts. Set by the controller during the probing phase;<br />cleared (nil) on success or when the workspace stops. |  | Optional: \{\} <br /> |
| `earliestNextProbeTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#time-v1-meta)_ | EarliestNextProbeTime is the earliest wall-clock time at which the next<br />access startup probe may fire. Set by the controller after each probe<br />attempt to enforce spacing; survives watch-triggered re-reconciliations. |  | Optional: \{\} <br /> |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#condition-v1-meta) array_ | Conditions represent the current state of the Workspace resource.<br />Each condition has a unique type and reflects the status of a specific aspect of the resource.<br />Standard condition types include:<br />- "Available": the resource is fully functional and ready to use<br />- "Progressing": the resource is being created, updated, or stopped<br />- "Degraded": the resource failed to reach or maintain its desired state<br />- "Stopped": the workspace has been stopped and resources scaled down<br />The status of each condition is one of True, False, or Unknown. |  | Optional: \{\} <br /> |


