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
Object storage selector labels
*/}}
{{- define "opensynapse.objectStorage.selectorLabels" -}}
{{ include "opensynapse.selectorLabels" . }}
app.kubernetes.io/component: object-storage
{{- end }}

{{/*
A credential that is generated on install and kept on upgrade: the value
given in values, else the one already stored in the release's app Secret,
else a new random one. Call with (dict "root" . "key" "x" "value" .Values.y).
*/}}
{{- define "opensynapse.credential" -}}
{{- $root := .root -}}
{{- if .value -}}
{{- .value -}}
{{- else -}}
{{- $existing := lookup "v1" "Secret" $root.Release.Namespace (printf "%s-app" $root.Release.Name) -}}
{{- if and $existing $existing.data (hasKey $existing.data .key) -}}
{{- index $existing.data .key | b64dec -}}
{{- else if eq .key "secret-key" -}}
{{- randBytes 32 -}}
{{- else -}}
{{- randAlphaNum 32 -}}
{{- end -}}
{{- end -}}
{{- end }}

{{/*
In-cluster DNS name of the control plane's Service.
*/}}
{{- define "opensynapse.controlPlaneHost" -}}
{{ .Release.Name }}-control-plane.{{ .Release.Namespace }}.svc.cluster.local
{{- end }}

{{/*
Image the runner pods use: workers.image, else the control-plane image.
*/}}
{{- define "opensynapse.runnerImage" -}}
{{- $repo := default .Values.controlPlane.image.repository .Values.workers.image.repository -}}
{{- $tag := default .Values.controlPlane.image.tag .Values.workers.image.tag -}}
{{ $repo }}:{{ $tag }}
{{- end }}

{{/*
Host header values the API accepts.
*/}}
{{- define "opensynapse.allowedHosts" -}}
{{- $hosts := list (printf "%s-control-plane" .Release.Name) (printf "%s-control-plane.%s" .Release.Name .Release.Namespace) (printf "%s-control-plane.%s.svc" .Release.Name .Release.Namespace) (include "opensynapse.controlPlaneHost" .) -}}
{{- if .Values.ingress.enabled -}}
{{- range .Values.ingress.hosts -}}
{{- $hosts = append $hosts .host -}}
{{- end -}}
{{- end -}}
{{- join "," (concat $hosts .Values.controlPlane.allowedHosts) -}}
{{- end }}
