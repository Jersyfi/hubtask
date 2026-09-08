#!/usr/bin/env bash
# SPDX-License-Identifier: BUSL-1.1
# Copyright (c) 2026 Jérôme Bastian Winkel
#
# RT-9 in CI: a point-in-time recovery to a moment between two writes, on a real CloudNativePG
# operator against a real object store, with the first write surviving and the second not.
#
# Production does not exist yet (docs/backlog/blocked-production-namespace.md), and a drill that
# has only ever been rendered is a drill nobody has seen work. So the whole path runs here: the
# chart's Cluster and its backup stanza, a base backup, WAL archiving into MinIO, the drill program
# from the image, a temporary cluster bootstrapped to a computed target, the checks, the teardown.
#
# What this cannot prove, and does not claim to: the *size* of the numbers. A kind cluster on a
# shared runner measures the runner. RPO and RTO become real figures only against production, where
# they stay internal (decision 7 of milestone 0.6.0).
#
# It expects a kind cluster to exist - the workflow creates one - and leaves everything behind for
# the job's logs; the cluster is thrown away with the runner.

set -euo pipefail

cd "$(dirname "$0")/.."

TAG="${1:?usage: pitr-drill.sh <image tag>}"
IMAGE="${HUBTASK_IMAGE:-ghcr.io/jersyfi/hubtask}"
NAMESPACE="${PITR_NAMESPACE:-hubtask-pitr}"
CLUSTER="hubtask-db"
DRILL_CLUSTER="hubtask-db-drill"
BUCKET="hubtask-backups"
# The manifest is pinned by version *and* by checksum, like every other tool this project
# downloads: an unpinned install is a supply chain decision made by whoever is on the network
# (ADR-0015). CNPG_VERSION and CNPG_SHA256 come from the Makefile so there is one place to change.
CNPG_VERSION="${CNPG_VERSION:?CNPG_VERSION must be set (see the Makefile)}"
CNPG_SHA256="${CNPG_SHA256:?CNPG_SHA256 must be set (see the Makefile)}"
HELM="${HELM:-.tools/helm}"

# Drawn rather than written down, for the reason the other smoke scripts give: a literal here would
# be a credential in the repository even though it protects nothing that outlives this job (SG-7).
SECRET_KEY="$(head -c 32 /dev/urandom | base64)"
S3_ACCESS_KEY="$(head -c 12 /dev/urandom | base64 | tr -d '/+=')"
S3_SECRET_KEY="$(head -c 24 /dev/urandom | base64 | tr -d '/+=')"

# scrape reads a metrics endpoint from the runner rather than from inside a container.
#
# `kubectl exec ... curl` is the shorter spelling and it does not work here: the application's
# image is distroless and has neither a shell nor a fetcher, and the operator's PostgreSQL image is
# not required to carry one either. Port-forwarding asks nothing of the image.
scrape() {
	local target="$1" port="$2" out
	out="$(mktemp)"
	kubectl -n "$NAMESPACE" port-forward "$target" ":$port" >"$out" 2>&1 &
	local forward=$!
	local local_port=""
	for _ in $(seq 1 30); do
		local_port="$(sed -n 's/^Forwarding from 127\.0\.0\.1:\([0-9]*\).*/\1/p' "$out" | head -1)"
		[ -n "$local_port" ] && break
		sleep 1
	done
	if [ -z "$local_port" ]; then
		kill "$forward" 2>/dev/null || true
		return 1
	fi
	curl -sf --max-time 30 "http://127.0.0.1:$local_port/metrics"
	local status=$?
	kill "$forward" 2>/dev/null || true
	wait "$forward" 2>/dev/null || true
	return $status
}

fail() {
	echo "FAILED: $*"
	echo "--- what the namespace looked like ---"
	kubectl -n "$NAMESPACE" get pods,clusters,backups,jobs 2>/dev/null || true
	kubectl -n "$NAMESPACE" describe cluster "$CLUSTER" 2>/dev/null | tail -40 || true
	kubectl -n "$NAMESPACE" logs -l app.kubernetes.io/component=restore-drill --tail=120 --all-containers 2>/dev/null || true
	exit 1
}

echo "--- the CloudNativePG operator, pinned and verified ---"
manifest="$(mktemp)"
curl -fsSL -o "$manifest" \
	"https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-${CNPG_VERSION%.*}/releases/cnpg-${CNPG_VERSION}.yaml"
actual="$(shasum -a 256 "$manifest" | cut -d' ' -f1)"
if [ "$actual" != "$CNPG_SHA256" ]; then
	echo "FAILED: the operator manifest for $CNPG_VERSION hashes to $actual, expected $CNPG_SHA256"
	exit 1
fi
# Server-side, because the CRDs are larger than the annotation a client-side apply would write.
kubectl apply --server-side -f "$manifest"
kubectl -n cnpg-system rollout status deployment/cnpg-controller-manager --timeout=300s

kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

echo "--- the image this commit produces, inside the cluster ---"
kind load docker-image "$IMAGE:$TAG" --name "${KIND_CLUSTER:-hubtask}"

