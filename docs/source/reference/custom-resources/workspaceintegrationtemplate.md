# WorkspaceIntegrationTemplate

## WorkspaceIntegrationTemplate



WorkspaceIntegrationTemplate is the Schema for the workspaceintegrationtemplates API.
It defines a declarative, template-driven integration for adding runtime capabilities
(sidecars, volumes, env vars) to workspace pods with dynamic resource lookup and
template expression resolution. A Workspace may attach several of these via
spec.integrationRefs.

| Field | Value or Description |
| --- | --- |
| `apiVersion` _string_ | `workspace.jupyter.org/v1alpha1` |
| `kind` _string_ | `WorkspaceIntegrationTemplate` |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[WorkspaceIntegrationTemplateSpec](#workspaceintegrationtemplatespec)_ | Spec defines the desired state of WorkspaceIntegrationTemplate |



## IntegrationStatusProbe



IntegrationStatusProbe defines how the operator checks an integration and records the
verdict in workspace.status.integrationStatuses[]. It is report-only (not gating), which is why
it is a statusProbe rather than a readinessProbe.

The handler fields mirror the corev1.Probe handler union: exactly one transport is set.
We deliberately do NOT embed corev1.Probe — it carries kubelet-only semantics
(successThreshold, terminationGracePeriodSeconds) that don't apply to an operator-run,
non-gating probe. This follows the AccessStartupProbe precedent, which hand-rolls its
own wrapper around an optional handler for the same reason.

Only Exec is implemented today (the operator execs the command in the workspace
container, mirroring the idle detector). HTTPGet/TCPSocket/GRPC are reserved as additive
optional siblings — adding one later is non-breaking.

_Appears in:_
- [WorkspaceIntegrationTemplateSpec](#workspaceintegrationtemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `exec` _[ExecAction](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#execaction-v1-core)_ | Exec runs a command inside the workspace container; exit code 0 means ready. The<br />command always runs in the workspace container (the operator fixes this; it is not<br />author-selectable) so it sees the pod's real network and auth context, catching<br />data-plane failures a control-plane status check would miss (e.g. a Ray GCS reconnect<br />loop while .status still reads ready). This is the only transport implemented today. |  | Optional: \{\} <br /> |
| `timeoutSeconds` _integer_ | Number of seconds after which a single probe attempt times out. Default: 5. Minimum: 1. |  | Optional: \{\} <br /> |



## IntegrationTemplateParameter



IntegrationTemplateParameter declares a single parameter a WorkspaceIntegrationTemplate consumes.
It is the admin's declaration of the parameter contract (name only); the user supplies the value on
the workspace's integrationTemplateRefs[].parameters. All declared parameters are required.

_Appears in:_
- [WorkspaceIntegrationTemplateSpec](#workspaceintegrationtemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the parameter, referenced in template expressions as \{\{ .Parameters.<Name> \}\} and supplied<br />by the workspace as integrationTemplateRefs[].parameters[].name. |  | MinLength: 1 <br /> |



## ResourceRef



ResourceRef identifies a Kubernetes resource to fetch at resolution time and gives it a stable
handle (Name) for use in template expressions: {{ resource "<name>" "<jsonpath>" }}. The shape
follows the KRO external-reference precedent
(https://kro.run/docs/concepts/rgd/resource-definitions/external-references): a handle Name plus
apiVersion/kind identify the KIND, and the nested Metadata identifies the specific object.

_Appears in:_
- [WorkspaceIntegrationTemplateSpec](#workspaceintegrationtemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the handle used to reference this resource in template expressions:<br />\{\{ resource "<name>" "<jsonpath>" \}\}. Must be unique within resourceRefs. |  | MaxLength: 63 <br />Pattern: `^[a-z][a-zA-Z0-9-]*$` <br /> |
| `apiVersion` _string_ | APIVersion of the target resource (e.g., "ray.io/v1") |  |  |
| `kind` _string_ | Kind of the target resource (e.g., "RayCluster") |  |  |
| `metadata` _[ResourceRefMetadata](#resourcerefmetadata)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |



## ResourceRefMetadata



ResourceRefMetadata is the templated object identity (name + optional namespace) of a ResourceRef.

_Appears in:_
- [ResourceRef](#resourceref)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the target object; supports template expressions. |  |  |
| `namespace` _string_ | Namespace of the target object; supports template expressions. Defaults to the workspace's<br />namespace if omitted. |  | Optional: \{\} <br /> |



## WorkspaceIntegrationTemplateSpec



WorkspaceIntegrationTemplateSpec defines the desired state of WorkspaceIntegrationTemplate

_Appears in:_
- [WorkspaceIntegrationTemplate](#workspaceintegrationtemplate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `displayName` _string_ | DisplayName is a human-readable name for this integration template |  | MaxLength: 253 <br /> |
| `parameters` _[IntegrationTemplateParameter](#integrationtemplateparameter) array_ | Parameters declares the parameters this template consumes via \{\{ .Parameters.<name> \}\}. It is the<br />single source of truth for the template's parameter contract: a template expression may only<br />reference a declared parameter (an undeclared \{\{ .Parameters.X \}\} is rejected at the template<br />author's write), and a referencing Workspace must supply every declared parameter. All declared<br />parameters are required -- there are no optional parameters or defaults, so an unsupplied one<br />would resolve to empty; the Workspace webhook rejects that at the user's write instead. |  | MaxItems: 10 <br />Optional: \{\} <br /> |
| `resourceRefs` _[ResourceRef](#resourceref) array_ | ResourceRefs defines the resources to fetch at resolution time. Each entry is<br />addressed in template expressions by its handle name: \{\{ resource "<name>" "<jsonpath>" \}\}.<br />A template exists to resolve values from at least one referenced resource, so at<br />least one ref is required; capped at one for now (see integrationTemplateRefs). |  | MaxItems: 1 <br />MinItems: 1 <br /> |
| `shareProcessNamespace` _boolean_ | ShareProcessNamespace enables a shared PID namespace across all containers in the<br />workspace pod, so the workspace container can see and signal processes running in<br />injected sidecars (e.g. the Ray sidecar). Mirrors corev1.PodSpec.ShareProcessNamespace.<br />Admin-controlled (this field only exists on the integration template, which admins<br />install). Containers sharing a PID namespace can read each other's /proc (filesystem<br />and environment); injected containers should run as non-root. |  | Optional: \{\} <br /> |
| `deploymentModifications` _[DeploymentModifications](#deploymentmodifications)_ | DeploymentModifications defines modifications to apply to workspace deployments<br />with template expressions in string fields |  | Optional: \{\} <br /> |
| `statusProbe` _[IntegrationStatusProbe](#integrationstatusprobe)_ | StatusProbe is an optional probe the operator runs periodically<br />to verify the integration is reachable from the workspace pod's actual runtime<br />context. There is one probe per integration (not per resourceRef): it yields a<br />single verdict surfaced in workspace.status.integrationStatuses[].<br />It is named statusProbe (not readinessProbe) because it is report-only: it writes<br />the verdict into status and never gates pod endpoint registration or restarts the<br />pod. Modeled on accessStrategy.spec.accessStartupProbe, but re-evaluated on the<br />operator poll cadence rather than as a one-shot gate. |  | Optional: \{\} <br /> |


