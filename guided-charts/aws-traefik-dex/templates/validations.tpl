# [AWS TRAEFIK DEX]: Configuration for aws-traefik-dex deployment mode
{{- if (.Capabilities.APIVersions.Has "helm.toolkit.fluxcd.io/v2beta1") }}
{{- fail "This chart is not compatible with Flux CD. Please use a different deployment method." }}
{{- end }}

{{- if not .Values.domain }}
{{- fail "domain is required" }}
{{- end }}

{{- if .Values.storageClass.efs.parameters.fileSystemId }}
{{- if not .Values.storageClass.efs.parameters.fileSystemId }}
{{- fail ".storageClass.efs.parameters.fileSystemId is required" }}
{{- end }}
{{- end }}

{{- if not .Values.certManager.email }}
{{- fail "certManager.email is required" }}
{{- end }}

{{- if not .Values.github.clientId }}
{{- fail "github.clientId is required" }}
{{- end }}

{{- if not .Values.github.clientSecret }}
{{- fail "github.clientSecret is required" }}
{{- end }}

{{- if not .Values.github.orgs }}
{{- fail "At least one organization must be specified" }}
{{- end }}

{{- if not .Values.oauth2Proxy.cookieSecret }}
{{- fail "oauth2Proxy.cookieSecret is required" }}
{{- end }}

# This file intentionally does not produce any Kubernetes resources
# It only validates and sets default values