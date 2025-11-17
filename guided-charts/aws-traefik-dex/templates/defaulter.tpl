{{/*
Auto-generate secrets if not provided
*/}}
{{- define "defaulter.oauth2ProxyClientSecret" -}}
{{- if .Values.dex.oauth2ProxyClientSecret -}}
{{- .Values.dex.oauth2ProxyClientSecret -}}
{{- else -}}
{{- randAlphaNum 32 | lower | trunc 32 -}}
{{- end -}}
{{- end -}}