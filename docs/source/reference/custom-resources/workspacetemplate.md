# WorkspaceTemplate

## WorkspaceTemplate



WorkspaceTemplate is the Schema for the workspacetemplates API
Templates define reusable, secure-by-default configurations for workspaces.
Template spec can be updated; existing workspaces keep their configuration (lazy application).

| Field | Value or Description |
| --- | --- |
| `apiVersion` _string_ | `workspace.jupyter.org/v1alpha1` |
| `kind` _string_ | `WorkspaceTemplate` |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[WorkspaceTemplateSpec](#workspacetemplatespec)_ |  |
| `status` _[WorkspaceTemplateStatus](#workspacetemplatestatus)_ |  |



## EnvRequirement



EnvRequirement defines a validation rule for a workspace environment variable

_Appears in:_
- [WorkspaceTemplateSpec](#workspacetemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the environment variable name to validate |  | MinLength: 1 <br />Required: \{\} <br /> |
| `required` _boolean_ | Required indicates whether the environment variable must be present on the workspace | false | Optional: \{\} <br /> |
| `regex` _string_ | Regex is a regular expression the environment variable value must match<br />If empty, any value is accepted |  | Optional: \{\} <br /> |



## IdleShutdownOverridePolicy



IdleShutdownOverridePolicy defines idle shutdown override constraints

_Appears in:_
- [WorkspaceTemplateSpec](#workspacetemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `allow` _boolean_ | Allow controls whether workspaces can override idle shutdown | true | Optional: \{\} <br /> |
| `minIdleTimeoutInMinutes` _integer_ | MinIdleTimeoutInMinutes is the minimum allowed timeout |  | Optional: \{\} <br /> |
| `maxIdleTimeoutInMinutes` _integer_ | MaxIdleTimeoutInMinutes is the maximum allowed timeout |  | Optional: \{\} <br /> |



## LabelRequirement



LabelRequirement defines a validation rule for a workspace label

_Appears in:_
- [WorkspaceTemplateSpec](#workspacetemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `key` _string_ | Key is the label key to validate |  | MinLength: 1 <br />Required: \{\} <br /> |
| `required` _boolean_ | Required indicates whether the label must be present on the workspace | false | Optional: \{\} <br /> |
| `regex` _string_ | Regex is a regular expression the label value must match<br />If empty, any value is accepted |  | Optional: \{\} <br /> |



## ResourceBounds



ResourceBounds defines minimum and maximum resource limits for any resource type.
Uses Kubernetes ResourceName as keys to support vendor-agnostic resource specifications.

_Appears in:_
- [WorkspaceTemplateSpec](#workspacetemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `resources` _object (keys:[ResourceName](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#resourcename-v1-core), values:[ResourceRange](#resourcerange))_ | Resources defines min/max bounds for any resource type.<br />Map keys use Kubernetes resource names following these conventions:<br />Standard resources (no vendor prefix):<br />  - cpu: CPU cores (e.g., "100m", "2")<br />  - memory: RAM (e.g., "128Mi", "4Gi")<br />Extended resources (vendor-prefixed):<br />  - nvidia.com/gpu: NVIDIA GPUs<br />  - amd.com/gpu: AMD GPUs<br />  - intel.com/gpu: Intel GPUs<br />  - nvidia.com/mig-1g.5gb: NVIDIA MIG profile (1 GPU instance, 5GB)<br />  - nvidia.com/mig-2g.10gb: NVIDIA MIG profile (2 GPU instances, 10GB)<br />Custom accelerators follow the pattern: vendor.example/resource-name |  | Optional: \{\} <br /> |



## ResourceRange



ResourceRange defines min and max for a resource
NOTE: CEL validation for min <= max is not possible due to resource.Quantity type limitations
Validation is enforced at runtime in the template resolver

_Appears in:_
- [ResourceBounds](#resourcebounds)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `min` _[Quantity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#quantity-resource-api)_ | Min is the minimum allowed value |  | Required: \{\} <br /> |
| `max` _[Quantity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#quantity-resource-api)_ | Max is the maximum allowed value |  | Required: \{\} <br /> |



## StorageConfig



StorageConfig defines storage settings
NOTE: CEL validation for minSize <= maxSize is not possible due to resource.Quantity type limitations
Validation is enforced at runtime in the template resolver

_Appears in:_
- [WorkspaceTemplateSpec](#workspacetemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `defaultSize` _[Quantity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#quantity-resource-api)_ | DefaultSize is the default storage size | 10Gi | Optional: \{\} <br /> |
| `minSize` _[Quantity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#quantity-resource-api)_ | MinSize is the minimum allowed storage size |  | Optional: \{\} <br /> |
| `maxSize` _[Quantity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#quantity-resource-api)_ | MaxSize is the maximum allowed storage size |  | Optional: \{\} <br /> |
| `defaultStorageClassName` _string_ | DefaultStorageClassName is the default storage class name |  | Optional: \{\} <br /> |
| `defaultMountPath` _string_ | DefaultMountPath is the default mount path for the storage | /home/jovyan | Optional: \{\} <br /> |



## TemplateLabel



TemplateLabel defines a label key-value pair to add to workspaces

_Appears in:_
- [WorkspaceTemplateSpec](#workspacetemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `key` _string_ | Key is the label key |  | MinLength: 1 <br />Required: \{\} <br /> |
| `value` _string_ | Value is the label value |  | Required: \{\} <br /> |



## WorkspaceTemplateSpec



WorkspaceTemplateSpec defines the desired state of WorkspaceTemplate

_Appears in:_
- [WorkspaceTemplate](#workspacetemplate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `displayName` _string_ | DisplayName is the human-readable name of this template |  | MaxLength: 100 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `description` _string_ | Description provides additional information about this template |  | MaxLength: 500 <br />Optional: \{\} <br /> |
| `defaultImage` _string_ | DefaultImage is the default container image for workspaces using this template |  | MaxLength: 500 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `allowedImages` _string array_ | AllowedImages is a list of container images that can be used with this template<br />If empty, only DefaultImage is allowed (secure by default)<br />If populated, workspace can override image with any from this list |  | MaxItems: 50 <br />Optional: \{\} <br /> |
| `allowCustomImages` _boolean_ | AllowCustomImages allows workspaces to use any container image, bypassing the AllowedImages restriction<br />When true, workspaces can specify any image regardless of the AllowedImages list | false | Optional: \{\} <br /> |
| `defaultResources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#resourcerequirements-v1-core)_ | DefaultResources specifies the default resource requirements |  | Optional: \{\} <br /> |
| `resourceBounds` _[ResourceBounds](#resourcebounds)_ | ResourceBounds defines the min/max boundaries for resource overrides |  | Optional: \{\} <br /> |
| `primaryStorage` _[StorageConfig](#storageconfig)_ | PrimaryStorage defines storage configuration |  | Optional: \{\} <br /> |
| `defaultContainerConfig` _[ContainerConfig](#containerconfig)_ | DefaultContainerConfig specifies default container command and args configuration |  | Optional: \{\} <br /> |
| `baseEnv` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#envvar-v1-core) array_ | BaseEnv specifies environment variables to add to workspaces using this template<br />Variables are added during defaulting if no variable with the same name exists on the workspace |  | MaxItems: 50 <br />Optional: \{\} <br /> |
| `envRequirements` _[EnvRequirement](#envrequirement) array_ | EnvRequirements specifies validation rules for workspace environment variables |  | MaxItems: 50 <br />Optional: \{\} <br /> |
| `allowSecondaryStorages` _boolean_ | AllowSecondaryStorages controls whether workspaces using this template<br />can mount additional storage volumes beyond the primary storage | true | Optional: \{\} <br /> |
| `defaultVolumes` _[VolumeSpec](#volumespec) array_ | DefaultVolumes specifies default additional volumes for workspaces using this template<br />Volumes are applied during defaulting only if the workspace does not specify any volumes<br />Each volume references a pre-existing PVC by name in the workspace's namespace |  | MaxItems: 10 <br />Optional: \{\} <br /> |
| `defaultNodeSelector` _object (keys:string, values:string)_ | DefaultNodeSelector specifies default node selection constraints |  | Optional: \{\} <br /> |
| `defaultAffinity` _[Affinity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#affinity-v1-core)_ | DefaultAffinity specifies default node affinity and anti-affinity rules |  | Optional: \{\} <br /> |
| `defaultTolerations` _[Toleration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#toleration-v1-core) array_ | DefaultTolerations specifies default tolerations for scheduling on nodes with taints |  | Optional: \{\} <br /> |
| `defaultOwnershipType` _string_ | DefaultOwnershipType specifies default ownershipType for workspaces using this template<br />OwnershipType controls which users may edit/delete the workspace | Public | Enum: [Public OwnerOnly] <br />Optional: \{\} <br /> |
| `baseLabels` _[TemplateLabel](#templatelabel) array_ | BaseLabels specifies labels to add to workspaces using this template<br />Labels are added during defaulting if not already present on the workspace |  | MaxItems: 50 <br />Optional: \{\} <br /> |
| `labelRequirements` _[LabelRequirement](#labelrequirement) array_ | LabelRequirements specifies validation rules for workspace labels |  | MaxItems: 50 <br />Optional: \{\} <br /> |
| `defaultIdleShutdown` _[IdleShutdownSpec](#idleshutdownspec)_ | DefaultIdleShutdown provides default idle shutdown configuration<br />Includes timeout, detection endpoint, and enable/disable |  | Optional: \{\} <br /> |
| `idleShutdownOverrides` _[IdleShutdownOverridePolicy](#idleshutdownoverridepolicy)_ | IdleShutdownOverrides controls override behavior and bounds |  | Optional: \{\} <br /> |
| `defaultAccessType` _string_ | DefaultAccessType specifies the default accessType for workspaces using this template<br />AccessType controls which users may create connections to the workspace. | Public | Enum: [Public OwnerOnly] <br />Optional: \{\} <br /> |
| `defaultAccessStrategy` _[AccessStrategyRef](#accessstrategyref)_ | DefaultAccessStrategy specifies the default access strategy for workspaces using this template |  | Optional: \{\} <br /> |
| `defaultLifecycle` _[Lifecycle](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#lifecycle-v1-core)_ | DefaultLifecycle specifies default lifecycle hooks for workspaces using this template |  | Optional: \{\} <br /> |
| `defaultPodSecurityContext` _[PodSecurityContext](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#podsecuritycontext-v1-core)_ | DefaultPodSecurityContext specifies default pod-level security context |  | Optional: \{\} <br /> |
| `defaultContainerSecurityContext` _[SecurityContext](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#securitycontext-v1-core)_ | DefaultContainerSecurityContext specifies default container-level security context for the main workspace container |  | Optional: \{\} <br /> |
| `defaultInitContainers` _[Container](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#container-v1-core) array_ | DefaultInitContainers specifies default init containers for workspaces using this template<br />Applied during defaulting if the workspace does not specify any init containers |  | MaxItems: 10 <br />Optional: \{\} <br /> |
| `allowCustomInitContainers` _boolean_ | AllowCustomInitContainers controls whether workspaces using this template<br />can specify custom init containers beyond the template defaults | false | Optional: \{\} <br /> |
| `appType` _string_ | AppType specifies the application type for workspaces using this template |  | Optional: \{\} <br /> |



## WorkspaceTemplateStatus



WorkspaceTemplateStatus defines the observed state of WorkspaceTemplate
Follows Kubernetes API conventions for status reporting

_Appears in:_
- [WorkspaceTemplate](#workspacetemplate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration reflects the generation of the most recently observed WorkspaceTemplate spec.<br />This field is used by controllers to determine if they need to reconcile the template.<br />When metadata.generation != status.observedGeneration, the controller has not yet processed the latest spec. |  | Optional: \{\} <br /> |



