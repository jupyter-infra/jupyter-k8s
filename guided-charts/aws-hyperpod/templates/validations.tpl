# [AWS TRAEFIK DEX]: Configuration for aws-traefik-dex deployment mode
{{- if (.Capabilities.APIVersions.Has "helm.toolkit.fluxcd.io/v2beta1") }}
{{- fail "This chart is not compatible with Flux CD. Please use a different deployment method." }}
{{- end }}

{{- if and .Values.clusterWebUI.enabled (not .Values.clusterWebUI.domain) }}
{{- fail "clusterWebUI.domain is required when clusterWebUI.enabled is true" }}
{{- end }}

{{- if and .Values.clusterWebUI.enabled (not .Values.clusterWebUI.auth.csrfAuthKey) }}
{{- fail "clusterWebUI.auth.csrfAuthKey is required when clusterWebUI.enabled is true (generate with: openssl rand -base64 32)" }}
{{- end }}

{{- if and .Values.clusterWebUI.enabled (eq .Values.clusterWebUI.auth.jwtSigningType "kms") (not .Values.clusterWebUI.auth.kmsKeyId) }}
{{- fail "clusterWebUI.auth.kmsKeyId is required when jwtSigningType is 'kms'" }}
{{- end }}

{{- if and .Values.remoteAccess.enabled (not .Values.remoteAccess.ssmManagedNodeRole) }}
{{- fail "remoteAccess.ssmManagedNodeRole is required when remoteAccess.enabled is true" }}
{{- end }}

{{- if and .Values.remoteAccess.enabled (not .Values.remoteAccess.ssmSidecarImage.containerRegistry) }}
{{- fail "remoteAccess.ssmSidecarImage.containerRegistry is required when remoteAccess.enabled is true" }}
{{- end }}

{{- if and .Values.remoteAccess.enabled (not .Values.remoteAccess.ssmSidecarImage.repository) }}
{{- fail "remoteAccess.ssmSidecarImage.repository is required when remoteAccess.enabled is true" }}
{{- end }}

{{- if and .Values.remoteAccess.enabled (not .Values.remoteAccess.ssmSidecarImage.tag) }}
{{- fail "remoteAccess.ssmSidecarImage.tag is required when remoteAccess.enabled is true" }}
{{- end }}

# This file intentionally does not produce any Kubernetes resources
# It only validates and sets default values