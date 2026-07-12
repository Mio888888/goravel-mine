{{- define "goravel-mine.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "goravel-mine.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "goravel-mine.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
app.kubernetes.io/name: {{ include "goravel-mine.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "goravel-mine.selectorLabels" -}}
app.kubernetes.io/name: {{ include "goravel-mine.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "goravel-mine.image" -}}
{{- if .Values.image.digest -}}
{{- printf "%s@%s" .Values.image.repository .Values.image.digest -}}
{{- else -}}
{{- printf "%s:%s" .Values.image.repository .Values.image.tag -}}
{{- end -}}
{{- end -}}

{{- define "goravel-mine.secretName" -}}
{{- if .Values.secret.create -}}
{{- printf "%s-secret" (include "goravel-mine.fullname" .) -}}
{{- else if .Values.secret.existingSecret -}}
{{- .Values.secret.existingSecret -}}
{{- else -}}
{{- required "Set secret.existingSecret or secret.create=true with non-placeholder secret.data values" .Values.secret.existingSecret -}}
{{- end -}}
{{- end -}}

{{- define "goravel-mine.storageClaimName" -}}
{{- if .Values.persistence.enabled -}}
{{- printf "%s-storage" (include "goravel-mine.fullname" .) -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end -}}

{{- define "goravel-mine.backupClaimName" -}}
{{- if .Values.backup.existingClaim -}}
{{- .Values.backup.existingClaim -}}
{{- else -}}
{{- printf "%s-backups" (include "goravel-mine.fullname" .) -}}
{{- end -}}
{{- end -}}
