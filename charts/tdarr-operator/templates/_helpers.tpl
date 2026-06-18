{{/* Expand the name of the chart. */}}
{{- define "tdarr-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Fully qualified app name. */}}
{{- define "tdarr-operator.fullname" -}}
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

{{- define "tdarr-operator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Common labels. */}}
{{- define "tdarr-operator.labels" -}}
helm.sh/chart: {{ include "tdarr-operator.chart" . }}
{{ include "tdarr-operator.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "tdarr-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "tdarr-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "tdarr-operator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "tdarr-operator.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/* Resource names. */}}
{{- define "tdarr-operator.serverName" -}}{{ include "tdarr-operator.fullname" . }}-server{{- end -}}
{{- define "tdarr-operator.controllerName" -}}{{ include "tdarr-operator.fullname" . }}-operator{{- end -}}
{{- define "tdarr-operator.nodeJobName" -}}{{ include "tdarr-operator.fullname" . }}-node{{- end -}}
{{- define "tdarr-operator.nodeTemplateConfigMap" -}}{{ include "tdarr-operator.fullname" . }}-node-template{{- end -}}

{{/* The in-cluster URL of the server web/API port, used by the controller. */}}
{{- define "tdarr-operator.serverURL" -}}
http://{{ include "tdarr-operator.serverName" . }}:{{ .Values.server.webUIPort }}
{{- end -}}

{{/*
Shared volumes (media + cache + extraVolumes), mounted on both the server and
the node pods. Claim name for created PVCs is "<fullname>-<key>".
*/}}
{{- define "tdarr-operator.sharedVolumes" -}}
{{- $full := include "tdarr-operator.fullname" . -}}
{{- range $key := list "media" "cache" -}}
{{- $p := index $.Values.persistence $key -}}
{{- if $p.enabled }}
- name: {{ $key }}
  {{- if $p.existingClaim }}
  persistentVolumeClaim:
    claimName: {{ $p.existingClaim }}
  {{- else if $p.create }}
  persistentVolumeClaim:
    claimName: {{ $full }}-{{ $key }}
  {{- else }}
  emptyDir: {}
  {{- end }}
{{- end }}
{{- end }}
{{- with .Values.extraVolumes }}
{{ toYaml . }}
{{- end }}
{{- end -}}

{{- define "tdarr-operator.sharedVolumeMounts" -}}
{{- range $key := list "media" "cache" -}}
{{- $p := index $.Values.persistence $key -}}
{{- if $p.enabled }}
- name: {{ $key }}
  mountPath: {{ $p.mountPath }}
{{- end }}
{{- end }}
{{- with .Values.extraVolumeMounts }}
{{ toYaml . }}
{{- end }}
{{- end -}}