echo "--- an object store for the archive ---"
# MinIO stands in for whatever S3-compatible storage the platform provides. One pod, one bucket,
# no persistence: it exists for the length of this job, and the archive it holds is written and
# read back inside it.
kubectl -n "$NAMESPACE" create secret generic minio-credentials \
	--from-literal=access-key="$S3_ACCESS_KEY" \
	--from-literal=secret-key="$S3_SECRET_KEY" \
	--dry-run=client -o yaml | kubectl apply -f -

kubectl -n "$NAMESPACE" apply -f - <<MANIFEST
apiVersion: apps/v1
kind: Deployment
metadata:
  name: minio
spec:
  replicas: 1
  selector: { matchLabels: { app: minio } }
  template:
    metadata: { labels: { app: minio } }
    spec:
      containers:
        - name: minio
          image: quay.io/minio/minio:RELEASE.2025-04-22T22-12-26Z
          args: ["server", "/data", "--console-address", ":9001"]
          env:
            - name: MINIO_ROOT_USER
              valueFrom: { secretKeyRef: { name: minio-credentials, key: access-key } }
            - name: MINIO_ROOT_PASSWORD
              valueFrom: { secretKeyRef: { name: minio-credentials, key: secret-key } }
          ports: [{ containerPort: 9000 }]
          readinessProbe:
            httpGet: { path: /minio/health/live, port: 9000 }
            initialDelaySeconds: 5
            periodSeconds: 5
          volumeMounts: [{ name: data, mountPath: /data }]
      volumes: [{ name: data, emptyDir: {} }]
---
apiVersion: v1
kind: Service
metadata:
  name: minio
spec:
  selector: { app: minio }
  ports: [{ port: 9000, targetPort: 9000 }]
MANIFEST
kubectl -n "$NAMESPACE" rollout status deployment/minio --timeout=300s

# The bucket has to exist before the first archive command runs; barman creates paths, not buckets.
# `mc` waits for the service by name rather than by address, which is why this is a Job in the
# cluster rather than a port-forward from the runner.
kubectl -n "$NAMESPACE" delete job create-bucket --ignore-not-found
kubectl -n "$NAMESPACE" apply -f - <<MANIFEST
apiVersion: batch/v1
kind: Job
metadata:
  name: create-bucket
spec:
  backoffLimit: 5
  template:
    spec:
      restartPolicy: OnFailure
      containers:
        - name: mc
          image: quay.io/minio/mc:RELEASE.2025-04-16T18-13-26Z
          command: ["/bin/sh", "-c"]
          args:
            - |
              mc alias set store http://minio:9000 "\$ACCESS_KEY" "\$SECRET_KEY" &&
              mc mb --ignore-existing store/$BUCKET &&
              mc ls store
          env:
            - name: ACCESS_KEY
              valueFrom: { secretKeyRef: { name: minio-credentials, key: access-key } }
            - name: SECRET_KEY
              valueFrom: { secretKeyRef: { name: minio-credentials, key: secret-key } }
MANIFEST
kubectl -n "$NAMESPACE" wait --for=condition=complete job/create-bucket --timeout=180s || fail "the bucket was not created"

echo "--- the release, with the database the chart owns ---"
kubectl -n "$NAMESPACE" create secret generic hubtask-secrets \
	--from-literal=secret-key="$SECRET_KEY" \
	--from-literal=db-dsn="placeholder" \
	--dry-run=client -o yaml | kubectl apply -f -

# The chart renders the Cluster, its ScheduledBackup and the migration hook. The drill is switched
# *off* for this install and switched on by the upgrade below: its hook runs at the end of a
# release, and at the end of this one there is not yet a base backup to recover to. Turning it on
# afterwards runs the same hook, the same pod and the same program - one release later, which is
# exactly the position every release after the first is in.
"$HELM" install hubtask k8s \
	--namespace "$NAMESPACE" \
	--set image.repository="$IMAGE" \
	--set-string image.tag="$TAG" \
	--set image.pullPolicy=Never \
	--set existingSecret=hubtask-secrets \
	--set database.enabled=true \
	--set database.storage.size=2Gi \
	--set 'database.resources.requests.cpu=100m' \
	--set 'database.resources.requests.memory=256Mi' \
	--set 'database.resources.limits.memory=1Gi' \
	--set database.backup.destinationPath="s3://$BUCKET/" \
	--set database.backup.endpointURL=http://minio:9000 \
	--set database.backup.existingSecret=minio-credentials \
	--set migration.dsnSecretName="$CLUSTER-app" \
	--set migration.dsnSecretKey=uri \
	--set restoreDrill.enabled=false \
	--set roles.api.replicas=1 --set roles.worker.replicas=1 \
	--set roles.scheduler.replicas=1 --set roles.automation.replicas=1 \
	--set config.tenancyMode=single \
	--set storage.kind=local \
	--set networkPolicy.enabled=false \
	--wait --timeout 15m || fail "the release did not become ready"

