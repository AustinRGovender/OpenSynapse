{{/*
Expand the name of the chart.
*/}}
{{- define "opensynapse.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "opensynapse.fullname" -}}
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
Common labels
*/}}
{{- define "opensynapse.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{ include "opensynapse.selectorLabels" . }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "opensynapse.selectorLabels" -}}
app.kubernetes.io/name: {{ include "opensynapse.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Control plane selector labels
*/}}
{{- define "opensynapse.controlPlane.selectorLabels" -}}
{{ include "opensynapse.selectorLabels" . }}
app.kubernetes.io/component: control-plane
{{- end }}

{{/*
Web UI selector labels
*/}}
{{- define "opensynapse.web.selectorLabels" -}}
{{ include "opensynapse.selectorLabels" . }}
app.kubernetes.io/component: web
{{- end }}

{{/*
Postgres selector labels
*/}}
{{- define "opensynapse.postgres.selectorLabels" -}}
{{ include "opensynapse.selectorLabels" . }}
app.kubernetes.io/component: postgres
{{- end }}

{{/*
MinIO selector labels
*/}}
{{- define "opensynapse.minio.selectorLabels" -}}
{{ include "opensynapse.selectorLabels" . }}
app.kubernetes.io/component: minio
{{- end }}

{{/*
Postgres DSN
*/}}
{{- define "opensynapse.postgres.dsn" -}}
postgres://{{ .Values.postgres.username }}:{{ .Values.postgres.password }}@{{ .Release.Name }}-postgres:{{ .Values.postgres.port }}/{{ .Values.postgres.database }}?sslmode=disable
{{- end }}
