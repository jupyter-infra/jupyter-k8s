#!/bin/bash
set -e

# Script to apply custom patches to helm chart files after generation
# Run this after 'kubebuilder edit --plugins=helm/v2-alpha --force'
#
# The v2-alpha plugin generates a solid base chart from kustomize output.
# This script adds features the plugin doesn't know about:
#   - Conditional flags in manager args (extensionApi JWT, traefik, pod watching, plugins)
#   - Plugin sidecar containers
#   - PodDisruptionBudget
#   - Pod annotations
#   - Templated JWT rotator (image, env vars)
#   - Conditional resource wrapping (extensionApi.enable, jwtSecret.enable)
#   - Extension API auth RoleBinding in kube-system
#   - Additional values sections

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHART_DIR="$(cd "${SCRIPT_DIR}/../dist/chart" && pwd)"
PATCHES_DIR="${SCRIPT_DIR}/helm-patches"

if [ ! -d "${CHART_DIR}" ]; then
    echo "Error: Chart directory not found at ${CHART_DIR}"
    echo "Run 'kubebuilder edit --plugins=helm/v2-alpha' first"
    exit 1
fi

echo "Applying custom patches to Helm chart files..."

# --- values.yaml: add custom sections ---
echo "Appending custom values sections..."
cat "${PATCHES_DIR}/values.yaml.patch" >> "${CHART_DIR}/values.yaml"

# --- values.yaml: add PDB config under manager ---
if ! grep -q "podDisruptionBudget:" "${CHART_DIR}/values.yaml"; then
    echo "Adding PDB and pod metadata config to manager section..."
    # Insert after tolerations: []
    sed -i '/^  tolerations: \[\]/a\\n  ## PodDisruptionBudget configuration\n  ##\n  podDisruptionBudget:\n    enabled: false\n    minAvailable: null\n    maxUnavailable: null\n\n  ## Pod metadata\n  ##\n  pod:\n    labels: {}\n    annotations: {}' "${CHART_DIR}/values.yaml"
fi

# --- values.yaml: add topologySpreadConstraints if missing ---
if ! grep -q "topologySpreadConstraints:" "${CHART_DIR}/values.yaml"; then
    sed -i '/^  tolerations: \[\]/a\\n  ## Topology spread constraints\n  ##\n  topologySpreadConstraints: []' "${CHART_DIR}/values.yaml"
fi

# --- manager.yaml: add conditional args ---
MANAGER_YAML="${CHART_DIR}/templates/manager/manager.yaml"
echo "Patching manager.yaml args..."

# The v2-alpha chart renders manager.args as a simple range loop.
# We add value-driven args after the range block so they're controlled
# by dedicated values sections rather than baked into the args list.
if ! grep -q "extensionApi" "${MANAGER_YAML}"; then
    sed -i '/{{- range .Values.manager.args }}/,/{{- end }}/ {
        /{{- end }}/a\        - "--application-images-pull-policy={{ .Values.application.imagesPullPolicy }}"\n        - "--application-images-registry={{ .Values.application.imagesRegistry }}"\n        - "--default-template-namespace={{ .Values.workspaceTemplates.defaultNamespace }}"\n        {{- if .Values.accessResources.traefik.enable }}\n        - --watch-traefik\n        {{- end }}\n        {{- if .Values.extensionApi.enable }}\n        - --enable-extension-api\n        {{- if .Values.extensionApi.jwtIssuer }}\n        - --jwt-issuer={{ .Values.extensionApi.jwtIssuer }}\n        {{- end }}\n        {{- if .Values.extensionApi.jwtAudience }}\n        - --jwt-audience={{ .Values.extensionApi.jwtAudience }}\n        {{- end }}\n        {{- end }}\n        {{- if and .Values.extensionApi.enable .Values.extensionApi.jwtSecret.enable }}\n        - --jwt-secret-name={{ .Values.extensionApi.jwtSecret.secretName }}\n        {{- if .Values.extensionApi.jwtSecret.tokenTTL }}\n        - --jwt-ttl={{ .Values.extensionApi.jwtSecret.tokenTTL }}\n        {{- end }}\n        {{- if .Values.extensionApi.jwtSecret.newKeyUseDelay }}\n        - --new-key-use-delay={{ .Values.extensionApi.jwtSecret.newKeyUseDelay }}\n        {{- end }}\n        {{- end }}\n        {{- if .Values.workspacePodWatching.enable }}\n        - --enable-workspace-pod-watching\n        {{- end }}\n        {{- if .Values.controller.plugins }}\n        - "--plugin-endpoints={{ range $i, $p := .Values.controller.plugins }}{{ if $i }},{{ end }}{{ $p.name }}=http://localhost:{{ $p.port }}{{ end }}"\n        {{- end }}
    }' "${MANAGER_YAML}"
