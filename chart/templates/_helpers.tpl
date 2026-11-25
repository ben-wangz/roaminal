{{/* Return the chart name with an optional release-local override. */}}
{{- define "roaminal.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Return a stable release-qualified resource name. */}}
{{- define "roaminal.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else if contains (include "roaminal.name" .) .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name (include "roaminal.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{/* Return the version-safe chart label. */}}
{{- define "roaminal.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Render labels, with required identity labels taking precedence. */}}
{{- define "roaminal.labels" -}}
{{- $ctx := .context -}}
{{- $labels := deepCopy $ctx.Values.commonLabels -}}
{{- if .extra }}{{- $labels = mergeOverwrite $labels .extra -}}{{- end }}
{{- $_ := set $labels "helm.sh/chart" (include "roaminal.chart" $ctx) -}}
{{- $_ := set $labels "app.kubernetes.io/name" (include "roaminal.name" $ctx) -}}
{{- $_ := set $labels "app.kubernetes.io/instance" $ctx.Release.Name -}}
{{- $_ := set $labels "app.kubernetes.io/managed-by" $ctx.Release.Service -}}
{{- $_ := set $labels "app.kubernetes.io/version" $ctx.Chart.AppVersion -}}
{{- toYaml $labels -}}
{{- end -}}

{{/* Render selector labels only from release identity. */}}
{{- define "roaminal.selectorLabels" -}}
app.kubernetes.io/name: {{ include "roaminal.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/* Render annotations with resource-specific values taking precedence. */}}
{{- define "roaminal.annotations" -}}
{{- $ctx := .context -}}
{{- $annotations := deepCopy $ctx.Values.commonAnnotations -}}
{{- if .extra }}{{- $annotations = mergeOverwrite $annotations .extra -}}{{- end }}
{{- if $annotations }}{{- toYaml $annotations -}}{{- end }}
{{- end -}}

{{/* Render the image reference, preferring an immutable digest when supplied. */}}
{{- define "roaminal.image" -}}
{{- $repository := .Values.image.repository -}}
{{- if .Values.image.registry }}{{- $repository = printf "%s/%s" .Values.image.registry .Values.image.repository -}}{{- end }}
{{- if .Values.image.digest -}}
{{- printf "%s@%s" $repository .Values.image.digest -}}
{{- else -}}
{{- printf "%s:%s" $repository .Values.image.tag -}}
{{- end -}}
{{- end -}}

{{/* Return the names used by the chart-owned resources. */}}
{{- define "roaminal.configMapName" -}}
{{- printf "%s-config" (include "roaminal.fullname" .) -}}
{{- end -}}
{{- define "roaminal.pvcName" -}}
{{- if .Values.persistence.existingClaim -}}{{- .Values.persistence.existingClaim -}}{{- else -}}{{- printf "%s-data" (include "roaminal.fullname" .) -}}{{- end -}}
{{- end -}}
{{- define "roaminal.sshSecretName" -}}
{{- .Values.ssh.secret.name -}}
{{- end -}}
{{- define "roaminal.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}{{- default (include "roaminal.fullname" .) .Values.serviceAccount.name -}}{{- else -}}{{- default "default" .Values.serviceAccount.name -}}{{- end -}}
{{- end -}}

{{/* Managed PostgreSQL is selected only by an explicit driver setting. */}}
{{- define "roaminal.postgres.managed" -}}
{{- if and (eq .Values.database.driver "postgres") (not .Values.database.postgres.existingSecret) -}}true{{- end -}}
{{- end -}}

{{/* Stay within CloudNativePG's 50-character limit without losing release identity. */}}
{{- define "roaminal.postgres.clusterName" -}}
{{- $release := .Release.Name -}}
{{- $name := printf "pg-%s" ($release | replace "." "-") -}}
{{- if or (gt (len $name) 50) (contains "." $release) -}}
{{- printf "%s-%s" (trunc 37 $name | trimSuffix "-") (sha256sum $release | trunc 12) -}}
{{- else -}}
{{- $name -}}
{{- end -}}
{{- end -}}

{{- define "roaminal.postgres.secretName" -}}
{{- .Values.database.postgres.existingSecret | default (printf "%s-credentials" (include "roaminal.postgres.clusterName" .)) -}}
{{- end -}}
{{- define "roaminal.postgres.bootstrapName" -}}
{{- printf "%s-bootstrap" (include "roaminal.postgres.clusterName" .) -}}
{{- end -}}
{{- define "roaminal.postgres.host" -}}
{{- printf "%s-rw.%s.svc" (include "roaminal.postgres.clusterName" .) .Release.Namespace -}}
{{- end -}}
{{- define "roaminal.postgres.caSecretName" -}}
{{- if include "roaminal.postgres.managed" . -}}
{{- printf "%s-ca" (include "roaminal.postgres.clusterName" .) -}}
{{- else if eq .Values.database.driver "postgres" -}}
{{- .Values.database.postgres.caSecret.name -}}
{{- end -}}
{{- end -}}
{{- define "roaminal.postgres.caFile" -}}
/etc/roaminal/postgres-ca/ca.crt
{{- end -}}

{{/* Fail early for values that could produce an unusable or unsafe workload. */}}
{{- define "roaminal.validateValues" -}}
{{- if ne (int .Values.replicaCount) 1 -}}
{{- fail "roaminal supports exactly one replica because terminal and SSH transports are process-local" -}}
{{- end -}}
{{- if ne .Values.updateStrategy.type "Recreate" -}}
{{- fail "updateStrategy.type must remain Recreate for process-local terminal state" -}}
{{- end -}}
{{- if empty .Values.auth.existingSecret -}}
{{- fail "auth.existingSecret is required; Roaminal does not accept passwords through values" -}}
{{- end -}}
{{- if .Values.webPush.existingSecret -}}
{{- if empty .Values.webPush.vapidPublicKeyKey }}{{- fail "webPush.vapidPublicKeyKey is required when webPush.existingSecret is set" }}{{- end }}
{{- if empty .Values.webPush.vapidPrivateKeyKey }}{{- fail "webPush.vapidPrivateKeyKey is required when webPush.existingSecret is set" }}{{- end }}
{{- if empty .Values.webPush.subjectKey }}{{- fail "webPush.subjectKey is required when webPush.existingSecret is set" }}{{- end }}
{{- end -}}
{{- if not .Values.app.acceptTerms -}}
{{- fail "app.acceptTerms must be explicitly true before installing Roaminal" -}}
{{- end -}}
{{- if not .Values.persistence.enabled -}}
{{- fail "persistence.enabled must be true for a durable database binding and workspace" -}}
{{- end -}}
{{- if .Values.automountServiceAccountToken -}}
{{- fail "Roaminal application Pods must not automount ServiceAccount tokens" -}}
{{- end -}}
{{- if include "roaminal.postgres.managed" . -}}
{{- $postgres := .Values.database.postgres -}}
{{- if $postgres.caSecret.name -}}
{{- fail "database.postgres.caSecret.name is only for external PostgreSQL; managed PostgreSQL uses the CloudNativePG CA" -}}
{{- end -}}
{{- if has $postgres.dsnKey (list "username" "password") -}}
{{- fail "database.postgres.dsnKey must not shadow username or password in the managed PostgreSQL Secret" -}}
{{- end -}}
{{- if has $postgres.managed.database (list "postgres" "template0" "template1") -}}
{{- fail "database.postgres.managed.database must not use a built-in database name" -}}
{{- end -}}
{{- if or (eq $postgres.managed.username "postgres") (hasPrefix "pg_" $postgres.managed.username) -}}
{{- fail "database.postgres.managed.username must not use a reserved PostgreSQL role name" -}}
{{- end -}}
{{- if eq (include "roaminal.serviceAccountName" .) (include "roaminal.postgres.bootstrapName" .) -}}
{{- fail "the application ServiceAccount must not use the managed PostgreSQL bootstrap identity" -}}
{{- end -}}
{{- end -}}
{{- if ne .Values.persistence.volumeMode "Filesystem" -}}
{{- fail "persistence.volumeMode must be Filesystem because the chart uses subPath directories" -}}
{{- end -}}
{{- if ne (int .Values.app.port) (int .Values.service.port) -}}
{{- fail "app.port and service.port must match" -}}
{{- end -}}
{{- if eq .Values.ssh.source "secret" -}}
{{- if empty .Values.ssh.secret.name }}{{- fail "ssh.secret.name is required when ssh.source is secret" }}{{- end }}
{{- end -}}
{{- if eq .Values.ssh.source "volume" -}}
{{- if empty .Values.ssh.volume.name }}{{- fail "ssh.volume.name is required when ssh.source is volume" }}{{- end }}
{{- end -}}
{{- $paths := list .Values.persistence.paths.state .Values.persistence.paths.workspace .Values.persistence.paths.ssh -}}
{{- if ne (len (uniq $paths)) 3 }}{{- fail "persistence.paths.state, workspace, and ssh must be distinct" }}{{- end }}
{{- $reservedLabels := list "app.kubernetes.io/name" "app.kubernetes.io/instance" "app.kubernetes.io/managed-by" "app.kubernetes.io/version" "helm.sh/chart" -}}
{{- range $label := $reservedLabels }}
{{- if hasKey $.Values.commonLabels $label }}{{- fail (printf "commonLabels cannot override %s" $label) }}{{- end }}
{{- if hasKey $.Values.podLabels $label }}{{- fail (printf "podLabels cannot override %s" $label) }}{{- end }}
{{- end }}
{{- $reservedEnv := list "ROAMINAL_HOST" "ROAMINAL_PORT" "ROAMINAL_PASSWORD" "ROAMINAL_WEBSOCKET_PING_INTERVAL" "ROAMINAL_SCROLLBACK_LINES" "ROAMINAL_MAX_CONNECTION_INSTANCES" "ROAMINAL_MAX_CLIENTS_PER_CONNECTION_INSTANCE" "ROAMINAL_DEBUG" "ROAMINAL_ACCEPT_TERMS" "ROAMINAL_CWD" "ROAMINAL_AUTH_ACCESS_TTL" "ROAMINAL_AUTH_REFRESH_TTL" "ROAMINAL_AUTH_MAX_ATTEMPTS" "ROAMINAL_CLIENT_DIAGNOSTICS_ENABLED" "ROAMINAL_WEB_PUSH_VAPID_PUBLIC_KEY" "ROAMINAL_WEB_PUSH_VAPID_PRIVATE_KEY" "ROAMINAL_WEB_PUSH_SUBJECT" "ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_CACHE_DIR" "ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_CACHE_TARGET_MIB" "ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_CACHE_MAX_AGE" "ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_CACHE_CLEANUP_INTERVAL" "ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_CONVERSIONS" "ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_SOURCE_MIB" "ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_OUTPUT_MIB" "ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_STATIC_PIXELS" "ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_FRAMES" "ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_ANIMATED_PIXELS" "ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_CONVERSION_TIMEOUT" -}}
{{- range .Values.extraEnv }}
{{- if or (has .name $reservedEnv) (hasPrefix "ROAMINAL_DB_" .name) }}{{- fail (printf "extraEnv cannot override reserved variable %s" .name) }}{{- end }}
{{- end }}
{{- range .Values.ssh.fileMounts }}
{{- if not (regexMatch `^/home/roaminal/\.ssh/[^/]+$` .mountPath) }}{{- fail (printf "ssh.fileMounts mountPath must be a direct file below /home/roaminal/.ssh: %s" .mountPath) }}{{- end }}
{{- if empty .name }}{{- fail "ssh.fileMounts entries require a volume name" }}{{- end }}
{{- if empty .subPath }}{{- fail "ssh.fileMounts entries require a subPath" }}{{- end }}
{{- end }}
{{- end -}}
