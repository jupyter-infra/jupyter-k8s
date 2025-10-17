# [AWS TRAEFIK DEX]: Configuration for aws-traefik-dex deployment mode
{{- if (.Capabilities.APIVersions.Has "helm.toolkit.fluxcd.io/v2beta1") }}
{{- fail "This chart is not compatible with Flux CD. Please use a different deployment method." }}
{{- end }}

{{- if not .Values.domain }}
{{- fail "domain is required" }}
{{- end }}

# This file intentionally does not produce any Kubernetes resources
# It only validates and sets default values