fi

# --- manager.yaml: add pod annotations support ---
if ! grep -q "manager.pod.annotations" "${MANAGER_YAML}"; then
    echo "Adding pod annotations support..."
    sed -i '/kubectl.kubernetes.io\/default-container: manager$/a\        {{- with .Values.manager.pod.annotations }}\n        {{- toYaml . | nindent 8 }}\n        {{- end }}' "${MANAGER_YAML}"
fi

# --- manager.yaml: add pod labels support ---
if ! grep -q "manager.pod.labels" "${MANAGER_YAML}"; then
    echo "Adding pod labels support..."
    # The label 'workspace.jupyter.org/component: controller' appears twice:
    # once in Deployment metadata, once in pod template metadata.
    # We only want custom labels in the pod template (second occurrence).
    awk '/workspace\.jupyter\.org\/component: controller/ { count++; print; if (count == 2) { print "        {{- with .Values.manager.pod.labels }}"; print "        {{- toYaml . | nindent 8 }}"; print "        {{- end }}" }; next } { print }' "${MANAGER_YAML}" > "${MANAGER_YAML}.tmp" && mv "${MANAGER_YAML}.tmp" "${MANAGER_YAML}"
fi

# --- manager.yaml: add topologySpreadConstraints support ---
if grep -q "topologySpreadConstraints: \[\]" "${MANAGER_YAML}"; then
    echo "Templating topologySpreadConstraints..."
    sed -i 's/      topologySpreadConstraints: \[\]/      {{- with .Values.manager.topologySpreadConstraints }}\n      topologySpreadConstraints: {{ toYaml . | nindent 10 }}\n      {{- end }}/' "${MANAGER_YAML}"
fi

# --- manager.yaml: add plugin sidecar containers ---
if ! grep -q "plugin-{{ .name }}" "${MANAGER_YAML}"; then
    echo "Adding plugin sidecar container support..."
    # Insert before nodeSelector (pod-level field after the containers list).
    # Plugins are additional containers at the same level as the manager container.
    sed -i '/{{- with .Values.manager.nodeSelector }}/i\      {{- range .Values.controller.plugins }}\n      - name: plugin-{{ .name }}\n        image: "{{ .image.repository }}:{{ .image.tag }}"\n        {{- if .imagePullPolicy }}\n        imagePullPolicy: {{ .imagePullPolicy }}\n        {{- end }}\n        ports:\n          - containerPort: {{ .port }}\n            protocol: TCP\n        {{- if .healthcheckCommand }}\n        livenessProbe:\n          exec:\n            command: {{ .healthcheckCommand | toJson }}\n          initialDelaySeconds: 5\n          periodSeconds: 10\n        readinessProbe:\n          exec:\n            command: {{ .healthcheckCommand | toJson }}\n          initialDelaySeconds: 2\n          periodSeconds: 5\n        {{- end }}\n        {{- if .env }}\n        env:\n          {{- range $key, $value := .env }}\n          - name: {{ $key }}\n            value: "{{ $value }}"\n          {{- end }}\n        {{- end }}\n        {{- if .resources }}\n        resources:\n          {{- toYaml .resources | nindent 10 }}\n        {{- end }}\n      {{- end }}' "${MANAGER_YAML}"
fi

# --- manager.yaml: rewrite volumeMounts and volumes sections ---
# The v2-alpha chart has unconditional extension-server-cert and no metrics-certs.
# Replace both sections with properly conditional versions.
echo "Rewriting volumeMounts and volumes sections..."

