{{/*
Expand the name of the chart.
*/}}
{{- define "jupyter-k8s-aws-hyperpod.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "jupyter-k8s-aws-hyperpod.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "jupyter-k8s-aws-hyperpod.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "jupyter-k8s-aws-hyperpod.labels" -}}
helm.sh/chart: {{ include "jupyter-k8s-aws-hyperpod.chart" . }}
{{ include "jupyter-k8s-aws-hyperpod.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "jupyter-k8s-aws-hyperpod.selectorLabels" -}}
app.kubernetes.io/name: {{ include "jupyter-k8s-aws-hyperpod.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Convert rotation interval to cron schedule.
Supports common durations: "Xm" (minutes), "Xh" (hours).
Examples: "5m" converts to every 5 minutes, "1h" converts to every hour.
*/}}
{{- define "hyperpod.rotationCronSchedule" -}}
{{- $interval := .Values.clusterWebUI.rotator.rotationInterval -}}
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
