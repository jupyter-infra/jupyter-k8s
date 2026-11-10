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
Consistency (min <= max) is enforced by the WorkspaceTemplate validating webhook

_Appears in:_
- [ResourceBounds](#resourcebounds)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `min` _[Quantity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#quantity-resource-api)_ | Min is the minimum allowed value |  | Required: \{\} <br /> |
| `max` _[Quantity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#quantity-resource-api)_ | Max is the maximum allowed value |  | Required: \{\} <br /> |



## SecurityContextOverridePolicy



SecurityContextOverridePolicy controls whether a workspace may deviate from the template's default
pod or container security context.

_Appears in:_
- [WorkspaceTemplateSpec](#workspacetemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `allow` _boolean_ | Allow controls whether workspaces may set their own security context. When false, a workspace's<br />security context must equal the template's default, which the defaulter copies onto it. | true | Optional: \{\} <br /> |



## SharedMemoryOverridePolicy



SharedMemoryOverridePolicy controls whether a workspace may deviate from the template's
DefaultSharedMemory.

_Appears in:_
- [WorkspaceTemplateSpec](#workspacetemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `allow` _boolean_ | Allow controls whether workspaces may set their own sharedMemory or mount their own volume at<br />/dev/shm. When false, workspaces get DefaultSharedMemory and nothing else, so a template that<br />sets it needs a DefaultSharedMemory. | true | Optional: \{\} <br /> |



## StorageConfig



StorageConfig defines storage settings
NOTE: CEL validation for minSize <= maxSize is not possible due to resource.Quantity type limitations
Consistency (minSize <= maxSize) is enforced by the WorkspaceTemplate validating webhook

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



## TemplatedPodSecurityContext



TemplatedPodSecurityContext mirrors corev1.PodSecurityContext, field for field, for a
WorkspaceTemplate's default. Its identity fields (runAsUser, runAsGroup, fsGroup,
supplementalGroups) also accept a Go template, resolved per workspace at admission:
{{ .User.Uid }} and {{ .User.Gid }} (the owner's PosixIdentity, allocated on first use), and
{{ .Namespace.Gid }} (the workspace namespace's workspace.jupyter.org/posix-gid label). A template
using a Go template in any identity field must also set runAsNonRoot: true and
supplementalGroupsPolicy: Strict.

Field docstrings are upstream's, verbatim; identity fields add one paragraph. Keep in sync with
corev1.PodSecurityContext: TestTemplatedSecurityContextsMirrorCore fails on a missing or retyped
field, TestTemplatedSecurityContextsMirrorCoreDocs on a docstring that differs from upstream's.

_Appears in:_
- [WorkspaceTemplateSpec](#workspacetemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `seLinuxOptions` _[SELinuxOptions](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#selinuxoptions-v1-core)_ | The SELinux context to be applied to all containers.<br />If unspecified, the container runtime will allocate a random SELinux context for each<br />container.  May also be set in SecurityContext.  If set in<br />both SecurityContext and PodSecurityContext, the value specified in SecurityContext<br />takes precedence for that container.<br />Note that this field cannot be set when spec.os.name is windows. |  | Optional: \{\} <br /> |
| `windowsOptions` _[WindowsSecurityContextOptions](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#windowssecuritycontextoptions-v1-core)_ | The Windows specific settings applied to all containers.<br />If unspecified, the options within a container's SecurityContext will be used.<br />If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence.<br />Note that this field cannot be set when spec.os.name is linux. |  | Optional: \{\} <br /> |
| `runAsUser` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#intorstring-intstr-util)_ | The UID to run the entrypoint of the container process.<br />Defaults to user specified in image metadata if unspecified.<br />May also be set in SecurityContext.  If set in both SecurityContext and<br />PodSecurityContext, the value specified in SecurityContext takes precedence<br />for that container.<br />Note that this field cannot be set when spec.os.name is windows.<br />An integer, or a Go template resolved for each workspace at admission: \{\{ .User.Uid \}\} (the<br />owner's uid, allocated on first use) or \{\{ .Namespace.Gid \}\} (the workspace namespace's<br />workspace.jupyter.org/posix-gid label). |  | Optional: \{\} <br /> |
| `runAsGroup` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#intorstring-intstr-util)_ | The GID to run the entrypoint of the container process.<br />Uses runtime default if unset.<br />May also be set in SecurityContext.  If set in both SecurityContext and<br />PodSecurityContext, the value specified in SecurityContext takes precedence<br />for that container.<br />Note that this field cannot be set when spec.os.name is windows.<br />An integer, or a Go template resolved for each workspace at admission: \{\{ .User.Gid \}\} (the<br />owner's primary gid, equal to their uid) or \{\{ .Namespace.Gid \}\}. |  | Optional: \{\} <br /> |
| `runAsNonRoot` _boolean_ | Indicates that the container must run as a non-root user.<br />If true, the Kubelet will validate the image at runtime to ensure that it<br />does not run as UID 0 (root) and fail to start the container if it does.<br />If unset or false, no such validation will be performed.<br />May also be set in SecurityContext.  If set in both SecurityContext and<br />PodSecurityContext, the value specified in SecurityContext takes precedence. |  | Optional: \{\} <br /> |
| `supplementalGroups` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#intorstring-intstr-util) array_ | A list of groups applied to the first process run in each container, in<br />addition to the container's primary GID and fsGroup (if specified).  If<br />the SupplementalGroupsPolicy feature is enabled, the<br />supplementalGroupsPolicy field determines whether these are in addition<br />to or instead of any group memberships defined in the container image.<br />If unspecified, no additional groups are added, though group memberships<br />defined in the container image may still be used, depending on the<br />supplementalGroupsPolicy field.<br />Note that this field cannot be set when spec.os.name is windows.<br />Each entry is an integer, or a Go template resolved for each workspace at admission:<br />\{\{ .User.Gid \}\} or \{\{ .Namespace.Gid \}\}, e.g. ["\{\{ .Namespace.Gid \}\}", 2147000123]. |  | MaxItems: 16 <br />Optional: \{\} <br /> |
| `supplementalGroupsPolicy` _[SupplementalGroupsPolicy](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#supplementalgroupspolicy-v1-core)_ | Defines how supplemental groups of the first container processes are calculated.<br />Valid values are "Merge" and "Strict". If not specified, "Merge" is used.<br />(Alpha) Using the field requires the SupplementalGroupsPolicy feature gate to be enabled<br />and the container runtime must implement support for this feature.<br />Note that this field cannot be set when spec.os.name is windows. |  | Optional: \{\} <br /> |
| `fsGroup` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#intorstring-intstr-util)_ | A special supplemental group that applies to all containers in a pod.<br />Some volume types allow the Kubelet to change the ownership of that volume<br />to be owned by the pod:<br />1. The owning GID will be the FSGroup<br />2. The setgid bit is set (new files created in the volume will be owned by FSGroup)<br />3. The permission bits are OR'd with rw-rw----<br />If unset, the Kubelet will not modify the ownership and permissions of any volume.<br />Note that this field cannot be set when spec.os.name is windows.<br />An integer, or a Go template resolved for each workspace at admission: \{\{ .User.Gid \}\} (the<br />owner's primary gid, equal to their uid) or \{\{ .Namespace.Gid \}\}. |  | Optional: \{\} <br /> |
| `sysctls` _[Sysctl](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#sysctl-v1-core) array_ | Sysctls hold a list of namespaced sysctls used for the pod. Pods with unsupported<br />sysctls (by the container runtime) might fail to launch.<br />Note that this field cannot be set when spec.os.name is windows. |  | Optional: \{\} <br /> |
| `fsGroupChangePolicy` _[PodFSGroupChangePolicy](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#podfsgroupchangepolicy-v1-core)_ | fsGroupChangePolicy defines behavior of changing ownership and permission of the volume<br />before being exposed inside Pod. This field will only apply to<br />volume types which support fsGroup based ownership(and permissions).<br />It will have no effect on ephemeral volume types such as: secret, configmaps<br />and emptydir.<br />Valid values are "OnRootMismatch" and "Always". If not specified, "Always" is used.<br />Note that this field cannot be set when spec.os.name is windows. |  | Optional: \{\} <br /> |
| `seccompProfile` _[SeccompProfile](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#seccompprofile-v1-core)_ | The seccomp options to use by the containers in this pod.<br />Note that this field cannot be set when spec.os.name is windows. |  | Optional: \{\} <br /> |
| `appArmorProfile` _[AppArmorProfile](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#apparmorprofile-v1-core)_ | appArmorProfile is the AppArmor options to use by the containers in this pod.<br />Note that this field cannot be set when spec.os.name is windows. |  | Optional: \{\} <br /> |
| `seLinuxChangePolicy` _[PodSELinuxChangePolicy](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#podselinuxchangepolicy-v1-core)_ | seLinuxChangePolicy defines how the container's SELinux label is applied to all volumes used by the Pod.<br />It has no effect on nodes that do not support SELinux or to volumes does not support SELinux.<br />Valid values are "MountOption" and "Recursive".<br />"Recursive" means relabeling of all files on all Pod volumes by the container runtime.<br />This may be slow for large volumes, but allows mixing privileged and unprivileged Pods sharing the same volume on the same node.<br />"MountOption" mounts all eligible Pod volumes with `-o context` mount option.<br />This requires all Pods that share the same volume to use the same SELinux label.<br />It is not possible to share the same volume among privileged and unprivileged Pods.<br />Eligible volumes are in-tree FibreChannel and iSCSI volumes, and all CSI volumes<br />whose CSI driver announces SELinux support by setting spec.seLinuxMount: true in their<br />CSIDriver instance. Other volumes are always re-labelled recursively.<br />"MountOption" value is allowed only when SELinuxMount feature gate is enabled.<br />If not specified and SELinuxMount feature gate is enabled, "MountOption" is used.<br />If not specified and SELinuxMount feature gate is disabled, "MountOption" is used for ReadWriteOncePod volumes<br />and "Recursive" for all other volumes.<br />This field affects only Pods that have SELinux label set, either in PodSecurityContext or in SecurityContext of all containers.<br />All Pods that use the same volume should use the same seLinuxChangePolicy, otherwise some pods can get stuck in ContainerCreating state.<br />Note that this field cannot be set when spec.os.name is windows. |  | Optional: \{\} <br /> |



## TemplatedSecurityContext



TemplatedSecurityContext mirrors corev1.SecurityContext, field for field, for a WorkspaceTemplate's
default container security context. Its identity fields (runAsUser, runAsGroup) also accept a Go
template, with the inputs of TemplatedPodSecurityContext.

Field docstrings are upstream's, verbatim; identity fields add one paragraph. Keep in sync with
corev1.SecurityContext as TemplatedPodSecurityContext is.

_Appears in:_
- [WorkspaceTemplateSpec](#workspacetemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `capabilities` _[Capabilities](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#capabilities-v1-core)_ | The capabilities to add/drop when running containers.<br />Defaults to the default set of capabilities granted by the container runtime.<br />Note that this field cannot be set when spec.os.name is windows. |  | Optional: \{\} <br /> |
| `privileged` _boolean_ | Run container in privileged mode.<br />Processes in privileged containers are essentially equivalent to root on the host.<br />Defaults to false.<br />Note that this field cannot be set when spec.os.name is windows. |  | Optional: \{\} <br /> |
| `seLinuxOptions` _[SELinuxOptions](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#selinuxoptions-v1-core)_ | The SELinux context to be applied to the container.<br />If unspecified, the container runtime will allocate a random SELinux context for each<br />container.  May also be set in PodSecurityContext.  If set in both SecurityContext and<br />PodSecurityContext, the value specified in SecurityContext takes precedence.<br />Note that this field cannot be set when spec.os.name is windows. |  | Optional: \{\} <br /> |
| `windowsOptions` _[WindowsSecurityContextOptions](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#windowssecuritycontextoptions-v1-core)_ | The Windows specific settings applied to all containers.<br />If unspecified, the options from the PodSecurityContext will be used.<br />If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence.<br />Note that this field cannot be set when spec.os.name is linux. |  | Optional: \{\} <br /> |
| `runAsUser` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#intorstring-intstr-util)_ | The UID to run the entrypoint of the container process.<br />Defaults to user specified in image metadata if unspecified.<br />May also be set in PodSecurityContext.  If set in both SecurityContext and<br />PodSecurityContext, the value specified in SecurityContext takes precedence.<br />Note that this field cannot be set when spec.os.name is windows.<br />An integer, or a Go template resolved for each workspace at admission: \{\{ .User.Uid \}\} (the<br />owner's uid, allocated on first use) or \{\{ .Namespace.Gid \}\} (the workspace namespace's<br />workspace.jupyter.org/posix-gid label). |  | Optional: \{\} <br /> |
| `runAsGroup` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#intorstring-intstr-util)_ | The GID to run the entrypoint of the container process.<br />Uses runtime default if unset.<br />May also be set in PodSecurityContext.  If set in both SecurityContext and<br />PodSecurityContext, the value specified in SecurityContext takes precedence.<br />Note that this field cannot be set when spec.os.name is windows.<br />An integer, or a Go template resolved for each workspace at admission: \{\{ .User.Gid \}\} (the<br />owner's primary gid, equal to their uid) or \{\{ .Namespace.Gid \}\}. |  | Optional: \{\} <br /> |
| `runAsNonRoot` _boolean_ | Indicates that the container must run as a non-root user.<br />If true, the Kubelet will validate the image at runtime to ensure that it<br />does not run as UID 0 (root) and fail to start the container if it does.<br />If unset or false, no such validation will be performed.<br />May also be set in PodSecurityContext.  If set in both SecurityContext and<br />PodSecurityContext, the value specified in SecurityContext takes precedence. |  | Optional: \{\} <br /> |
| `readOnlyRootFilesystem` _boolean_ | Whether this container has a read-only root filesystem.<br />Default is false.<br />Note that this field cannot be set when spec.os.name is windows. |  | Optional: \{\} <br /> |
| `allowPrivilegeEscalation` _boolean_ | AllowPrivilegeEscalation controls whether a process can gain more<br />privileges than its parent process. This bool directly controls if<br />the no_new_privs flag will be set on the container process.<br />AllowPrivilegeEscalation is true always when the container is:<br />1) run as Privileged<br />2) has CAP_SYS_ADMIN<br />Note that this field cannot be set when spec.os.name is windows. |  | Optional: \{\} <br /> |
| `procMount` _[ProcMountType](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#procmounttype-v1-core)_ | procMount denotes the type of proc mount to use for the containers.<br />The default value is Default which uses the container runtime defaults for<br />readonly paths and masked paths.<br />Note that this field cannot be set when spec.os.name is windows. |  | Optional: \{\} <br /> |
| `seccompProfile` _[SeccompProfile](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#seccompprofile-v1-core)_ | The seccomp options to use by this container. If seccomp options are<br />provided at both the pod & container level, the container options<br />override the pod options.<br />Note that this field cannot be set when spec.os.name is windows. |  | Optional: \{\} <br /> |
| `appArmorProfile` _[AppArmorProfile](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#apparmorprofile-v1-core)_ | appArmorProfile is the AppArmor options to use by this container. If set, this profile<br />overrides the pod's appArmorProfile.<br />Note that this field cannot be set when spec.os.name is windows. |  | Optional: \{\} <br /> |



## VolumeOverridePolicy



VolumeOverridePolicy controls whether a workspace may deviate from the template's DefaultVolumes.

_Appears in:_
- [WorkspaceTemplateSpec](#workspacetemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `allow` _boolean_ | Allow controls whether workspaces may drop or change the template's DefaultVolumes. When false, a<br />workspace must list every DefaultVolumes entry unchanged; a workspace that declares no volumes<br />receives them from the defaulter, and one that declares its own must include them alongside.<br />Set AllowSecondaryStorages to false as well to restrict workspaces to DefaultVolumes exactly as<br />defined; otherwise a workspace can add its own volume on a default volume's claim. | true | Optional: \{\} <br /> |



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
| `defaultVolumes` _[VolumeSpec](#volumespec) array_ | DefaultVolumes specifies default additional volumes for workspaces using this template.<br />Each volume references either a pre-existing PVC in the workspace namespace or an emptyDir source.<br />The admission webhook copies them onto a workspace that declares no volumes of its own;<br />VolumeOverrides controls whether a workspace may drop or change them. |  | MaxItems: 10 <br />Optional: \{\} <br /> |
| `volumeOverrides` _[VolumeOverridePolicy](#volumeoverridepolicy)_ | VolumeOverrides controls whether a workspace may drop or change DefaultVolumes. When locked, the<br />volumes of any workspace referencing the template must include the template's defaults, exactly<br />as defined. |  | Optional: \{\} <br /> |
| `defaultSharedMemory` _[SharedMemorySpec](#sharedmemoryspec)_ | DefaultSharedMemory is the /dev/shm setting for workspaces using this template; set it, \{\} is<br />enough, to give every workspace of the template the enlarged /dev/shm. Copied in whole onto a<br />workspace that sets no sharedMemory. |  | Optional: \{\} <br /> |
| `sharedMemoryOverrides` _[SharedMemoryOverridePolicy](#sharedmemoryoverridepolicy)_ | SharedMemoryOverrides controls whether a workspace may deviate from DefaultSharedMemory, including<br />with a volume it mounts at /dev/shm itself. |  | Optional: \{\} <br /> |
| `defaultNodeSelector` _object (keys:string, values:string)_ | DefaultNodeSelector specifies default node selection constraints |  | Optional: \{\} <br /> |
| `defaultAffinity` _[Affinity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#affinity-v1-core)_ | DefaultAffinity specifies default node affinity and anti-affinity rules |  | Optional: \{\} <br /> |
| `defaultTolerations` _[Toleration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#toleration-v1-core) array_ | DefaultTolerations specifies default tolerations for scheduling on nodes with taints |  | Optional: \{\} <br /> |
| `defaultOwnershipType` _string_ | DefaultOwnershipType specifies default ownershipType for workspaces using this template<br />OwnershipType controls which users may edit/delete the workspace | Public | Enum: [Public OwnerOnly] <br />Optional: \{\} <br /> |
| `allowCustomOwnershipType` _boolean_ | AllowCustomOwnershipType controls whether workspaces using this template may set an ownershipType<br />other than DefaultOwnershipType. When false, a workspace's ownershipType must equal<br />DefaultOwnershipType. | true | Optional: \{\} <br /> |
| `baseLabels` _[TemplateLabel](#templatelabel) array_ | BaseLabels specifies labels to add to workspaces using this template<br />Labels are added during defaulting if not already present on the workspace |  | MaxItems: 50 <br />Optional: \{\} <br /> |
| `labelRequirements` _[LabelRequirement](#labelrequirement) array_ | LabelRequirements specifies validation rules for workspace labels |  | MaxItems: 50 <br />Optional: \{\} <br /> |
| `defaultIdleShutdown` _[IdleShutdownSpec](#idleshutdownspec)_ | DefaultIdleShutdown provides default idle shutdown configuration<br />Includes timeout, detection endpoint, and enable/disable |  | Optional: \{\} <br /> |
| `idleShutdownOverrides` _[IdleShutdownOverridePolicy](#idleshutdownoverridepolicy)_ | IdleShutdownOverrides controls override behavior and bounds |  | Optional: \{\} <br /> |
| `defaultAccessType` _string_ | DefaultAccessType specifies the default accessType for workspaces using this template<br />AccessType controls which users may create connections to the workspace. | Public | Enum: [Public OwnerOnly] <br />Optional: \{\} <br /> |
| `allowCustomAccessType` _boolean_ | AllowCustomAccessType controls whether workspaces using this template may set an accessType other<br />than DefaultAccessType. When false, a workspace's accessType must equal DefaultAccessType. | true | Optional: \{\} <br /> |
| `defaultAccessStrategy` _[AccessStrategyRef](#accessstrategyref)_ | DefaultAccessStrategy specifies the default access strategy for workspaces using this template |  | Optional: \{\} <br /> |
| `defaultLifecycle` _[Lifecycle](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#lifecycle-v1-core)_ | DefaultLifecycle specifies default lifecycle hooks for workspaces using this template |  | Optional: \{\} <br /> |
| `defaultReadinessProbe` _[Probe](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#probe-v1-core)_ | DefaultReadinessProbe specifies the default readiness probe for the main workspace<br />container for workspaces using this template.<br />Applied only if the workspace does not specify its own readiness probe. |  | Optional: \{\} <br /> |
| `defaultPodSecurityContext` _[TemplatedPodSecurityContext](#templatedpodsecuritycontext)_ | DefaultPodSecurityContext specifies default pod-level security context. Its identity fields<br />(runAsUser, runAsGroup, fsGroup, supplementalGroups) accept an integer or a Go template resolved<br />per workspace at admission, e.g. "\{\{ .User.Uid \}\}"; see TemplatedPodSecurityContext. |  | Optional: \{\} <br /> |
| `defaultContainerSecurityContext` _[TemplatedSecurityContext](#templatedsecuritycontext)_ | DefaultContainerSecurityContext specifies default container-level security context for the main<br />workspace container. Its identity fields (runAsUser, runAsGroup) accept an integer or a Go<br />template; see TemplatedSecurityContext. |  | Optional: \{\} <br /> |
| `podSecurityContextOverrides` _[SecurityContextOverridePolicy](#securitycontextoverridepolicy)_ | PodSecurityContextOverrides controls whether a workspace may set its own podSecurityContext.<br />When locked, workspaces get DefaultPodSecurityContext, or no pod security context if unset. |  | Optional: \{\} <br /> |
| `containerSecurityContextOverrides` _[SecurityContextOverridePolicy](#securitycontextoverridepolicy)_ | ContainerSecurityContextOverrides controls whether a workspace may set its own<br />containerSecurityContext. When locked, workspaces get DefaultContainerSecurityContext, or no<br />container security context if unset. |  | Optional: \{\} <br /> |
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



