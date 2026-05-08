# Operator Chart

Configuration reference for the **Jupyter K8s** operator Helm chart (`oci://ghcr.io/jupyter-infra/charts/jupyter-k8s`).

Chart version: 0.1.0 | App version: 0.1.0

## Values

```{list-table}
:header-rows: 1
:widths: 30 12 29 29

* - Key
  - Type
  - Default
  - Description
* - `accessResources.additionalGvk`
  - list
  - `[]`
  - Additional Group-Version-Kind resources to watch for access strategy
* - `accessResources.traefik.enable`
  - bool
  - `false`
  - Enable watching Traefik IngressRoute resources
* - `application.imagesPullPolicy`
  - string
  - `"IfNotPresent"`
  - Image pull policy for workspace pod containers
* - `application.imagesRegistry`
  - string
  - `"docker.io/library"`
  - Image registry prefix for workspace pod containers
* - `certManager.enable`
  - bool
  - `true`
  - Enable cert-manager integration (required for webhooks and metrics TLS)
* - `controller.plugins`
  - list
  - `[]`
  - Plugin sidecars to deploy alongside the controller. Each plugin runs as a sidecar container in the controller pod.
* - `crd.enable`
  - bool
  - `true`
  - Install CRDs with the chart
* - `crd.keep`
  - bool
  - `true`
  - Keep CRDs when uninstalling
* - `extensionApi.enable`
  - bool
  - `true`
  - Enable the Extension API server (serves Connection APIs)
* - `extensionApi.jwtAudience`
  - string
  - `"workspaces-controller"`
  - JWT audience claim
* - `extensionApi.jwtIssuer`
  - string
  - `"workspaces-controller"`
  - JWT issuer claim
* - `extensionApi.jwtSecret.enable`
  - bool
  - `false`
  - Enable K8s-native JWT signing (HMAC keys from a K8s Secret, rotated by CronJob)
* - `extensionApi.jwtSecret.newKeyUseDelay`
  - string
  - `"5s"`
  - Delay before using a newly rotated key (allows propagation)
* - `extensionApi.jwtSecret.rotationInterval`
  - string
  - `"15m"`
  - How often the CronJob rotates the key
* - `extensionApi.jwtSecret.rotator.imageName`
  - string
  - `"jupyter-k8s-rotator"`
  - Rotator image name
* - `extensionApi.jwtSecret.rotator.imagePullPolicy`
  - string
  - `"IfNotPresent"`
  - Rotator image pull policy
* - `extensionApi.jwtSecret.rotator.imageTag`
  - string
  - `""`
  - Rotator image tag (defaults to chart appVersion)
* - `extensionApi.jwtSecret.rotator.repository`
  - string
  - `"ghcr.io/jupyter-infra"`
  - Rotator image repository
* - `extensionApi.jwtSecret.rotator.resources`
  - object
  - `{"limits":{"cpu":"100m","memory":"128Mi"},"requests":{"cpu":"50m","memory":"64Mi"}}`
  - Rotator resource limits and requests
* - `extensionApi.jwtSecret.secretName`
  - string
  - `"jupyter-k8s-extensionapi-secrets"`
  - Name of the Kubernetes Secret holding signing keys
* - `extensionApi.jwtSecret.tokenTTL`
  - string
  - `"5m"`
  - Token time-to-live
* - `fullnameOverride`
  - string
  - `"jupyter-k8s"`
  - String to fully override chart.fullname template
* - `manager.affinity`
  - object
  - `{"nodeAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":{"nodeSelectorTerms":[{"matchExpressions":[{"key":"kubernetes.io/arch","operator":"In","values":["amd64","arm64","ppc64le","s390x"]},{"key":"kubernetes.io/os","operator":"In","values":["linux"]}]}]}}}`
  - Node affinity rules for the manager pod
* - `manager.args`
  - list
  - `["--leader-elect"]`
  - Controller manager arguments
* - `manager.env`
  - list
  - `[{"name":"CONTROLLER_POD_NAMESPACE","valueFrom":{"fieldRef":{"fieldPath":"metadata.namespace"}}},{"name":"CONTROLLER_POD_SERVICE_ACCOUNT","valueFrom":{"fieldRef":{"fieldPath":"spec.serviceAccountName"}}},{"name":"CLUSTER_ADMIN_GROUP","value":"cluster-workspace-admin"}]`
  - Environment variables for the controller container
* - `manager.envOverrides`
  - object
  - `{}`
  - Environment variable overrides (same name in env above: this value takes precedence)
* - `manager.image.pullPolicy`
  - string
  - `"IfNotPresent"`
  - Image pull policy
* - `manager.image.repository`
  - string
  - `"ghcr.io/jupyter-infra/jupyter-k8s-controller"`
  - Controller image repository
* - `manager.image.tag`
  - string
  - `""`
  - Controller image tag (defaults to chart appVersion)
* - `manager.imagePullSecrets`
  - list
  - `[]`
  - Image pull secrets for private registries
* - `manager.nodeSelector`
  - object
  - `{}`
  - Node selector for the manager pod
* - `manager.pod`
  - object
  - `{"annotations":{},"labels":{}}`
  - Pod metadata
* - `manager.pod.annotations`
  - object
  - `{}`
  - Additional annotations for the manager pod
* - `manager.pod.labels`
  - object
  - `{}`
  - Additional labels for the manager pod
* - `manager.podDisruptionBudget`
  - object
  - `{"enabled":false,"maxUnavailable":null,"minAvailable":null}`
  - PodDisruptionBudget configuration
* - `manager.podDisruptionBudget.enabled`
  - bool
  - `false`
  - Enable PodDisruptionBudget
* - `manager.podDisruptionBudget.maxUnavailable`
  - string
  - `nil`
  - Maximum unavailable pods
* - `manager.podDisruptionBudget.minAvailable`
  - string
  - `nil`
  - Minimum available pods
* - `manager.podSecurityContext`
  - object
  - `{"runAsNonRoot":true,"seccompProfile":{"type":"RuntimeDefault"}}`
  - Pod-level security settings
* - `manager.replicas`
  - int
  - `1`
  - Number of controller manager replicas
* - `manager.resources`
  - object
  - `{"limits":{"cpu":"1000m","memory":"256Mi"},"requests":{"cpu":"20m","memory":"128Mi"}}`
  - Resource limits and requests for the controller container
* - `manager.securityContext`
  - object
  - `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"readOnlyRootFilesystem":true}`
  - Container-level security settings
* - `manager.tolerations`
  - list
  - `[]`
  - Tolerations for the manager pod
* - `manager.topologySpreadConstraints`
  - list
  - `[]`
  - Topology spread constraints for the manager pod
* - `metrics.enable`
  - bool
  - `true`
  - Enable metrics endpoint
* - `metrics.port`
  - int
  - `8443`
  - Metrics server port
* - `prometheus.enable`
  - bool
  - `false`
  - Enable Prometheus ServiceMonitor
* - `rbacHelpers.enable`
  - bool
  - `true`
  - Install convenience admin/editor/viewer roles for CRDs
* - `webhook.enable`
  - bool
  - `true`
  - Enable admission webhooks
* - `webhook.port`
  - int
  - `9443`
  - Webhook server port
* - `workspacePodWatching.enable`
  - bool
  - `false`
  - Enable workspace pod event watching for lifecycle management (required for remote access plugins)
* - `workspaceTemplates.defaultNamespace`
  - string
  - `"jupyter-k8s-shared"`
  - Namespace where shared workspace templates are stored
```