# Replace the volumeMounts block (from 'volumeMounts:' to just before the plugin range)
sed -i '/^        volumeMounts:$/,/^      {{- range .Values.controller.plugins }}$/{
    /^      {{- range .Values.controller.plugins }}$/!d
}' "${MANAGER_YAML}"
sed -i '/^      {{- range .Values.controller.plugins }}$/i\        volumeMounts:\n        {{- if .Values.certManager.enable }}\n        - mountPath: /tmp/k8s-webhook-server/serving-certs\n          name: webhook-certs\n          readOnly: true\n        {{- end }}\n        {{- if and .Values.metrics.enable .Values.certManager.enable }}\n        - mountPath: /tmp/k8s-metrics-server/metrics-certs\n          name: metrics-certs\n          readOnly: true\n        {{- end }}\n        {{- if .Values.extensionApi.enable }}\n        - mountPath: /tmp/extension-server/serving-certs\n          name: extension-server-cert\n          readOnly: true\n        {{- end }}' "${MANAGER_YAML}"

# Replace the volumes block (from 'volumes:' to end of file)
sed -i '/^      volumes:$/,$d' "${MANAGER_YAML}"
cat >> "${MANAGER_YAML}" << 'VOLEOF'
      volumes:
      {{- if .Values.certManager.enable }}
      - name: webhook-certs
        secret:
          secretName: webhook-server-cert
      {{- end }}
      {{- if and .Values.metrics.enable .Values.certManager.enable }}
      - name: metrics-certs
        secret:
          secretName: metrics-server-cert
      {{- end }}
      {{- if .Values.extensionApi.enable }}
      - name: extension-server-cert
        secret:
          secretName: extension-server-cert
      {{- end }}
VOLEOF

# --- Controller ServiceAccount ---
# v2-alpha doesn't generate a ServiceAccount resource; add one explicitly.
echo "Creating controller ServiceAccount template..."
cat > "${CHART_DIR}/templates/rbac/service-account.yaml" << 'SAEOF'
apiVersion: v1
kind: ServiceAccount
metadata:
  labels:
    app.kubernetes.io/managed-by: {{ .Release.Service }}
    app.kubernetes.io/name: {{ include "jupyter-k8s.name" . }}
    helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
    app.kubernetes.io/instance: {{ .Release.Name }}
  name: {{ include "jupyter-k8s.resourceName" (dict "suffix" "controller-manager" "context" $) }}
  namespace: {{ .Release.Namespace }}
SAEOF

# --- PodDisruptionBudget template ---
echo "Creating PodDisruptionBudget template..."
cat > "${CHART_DIR}/templates/manager/poddisruptionbudget.yaml" << 'PDBEOF'
{{- if .Values.manager.podDisruptionBudget.enabled }}
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: {{ include "jupyter-k8s.resourceName" (dict "suffix" "controller-manager" "context" $) }}
  namespace: {{ .Release.Namespace }}
  labels:
    app.kubernetes.io/managed-by: {{ .Release.Service }}
    app.kubernetes.io/name: {{ include "jupyter-k8s.name" . }}
    helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
    app.kubernetes.io/instance: {{ .Release.Name }}
    control-plane: controller-manager
    workspace.jupyter.org/component: controller
spec:
  {{- if .Values.manager.podDisruptionBudget.minAvailable }}
  minAvailable: {{ .Values.manager.podDisruptionBudget.minAvailable }}
  {{- end }}
  {{- if .Values.manager.podDisruptionBudget.maxUnavailable }}
  maxUnavailable: {{ .Values.manager.podDisruptionBudget.maxUnavailable }}
  {{- end }}
  selector:
    matchLabels:
      app.kubernetes.io/name: {{ include "jupyter-k8s.name" . }}
      control-plane: controller-manager
{{- end }}
PDBEOF

