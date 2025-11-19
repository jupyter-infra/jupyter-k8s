{{/*
Auto-generate secrets if not provided
*/}}
{{- define "defaulter.oauth2ProxyClientSecret" -}}
{{- if .Values.dex.oauth2ProxyClientSecret -}}
{{- .Values.dex.oauth2ProxyClientSecret -}}
{{- else -}}
{{- if not .Values._generated -}}
{{- $_ := set .Values "_generated" dict -}}
{{- end -}}
{{- if not .Values._generated.oauth2ProxyClientSecret -}}
{{- $_ := set .Values._generated "oauth2ProxyClientSecret" (randAlphaNum 32 | lower | trunc 32) -}}
{{- end -}}
{{- .Values._generated.oauth2ProxyClientSecret -}}
{{- end -}}
{{- end -}}