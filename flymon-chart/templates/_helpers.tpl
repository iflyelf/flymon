{{/*
Expand the name of the chart.
*/}}
{{- define "flymon.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "flymon.fullname" -}}
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
{{- define "flymon.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "flymon.labels" -}}
helm.sh/chart: {{ include "flymon.chart" . }}
{{ include "flymon.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/component: {{ .Values.mode }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "flymon.selectorLabels" -}}
app.kubernetes.io/name: {{ include "flymon.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Gateway labels
*/}}
{{- define "flymon.gateway.labels" -}}
helm.sh/chart: {{ include "flymon.chart" . }}
{{ include "flymon.gateway.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/component: gateway
{{- end }}

{{/*
Gateway selector labels
*/}}
{{- define "flymon.gateway.selectorLabels" -}}
app.kubernetes.io/name: {{ include "flymon.name" . }}-gateway
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "flymon.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "flymon.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Return the proper image name
*/}}
{{- define "flymon.image" -}}
{{- $registryName := .Values.image.registry -}}
{{- $repositoryName := .Values.image.repository -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion | toString -}}
{{- if .Values.global }}
    {{- if .Values.global.imageRegistry }}
        {{- $registryName = .Values.global.imageRegistry -}}
    {{- end -}}
{{- end -}}
{{- if $registryName }}
{{- printf "%s/%s:%s" $registryName $repositoryName $tag -}}
{{- else }}
{{- printf "%s:%s" $repositoryName $tag -}}
{{- end }}
{{- end }}

{{/*
Return the proper Docker Image Registry Secret Names
*/}}
{{- define "flymon.imagePullSecrets" -}}
{{- $pullSecrets := list }}
{{- if .Values.global }}
  {{- range .Values.global.imagePullSecrets }}
    {{- $pullSecrets = append $pullSecrets . }}
  {{- end }}
{{- end }}
{{- range .Values.image.pullSecrets }}
  {{- $pullSecrets = append $pullSecrets . }}
{{- end }}
{{- if $pullSecrets }}
imagePullSecrets:
  {{- range $pullSecrets }}
  - name: {{ . }}
  {{- end }}
{{- end }}
{{- end }}

{{/*
Return the flymon command based on mode
*/}}
{{- define "flymon.command" -}}
{{- if eq .Values.mode "center" -}}
- "flymon"
{{- else if eq .Values.mode "edge" -}}
- "flymon-edge"
{{- else if eq .Values.mode "pushgw" -}}
- "flymon-pushgw"
{{- end -}}
{{- end -}}

{{/*
Return anti-affinity configuration
*/}}
{{- define "flymon.affinities.pods" -}}
{{- $preset := .Values.podAntiAffinityPreset -}}
{{- if eq $preset "soft" }}
podAntiAffinity:
  preferredDuringSchedulingIgnoredDuringExecution:
    - weight: 100
      podAffinityTerm:
        labelSelector:
          matchLabels: {{- include "flymon.selectorLabels" . | nindent 12 }}
        topologyKey: kubernetes.io/hostname
{{- else if eq $preset "hard" }}
podAntiAffinity:
  requiredDuringSchedulingIgnoredDuringExecution:
    - labelSelector:
        matchLabels: {{- include "flymon.selectorLabels" . | nindent 10 }}
      topologyKey: kubernetes.io/hostname
{{- end }}
{{- end }}

{{/*
Gateway anti-affinity
*/}}
{{- define "flymon.gateway.affinities.pods" -}}
{{- $preset := .Values.podAntiAffinityPreset -}}
{{- if eq $preset "soft" }}
podAntiAffinity:
  preferredDuringSchedulingIgnoredDuringExecution:
    - weight: 100
      podAffinityTerm:
        labelSelector:
          matchLabels: {{- include "flymon.gateway.selectorLabels" . | nindent 12 }}
        topologyKey: kubernetes.io/hostname
{{- else if eq $preset "hard" }}
podAntiAffinity:
  requiredDuringSchedulingIgnoredDuringExecution:
    - labelSelector:
        matchLabels: {{- include "flymon.gateway.selectorLabels" . | nindent 10 }}
      topologyKey: kubernetes.io/hostname
{{- end }}
{{- end }}
