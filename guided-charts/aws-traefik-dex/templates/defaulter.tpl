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

{{/*
Convert rotation interval to cron schedule.
Supports common durations: "Xm" (minutes), "Xh" (hours).
Examples: "5m" converts to every 5 minutes, "1h" converts to every hour.
*/}}
{{- define "defaulter.rotationCronSchedule" -}}
{{- $interval := .Values.rotator.rotationInterval -}}
{{- if hasSuffix "m" $interval -}}
  {{- $minutes := trimSuffix "m" $interval | int -}}
  {{- if eq $minutes 60 -}}
0 * * * *
  {{- else if le $minutes 59 -}}
*/{{ $minutes }} * * * *
  {{- else -}}
  {{- fail (printf "Invalid rotation interval: %s (minutes must be <= 59)" $interval) -}}
  {{- end -}}
{{- else if hasSuffix "h" $interval -}}
  {{- $hours := trimSuffix "h" $interval | int -}}
  {{- if eq $hours 1 -}}
0 * * * *
  {{- else if le $hours 23 -}}
0 */{{ $hours }} * * *
  {{- else -}}
  {{- fail (printf "Invalid rotation interval: %s (hours must be <= 23)" $interval) -}}
  {{- end -}}
{{- else -}}
  {{- fail (printf "Unsupported rotation interval format: %s (use Xm for minutes or Xh for hours)" $interval) -}}
{{- end -}}
{{- end -}}