# --- Wrap extras/jwt-rotator.yaml with conditional + template values ---
echo "Templating JWT rotator CronJob..."
ROTATOR_YAML="${CHART_DIR}/templates/extras/jwt-rotator.yaml"
cat > "${ROTATOR_YAML}" << 'ROTATOREOF'
{{- if and .Values.extensionApi.enable .Values.extensionApi.jwtSecret.enable }}
apiVersion: batch/v1
kind: CronJob
metadata:
  labels:
    app: extensionapi-jwt-rotator
    component: security
  name: {{ include "jupyter-k8s.resourceName" (dict "suffix" "jwt-rotator" "context" $) }}
  namespace: {{ .Release.Namespace }}
spec:
  concurrencyPolicy: Forbid
  failedJobsHistoryLimit: 3
  jobTemplate:
    spec:
      backoffLimit: 3
      template:
        metadata:
          labels:
            app: extensionapi-jwt-rotator
            component: security
        spec:
          containers:
          - env:
            - name: SECRET_NAME
              value: {{ .Values.extensionApi.jwtSecret.secretName }}
            - name: SECRET_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
            - name: TOKEN_TTL
              value: {{ .Values.extensionApi.jwtSecret.tokenTTL | quote }}
            - name: ROTATION_INTERVAL
              value: {{ .Values.extensionApi.jwtSecret.rotationInterval | quote }}
            - name: DRY_RUN
              value: "false"
            image: "{{ .Values.extensionApi.jwtSecret.rotator.repository }}/{{ .Values.extensionApi.jwtSecret.rotator.imageName }}:{{ .Values.extensionApi.jwtSecret.rotator.imageTag | default .Chart.AppVersion }}"
            imagePullPolicy: {{ .Values.extensionApi.jwtSecret.rotator.imagePullPolicy }}
            name: rotator
            resources:
              {{- toYaml .Values.extensionApi.jwtSecret.rotator.resources | nindent 14 }}
            securityContext:
              allowPrivilegeEscalation: false
              capabilities:
                drop:
                - ALL
              readOnlyRootFilesystem: true
          restartPolicy: OnFailure
          securityContext:
            fsGroup: 65532
            runAsGroup: 65532
            runAsNonRoot: true
            runAsUser: 65532
            seccompProfile:
              type: RuntimeDefault
          serviceAccountName: {{ include "jupyter-k8s.resourceName" (dict "suffix" "jwt-rotator" "context" $) }}
  schedule: '*/15 * * * *'
  successfulJobsHistoryLimit: 3
{{- end }}
ROTATOREOF

# --- Wrap extras/extensionapi-secrets.yaml with conditional + random key ---
echo "Templating extensionapi secrets..."
SECRETS_YAML="${CHART_DIR}/templates/extras/extensionapi-secrets.yaml"
cat > "${SECRETS_YAML}" << 'SECRETSEOF'
{{- if and .Values.extensionApi.enable .Values.extensionApi.jwtSecret.enable }}
apiVersion: v1
kind: Secret
metadata:
  name: {{ .Values.extensionApi.jwtSecret.secretName }}
  namespace: {{ .Release.Namespace }}
  labels:
    app: extensionapi-jwt-rotator
    component: security
type: Opaque
data:
  jwt-signing-key-{{ now | unixEpoch }}: {{ randBytes 48 | b64enc | quote }}
{{- end }}
SECRETSEOF

# --- Wrap JWT rotator RBAC with conditionals ---
echo "Wrapping JWT rotator RBAC with conditionals..."
for f in jwt-rotator.yaml jwt-rotator-secrets-manager.yaml jwt-rotator-secrets-manager-binding.yaml; do
    target="${CHART_DIR}/templates/rbac/${f}"
    if [ -f "${target}" ] && ! grep -q "extensionApi" "${target}"; then
        sed -i '1i {{- if and .Values.extensionApi.enable .Values.extensionApi.jwtSecret.enable }}' "${target}"
        echo '{{- end }}' >> "${target}"
    fi
done

# The jwt-secrets-reader role/binding is needed by the controller to read JWT keys.
# Conditional on extensionApi.jwtSecret.enable
for f in jwt-secrets-reader.yaml jwt-secrets-reader-binding.yaml; do
    target="${CHART_DIR}/templates/rbac/${f}"
    if [ -f "${target}" ] && ! grep -q "extensionApi" "${target}"; then
        sed -i '1i {{- if and .Values.extensionApi.enable .Values.extensionApi.jwtSecret.enable }}' "${target}"
        echo '{{- end }}' >> "${target}"
    fi
