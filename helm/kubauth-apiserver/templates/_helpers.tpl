
{{/*
Create a default fully qualified app name, to use as base bame for all ressources.
Use the release name by default
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
*/}}
{{- define "kubauth-kit.baseName" -}}
{{- if .Values.baseNameOverride }}
{{- .Values.baseNameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}


{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "kubauth-kit.chartName" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "kubauth-kit.labels" -}}
helm.sh/chart: {{ include "kubauth-kit.chartName" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}


{{/*
Create the name of the monitor job
*/}}
{{- define "monitorJobName" -}}
{{- default (printf "%s-monitor" (include "kubauth-kit.baseName" .)) .Values.monitorJobName }}
{{- end }}


{{/*
Create the name of the patcher job
*/}}
{{- define "patcherJobName" -}}
{{- default (printf "%s-patcher" (include "kubauth-kit.baseName" .)) .Values.patcherJobName }}
{{- end }}
{{/*

Create the name of the unpatcher job
*/}}
{{- define "unPatcherJobName" -}}
{{- default (printf "%s-unpatcher" (include "kubauth-kit.baseName" .)) .Values.unPatcherJobName }}
{{- end }}

{{/*
Create the name of the configuration configmap
*/}}
{{- define "configmapName" -}}
{{- default (printf "%s" (include "kubauth-kit.baseName" .)) .Values.configmapName }}
{{- end }}

{{/*
Create the name of the service account
*/}}
{{- define "serviceAccountName" -}}
{{- default (printf "%s" (include "kubauth-kit.baseName" .)) .Values.serviceAccountName }}
{{- end }}

{{/*
Create the name of the clusterRole
*/}}
{{- define "clusterRoleName" -}}
{{- default (printf "%s" (include "kubauth-kit.baseName" .)) .Values.clusterRoleName }}
{{- end }}
