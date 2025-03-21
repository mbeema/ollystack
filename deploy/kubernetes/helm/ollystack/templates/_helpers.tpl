{{/*
Expand the name of the chart.
*/}}
{{- define "ollystack.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "ollystack.fullname" -}}
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
{{- define "ollystack.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "ollystack.labels" -}}
helm.sh/chart: {{ include "ollystack.chart" . }}
{{ include "ollystack.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "ollystack.selectorLabels" -}}
app.kubernetes.io/name: {{ include "ollystack.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Kafka brokers connection string
*/}}
{{- define "ollystack.kafkaBrokers" -}}
{{- if .Values.kafka.enabled }}
{{- printf "%s-kafka:9092" .Release.Name }}
{{- else }}
{{- .Values.externalKafka.brokers | join "," }}
{{- end }}
{{- end }}

{{/*
ClickHouse connection string
*/}}
{{- define "ollystack.clickhouseHost" -}}
{{- if .Values.clickhouse.enabled }}
{{- printf "%s-clickhouse" .Release.Name }}
{{- else }}
{{- .Values.externalClickhouse.host }}
{{- end }}
{{- end }}

{{/*
Redis connection string
*/}}
{{- define "ollystack.redisHost" -}}
{{- if .Values.redis.enabled }}
{{- printf "%s-redis-master:6379" .Release.Name }}
{{- else }}
{{- .Values.externalRedis.host }}
{{- end }}
{{- end }}

{{/*
AlertManager URL
*/}}
{{- define "ollystack.alertmanagerUrl" -}}
{{- if .Values.alertmanager.enabled }}
{{- printf "http://%s-alertmanager:9093" .Release.Name }}
{{- else }}
{{- .Values.externalAlertmanager.url }}
{{- end }}
{{- end }}