done

# --- Template secret name in JWT RBAC ---
echo "Templating secret name references in JWT RBAC..."
for f in jwt-rotator-secrets-manager.yaml jwt-secrets-reader.yaml; do
    target="${CHART_DIR}/templates/rbac/${f}"
    if [ -f "${target}" ]; then
        sed -i 's/- jupyter-k8s-extensionapi-secrets/- {{ .Values.extensionApi.jwtSecret.secretName | quote }}/' "${target}"
    fi
done

# --- Wrap extension API resources with conditional ---
echo "Wrapping extension API resources with conditionals..."
for f in extension-server.yaml v1alpha1.connection.workspace.jupyter.org.yaml; do
    target="${CHART_DIR}/templates/extras/${f}"
    if [ -f "${target}" ] && ! grep -q "extensionApi" "${target}"; then
        sed -i '1i {{- if .Values.extensionApi.enable }}' "${target}"
        echo '{{- end }}' >> "${target}"
    fi
done

# Also wrap extension-server-cert
target="${CHART_DIR}/templates/cert-manager/extension-server-cert.yaml"
if [ -f "${target}" ] && ! grep -q "extensionApi" "${target}"; then
    # Insert extensionApi check inside the existing certManager.enable check
    sed -i 's/{{- if .Values.certManager.enable }}/{{- if and .Values.certManager.enable .Values.extensionApi.enable }}/' "${target}"
fi

# --- Fix cert-manager certificate DNS names ---
# v2-alpha hardcodes the service names in cert dnsNames but the actual service names
# use the fullname helper (release-chart-suffix). Fix to use the resource name helper.
echo "Fixing cert-manager certificate DNS names..."
SERVING_CERT="${CHART_DIR}/templates/cert-manager/serving-cert.yaml"
if [ -f "${SERVING_CERT}" ]; then
    sed -i 's|jupyter-k8s-controller-manager\.{{ .Release.Namespace }}|{{ include "jupyter-k8s.resourceName" (dict "suffix" "controller-manager" "context" $) }}.{{ .Release.Namespace }}|g' "${SERVING_CERT}"
fi
EXT_CERT="${CHART_DIR}/templates/cert-manager/extension-server-cert.yaml"
if [ -f "${EXT_CERT}" ]; then
    sed -i 's|jupyter-k8s-extension-server\.{{ .Release.Namespace }}|{{ include "jupyter-k8s.resourceName" (dict "suffix" "extension-server" "context" $) }}.{{ .Release.Namespace }}|g' "${EXT_CERT}"
fi

# Fix hardcoded cert name in APIService annotation
APISERVICE_YAML="${CHART_DIR}/templates/extras/v1alpha1.connection.workspace.jupyter.org.yaml"
if [ -f "${APISERVICE_YAML}" ]; then
    sed -i 's|{{ .Release.Namespace }}/jupyter-k8s-extension-server-cert|{{ .Release.Namespace }}/{{ include "jupyter-k8s.resourceName" (dict "suffix" "extension-server-cert" "context" $) }}|' "${APISERVICE_YAML}"
fi

# --- Create extension API auth RoleBinding in kube-system ---
echo "Creating extension API auth RoleBinding template..."
cat > "${CHART_DIR}/templates/rbac/extension-api-auth-binding.yaml" << 'AUTHEOF'
{{- if .Values.extensionApi.enable }}
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  labels:
    app.kubernetes.io/managed-by: {{ .Release.Service }}
    app.kubernetes.io/name: {{ include "jupyter-k8s.name" . }}
    helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
    app.kubernetes.io/instance: {{ .Release.Name }}
  name: jupyter-k8s-extension-api-auth
  namespace: kube-system
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: extension-apiserver-authentication-reader
subjects:
- kind: ServiceAccount
  name: {{ include "jupyter-k8s.resourceName" (dict "suffix" "controller-manager" "context" $) }}
  namespace: {{ .Release.Namespace }}
{{- end }}
AUTHEOF

