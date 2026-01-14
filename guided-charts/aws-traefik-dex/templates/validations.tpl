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

{{/* Validate rotator configuration if enabled */}}
{{- if .Values.rotator.enabled }}
{{- if not .Values.rotator.rotationInterval }}
{{- fail "rotator.rotationInterval is required when rotator is enabled" }}
{{- end }}
{{- if not (or (hasSuffix "m" .Values.rotator.rotationInterval) (hasSuffix "h" .Values.rotator.rotationInterval)) }}
{{- fail "rotator.rotationInterval must end with 'm' (minutes) or 'h' (hours)" }}
{{- end }}
{{- if not .Values.rotator.numberOfKeys }}
{{- fail "rotator.numberOfKeys is required when rotator is enabled" }}
{{- end }}
{{- if lt (.Values.rotator.numberOfKeys | int) 1 }}
{{- fail "rotator.numberOfKeys must be at least 1" }}
{{- end }}

{{/* Validate key retention is sufficient for JWT expiration */}}
{{- $jwtExpirationMinutes := 0 }}
{{- if hasSuffix "h" .Values.authmiddleware.jwtExpiration }}
{{- $jwtExpirationMinutes = (trimSuffix "h" .Values.authmiddleware.jwtExpiration | int | mul 60) }}
{{- else if hasSuffix "m" .Values.authmiddleware.jwtExpiration }}
{{- $jwtExpirationMinutes = (trimSuffix "m" .Values.authmiddleware.jwtExpiration | int) }}
{{- else }}
{{- fail "authmiddleware.jwtExpiration must end with 'm' (minutes) or 'h' (hours)" }}
{{- end }}

{{- $rotationIntervalMinutes := 0 }}
{{- if hasSuffix "h" .Values.rotator.rotationInterval }}
{{- $rotationIntervalMinutes = (trimSuffix "h" .Values.rotator.rotationInterval | int | mul 60) }}
{{- else if hasSuffix "m" .Values.rotator.rotationInterval }}
{{- $rotationIntervalMinutes = (trimSuffix "m" .Values.rotator.rotationInterval | int) }}
{{- end }}

{{- $retentionMinutes := (mul (.Values.rotator.numberOfKeys | int) $rotationIntervalMinutes) }}
{{- $requiredRetentionMinutes := (add $jwtExpirationMinutes 30) }}
{{- if lt $retentionMinutes $requiredRetentionMinutes }}
{{- fail (printf "Key retention (%d min) must be >= JWT expiration + 30min buffer (%d min). Increase numberOfKeys or rotationInterval." $retentionMinutes $requiredRetentionMinutes) }}
{{- end }}

{{/* Validate jwtNewKeyUseDelay */}}
{{- $newKeyUseDelaySeconds := 0 }}
{{- if hasSuffix "h" .Values.authmiddleware.jwtNewKeyUseDelay }}
{{- $newKeyUseDelaySeconds = (trimSuffix "h" .Values.authmiddleware.jwtNewKeyUseDelay | int | mul 3600) }}
{{- else if hasSuffix "m" .Values.authmiddleware.jwtNewKeyUseDelay }}
{{- $newKeyUseDelaySeconds = (trimSuffix "m" .Values.authmiddleware.jwtNewKeyUseDelay | int | mul 60) }}
{{- else if hasSuffix "s" .Values.authmiddleware.jwtNewKeyUseDelay }}
{{- $newKeyUseDelaySeconds = (trimSuffix "s" .Values.authmiddleware.jwtNewKeyUseDelay | int) }}
{{- else }}
{{- fail "authmiddleware.jwtNewKeyUseDelay must end with 's' (seconds), 'm' (minutes), or 'h' (hours)" }}
{{- end }}

{{/* Convert jwtExpiration to seconds */}}
{{- $jwtExpirationSeconds := 0 }}
{{- if hasSuffix "h" .Values.authmiddleware.jwtExpiration }}
{{- $jwtExpirationSeconds = (trimSuffix "h" .Values.authmiddleware.jwtExpiration | int | mul 3600) }}
{{- else if hasSuffix "m" .Values.authmiddleware.jwtExpiration }}
{{- $jwtExpirationSeconds = (trimSuffix "m" .Values.authmiddleware.jwtExpiration | int | mul 60) }}
{{- else if hasSuffix "s" .Values.authmiddleware.jwtExpiration }}
{{- $jwtExpirationSeconds = (trimSuffix "s" .Values.authmiddleware.jwtExpiration | int) }}
{{- end }}

{{/* Convert rotationInterval to seconds */}}
{{- $rotationIntervalSeconds := (mul $rotationIntervalMinutes 60) }}

{{/* Validate: jwtNewKeyUseDelay < jwtExpiration */}}
{{- if ge $newKeyUseDelaySeconds $jwtExpirationSeconds }}
{{- fail (printf "authmiddleware.jwtNewKeyUseDelay (%s = %d sec) must be less than jwtExpiration (%s = %d sec)" .Values.authmiddleware.jwtNewKeyUseDelay $newKeyUseDelaySeconds .Values.authmiddleware.jwtExpiration $jwtExpirationSeconds) }}
{{- end }}

{{/* Validate: jwtNewKeyUseDelay < rotationInterval */}}
{{- if ge $newKeyUseDelaySeconds $rotationIntervalSeconds }}
{{- fail (printf "authmiddleware.jwtNewKeyUseDelay (%s = %d sec) must be less than rotator.rotationInterval (%s = %d sec)" .Values.authmiddleware.jwtNewKeyUseDelay $newKeyUseDelaySeconds .Values.rotator.rotationInterval $rotationIntervalSeconds) }}
{{- end }}
{{- end }}

# This file intentionally does not produce any Kubernetes resources
# It only validates and sets default values