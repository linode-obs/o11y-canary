{{- define "o11y-canary.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{- define "o11y-canary.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := include "o11y-canary.name" . -}}
{{- if contains $name .Release.Name -}}
{{- $name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end }}

{{- define "o11y-canary.labels" -}}
{{- $labels := .Values.labels | default (dict) -}}
app: {{ include "o11y-canary.name" . }}
chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
release: {{ .Release.Name }}
{{- range $k, $v := $labels }}
{{ $k }}: {{ $v }}
{{- end }}
{{- end }}