{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "regcred-injector.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "regcred-injector.fullname" -}}
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
{{- define "regcred-injector.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "regcred-injector.labels" -}}
helm.sh/chart: {{ include "regcred-injector.chart" . }}
{{ include "regcred-injector.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "regcred-injector.selectorLabels" -}}
app.kubernetes.io/name: {{ include "regcred-injector.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: "regcred-injector"
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "regcred-injector.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "regcred-injector.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "regcred-injector.gen-certs" -}}
    {{- $ca := genCA (include "regcred-injector.fullname" .) 3650 -}}

    {{- $cn := printf "%s.%s.svc" (include "regcred-injector.fullname" .) .Release.Namespace -}}
    {{- $san := list $cn -}}
    {{- $cert := genSignedCert $cn nil $san 3650 $ca -}}

        {{- printf "%s$%s$%s" ($cert.Cert | b64enc) ($cert.Key | b64enc) ($ca.Cert | b64enc) -}}
{{- end -}}