echo "--- the first base backup, which the drill needs something to recover to ---"
# The ScheduledBackup takes one at creation (`immediate: true`); this waits for it to complete.
for _ in $(seq 1 60); do
	phase="$(kubectl -n "$NAMESPACE" get backup -o jsonpath='{.items[0].status.phase}' 2>/dev/null || true)"
	[ "$phase" = "completed" ] && break
	[ "$phase" = "failed" ] && fail "the first base backup failed"
	sleep 10
done
[ "$phase" = "completed" ] || fail "no base backup completed within ten minutes (phase: ${phase:-none})"

echo "--- the metric names A-12's rules read, against a real instance ---"
# The half a promtool test cannot prove: that the operator publishes these series under these
# names. A rule reading a name nobody emits is silent rather than noisy, so a rename has to turn
# this build red (observability-reliability.md §11).
cnpg_metrics="$(scrape "pod/$CLUSTER-1" 9187)" || fail "the database's metrics port did not answer"
missing=0
for metric in \
	cnpg_collector_pg_wal_archive_status \
	cnpg_collector_first_recoverability_point \
	cnpg_collector_last_available_backup_timestamp \
	cnpg_collector_up; do
	if ! printf '%s' "$cnpg_metrics" | grep -q "^# TYPE $metric "; then
		echo "  MISSING: $metric is read by deploy/observability/alerts/prometheus-rules-pitr.yaml"
		missing=1
	else
		echo "  $metric"
	fi
done
[ "$missing" -eq 0 ] || fail "a rule reads a metric this operator does not publish"

echo "--- the drill: a restore to a point between two writes ---"
# Run the way a release runs it: `helm upgrade` fires the post-upgrade hook, and --wait waits for
# it. Nothing is rewritten or re-created by hand here, so what this proves is the object the chart
# actually ships rather than a copy of it.
#
# failRelease is on, and only here. In an installation a failed drill is a page rather than a
# failed release (k8s/values.yaml); a build wants the exit code, and this is the build.
if "$HELM" upgrade hubtask k8s \
	--namespace "$NAMESPACE" \
	--reuse-values \
	--set restoreDrill.enabled=true \
	--set restoreDrill.failRelease=true \
	--set restoreDrill.evidence.bucket="" \
	--wait --timeout 25m; then
	drill=passed
else
	drill=failed
fi

echo "--- what the drill said ---"
# The hook's Job is kept until the next one (`before-hook-creation`), so its log is here whichever
# way it went.
kubectl -n "$NAMESPACE" logs job/hubtask-restore-drill --tail=200 || true

[ "$drill" = passed ] || fail "the drill ran and did not pass"

echo "--- and what it left behind ---"
# Three properties the log alone would not prove.
#
# 1. The record the gauge is read from, because A-20 has waited for it since 0.4.5.
record="$(kubectl -n "$NAMESPACE" get configmap hubtask-restore-drill -o jsonpath='{.data.last_success_unix}' 2>/dev/null || true)"
case "$record" in
	'') fail "the drill wrote no last_success_unix into its record" ;;
	*[!0-9]*) fail "the record holds something that is not a Unix timestamp" ;;
esac
echo "  the record carries a timestamp"

# 2. The gauge itself, through the mounted record and out of the application's metrics endpoint -
#    the whole path, not just the file.
kubectl -n "$NAMESPACE" rollout restart deployment/hubtask-api
kubectl -n "$NAMESPACE" rollout status deployment/hubtask-api --timeout=300s
api="$(kubectl -n "$NAMESPACE" get pod -l app.kubernetes.io/component=api -o jsonpath='{.items[0].metadata.name}')"
app_metrics="$(scrape "pod/$api" 9090)" || fail "the application's operations port did not answer"
printf '%s' "$app_metrics" | grep -q "^hubtask_restore_drill_last_success_timestamp_seconds " \
	|| fail "the drill passed and the gauge A-20 reads is still absent"
echo "  the gauge reports the drill"

# While the scrape is here: the families a production dashboard and the burn alerts are built on
# (observability-reliability.md §4). They are emitted by seeding rather than by traffic, so their
# absence is a wiring defect rather than a quiet cluster - which is exactly what this catches.
for family in \
	hubtask_http_requests_total \
	hubtask_http_request_duration_seconds \
	hubtask_db_pool_connections \
	hubtask_job_queue_depth \
	hubtask_build_info; do
	printf '%s' "$app_metrics" | grep -q "^# TYPE $family " || fail "$family is not on /metrics"
done
echo "  the RED, pool and queue families are on the endpoint"

# 3. The temporary cluster is gone. A drill that leaves one behind is a drill that fails the next
#    one on quota, and it is the property most easily lost in a refactor of the teardown.
if kubectl -n "$NAMESPACE" get cluster "$DRILL_CLUSTER" >/dev/null 2>&1; then
	fail "the temporary cluster $DRILL_CLUSTER was left behind"
fi
echo "  the temporary cluster is gone"

echo
echo "pitr: a recovery to a point between two writes kept the first and not the second"
