{{/* Labels on every object. */}}
{{- define "service.labels" -}}
{{ include "service.selectorLabels" . }}
app.kubernetes.io/part-of: opspilot
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end }}

{{/* Labels the Service and Deployment use to find the pods. Never change these on a live release. */}}
{{- define "service.selectorLabels" -}}
app.kubernetes.io/name: {{ .Release.Name }}
{{- end }}
