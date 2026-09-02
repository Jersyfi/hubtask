{{/*
The names and labels every object in this chart shares.

One image, four deployments (ADR-0014): every object carries the role it belongs to as
app.kubernetes.io/component, and the selectors include it. Without that a service would send
interactive traffic to a worker pod, and a PodDisruptionBudget meant for the API would count
scheduler pods towards its budget.
*/}}

{{- define "hubtask.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
fullname is the release name unless the release is already named after the chart, so that
`helm install hubtask ...` produces `hubtask-api` rather than `hubtask-hubtask-api`.
*/}}
{{- define "hubtask.fullname" -}}
{{- $name := include "hubtask.name" . -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "hubtask.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
The version label carries the application version, not the chart version: it is what
hubtask_build_info reports, and alert A-13 compares the two across a cluster.
*/}}
{{- define "hubtask.labels" -}}
helm.sh/chart: {{ include "hubtask.chart" .root }}
{{ include "hubtask.selectorLabels" . }}
app.kubernetes.io/version: {{ .root.Values.image.tag | default .root.Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .root.Release.Service }}
app.kubernetes.io/part-of: hubtask
{{- end -}}

{{/*
Selector labels are immutable on a Deployment, so they stay to the four that identify the
workload and never carry the version - a rolling update must not need a delete first.
*/}}
{{- define "hubtask.selectorLabels" -}}
app.kubernetes.io/name: {{ include "hubtask.name" .root }}
app.kubernetes.io/instance: {{ .root.Release.Name }}
{{- if .role }}
app.kubernetes.io/component: {{ .role }}
{{- end }}
{{- end -}}

{{- define "hubtask.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- include "hubtask.fullname" . -}}
{{- else -}}
default
{{- end -}}
{{- end -}}

{{/*
image is the reference every pod and the migration hook is started from.

`toString` is load-bearing, not decoration. `--set` infers a type, so a tag of nothing but digits
- `20260831`, or a short commit SHA that happens to carry no letter - arrives as an int64, and
`printf "%s"` renders that as `%!s(int64=20260831)`. Kubernetes then answers `InvalidImageName`
and the pod never starts. It is a defect that hides: the same chart, the same command, works on
every tag that contains a letter (#247).
*/}}
{{- define "hubtask.image" -}}
{{- printf "%s:%s" .Values.image.repository (.Values.image.tag | default .Chart.AppVersion | toString) -}}
{{- end -}}

{{/*
secretName is where the two mandatory secrets come from. There is deliberately no way to put them
into values.yaml: a value ends up in the release history, in a git repository, and in the output of
`helm get values` (deployment.md §6).
*/}}
{{- define "hubtask.secretName" -}}
{{- if .Values.existingSecret -}}
{{- .Values.existingSecret -}}
{{- else -}}
{{- fail "existingSecret is required: Hubtask needs a Kubernetes secret with the keys db-dsn and secret-key. Secrets do not belong in values.yaml." -}}
{{- end -}}
{{- end -}}

{{/*
The environment of one pod: what every role shares, plus the role itself.

The configuration surface is HUBTASK_* only (arc42 §7.4). What is not a secret comes from the
ConfigMap by reference rather than by value, so that changing it is one object and one rollout
rather than a re-render of every deployment.
*/}}
{{- define "hubtask.env" -}}
- name: HUBTASK_ROLES
  value: {{ .role | quote }}
{{- with (index .root.Values.roles .role).loadShedInflight }}
# Admission control, per role rather than per installation: each role is its own deployment with
# its own resources, so the number of requests in flight at which deferrable work is refused is a
# property of this deployment (observability-reliability.md §6, H-11).
- name: HUBTASK_LOAD_SHED_INFLIGHT
  value: {{ . | quote }}
{{- end }}
- name: HUBTASK_DB_DSN
  valueFrom:
    secretKeyRef:
      name: {{ include "hubtask.secretName" .root }}
      key: db-dsn
- name: HUBTASK_SECRET_KEY
  valueFrom:
    secretKeyRef:
      name: {{ include "hubtask.secretName" .root }}
      key: secret-key
{{- if .root.Values.smtp.existingSecretKey }}
- name: HUBTASK_SMTP_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ include "hubtask.secretName" .root }}
      key: {{ .root.Values.smtp.existingSecretKey }}
{{- end }}
{{- if and (eq .root.Values.storage.kind "s3") .root.Values.storage.existingSecret }}
- name: HUBTASK_S3_ACCESS_KEY
  valueFrom:
    secretKeyRef:
      name: {{ .root.Values.storage.existingSecret }}
      key: access-key
- name: HUBTASK_S3_SECRET_KEY
  valueFrom:
    secretKeyRef:
      name: {{ .root.Values.storage.existingSecret }}
      key: secret-key
{{- end }}
{{- end -}}

{{/*
The probes, identical for every role and deliberately so.

/healthz is liveness and touches no dependency: a database outage would otherwise restart every pod
in the cluster at once, turning a recoverable outage into a thundering herd (ADR-0016).
/readyz is readiness and does check them, so an instance that cannot serve is taken out of the
service rather than answering with errors. /startupz gives a slow start room without loosening the
liveness deadline.
*/}}
{{- define "hubtask.probes" -}}
startupProbe:
  httpGet: { path: /startupz, port: ops }
  periodSeconds: 3
  failureThreshold: 40
livenessProbe:
  httpGet: { path: /healthz, port: ops }
  periodSeconds: 10
  timeoutSeconds: 2
  failureThreshold: 3
readinessProbe:
  httpGet: { path: /readyz, port: ops }
  periodSeconds: 5
  timeoutSeconds: 3
  failureThreshold: 2
{{- end -}}