# --- Remove connection CRDs if present ---
echo "Removing connection CRDs from chart..."
find "${CHART_DIR}/templates/crd/" -name "connection*" -delete 2>/dev/null || true

# --- Remove webhook port override from manager args ---
# v2-alpha adds --webhook-cert-path conditionally, which is good.
# But we need to also handle the case where webhook is disabled but extensionApi is enabled
# (extensionApi needs cert volumes too). Already handled by our conditional above.

# --- values.yaml: add CLUSTER_ADMIN_GROUP to manager.env ---
# The webhook reads this via os.Getenv to bypass workspace validation for cluster admins.
if ! grep -q "CLUSTER_ADMIN_GROUP" "${CHART_DIR}/values.yaml"; then
    echo "Adding CLUSTER_ADMIN_GROUP env var..."
    sed -i '/fieldPath: spec.serviceAccountName$/a\    - name: CLUSTER_ADMIN_GROUP\n      value: "cluster-workspace-admin"' "${CHART_DIR}/values.yaml"
fi

# --- manager.yaml: add imagePullSecrets support ---
if ! grep -q "imagePullSecrets" "${MANAGER_YAML}"; then
    echo "Adding imagePullSecrets support..."
    sed -i '/serviceAccountName:.*controller-manager/a\      {{- with .Values.manager.imagePullSecrets }}\n      imagePullSecrets: {{- toYaml . | nindent 8 }}\n      {{- end }}' "${MANAGER_YAML}"
fi


# --- Set fullnameOverride so resource names are jupyter-k8s-* regardless of release name ---
sed -i 's/^# fullnameOverride: ""/fullnameOverride: "jupyter-k8s"/' "${CHART_DIR}/values.yaml"

# --- Set GHCR image defaults ---
# Manager image: use GHCR path with empty tag (falls back to .Chart.AppVersion)
echo "Setting GHCR image defaults..."
sed -i '/^  image:$/,/^  ##/{
    s|repository: .*|repository: ghcr.io/jupyter-infra/jupyter-k8s-controller|
    s|tag: .*|tag: ""|
}' "${CHART_DIR}/values.yaml"

# --- Manager image tag: use appVersion fallback ---
echo "Setting appVersion fallback for image tags..."
sed -i 's#image: "{{ .Values.manager.image.repository }}:{{ .Values.manager.image.tag }}"#image: "{{ .Values.manager.image.repository }}:{{ .Values.manager.image.tag | default .Chart.AppVersion }}"#' "${MANAGER_YAML}"

# --- Ensure rbacHelpers defaults to true ---
# Our chart has always shipped these roles; v2-alpha defaults to false
sed -i 's/rbacHelpers:/rbacHelpers:/' "${CHART_DIR}/values.yaml"
sed -i 's/  # Install convenience admin\/editor\/viewer roles for CRDs/  # Install convenience admin\/editor\/viewer roles for CRDs/' "${CHART_DIR}/values.yaml"
sed -i '/^  # Install convenience admin\/editor\/viewer roles for CRDs/{n;s/enable: false/enable: true/}' "${CHART_DIR}/values.yaml"

# --- Remove args that are now injected by the template from values ---
# These are controlled by dedicated values sections, not the args list
sed -i '/--watch-traefik/d' "${CHART_DIR}/values.yaml"
sed -i '/--enable-extension-api/d' "${CHART_DIR}/values.yaml"
sed -i '/--application-images-pull-policy/d' "${CHART_DIR}/values.yaml"
sed -i '/--application-images-registry/d' "${CHART_DIR}/values.yaml"
sed -i '/--default-template-namespace/d' "${CHART_DIR}/values.yaml"

# --- Restore CI workflow files that kubebuilder overwrites with local values ---
# kubebuilder edit substitutes CONTAINER_TOOL and IMG with local defaults
# (e.g. finch instead of docker), breaking the CI workflow.
echo "Restoring CI workflow files..."
git checkout -- "${SCRIPT_DIR}/../.github/workflows/test-chart.yml" 2>/dev/null || true

echo "Helm chart patches applied successfully"
