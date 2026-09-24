#!/usr/bin/env bash
# SPDX-License-Identifier: BUSL-1.1
# Copyright (c) 2026 Jérôme Bastian Winkel
#
# RT-9 in CI: a point-in-time recovery to a moment between two writes, on a real CloudNativePG
# operator against a real object store, with the first write surviving and the second not.
#
# Production does not exist yet (docs/backlog/blocked-production-namespace.md), and a drill that
# has only ever been rendered is a drill nobody has seen work. So the whole path runs here: the
# chart's Cluster and its backup stanza, a base backup, WAL archiving into the object store, the drill program
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

# The CloudNativePG series A-12's rules read, and what this installation expects of each.
#
# Both lists are reconciled with deploy/observability/alerts/prometheus-rules-pitr.yaml by
# test/observability on every pull request: every name a rule reads has to appear in one of them,
# and every name here has to be read by a rule. Without that, a list drifts from the file it
# claims to check and goes on reporting confidently - `cnpg_collector_up` sat in it for months
# without a single rule reading it (#940).
#
# Required: published by the instance manager's own collector, on the primary, once the cluster
# has a backup configured - which the step above has just waited for.
PITR_REQUIRED_METRICS="cnpg_collector_pg_wal_archive_status
cnpg_collector_first_recoverability_point
cnpg_collector_last_available_backup_timestamp"

# Absent by design here. `cnpg_pg_replication_lag` comes from the operator's default monitoring
# queries rather than from the Go collector, and it has a value only where there is a standby to
# measure. ADR-0046 decided one instance - what protects this installation is the archive, not a
# replica - so the rule that reads it is written for the day a second instance is added and is
# silent until then. That is a decision, so it is written down rather than left as a gap.
PITR_ABSENT_METRICS="cnpg_pg_replication_lag"
DRILL_CLUSTER="hubtask-db-drill"
# The S3-compatible server, the same pin test/s3test holds for the Go suites. Overridable for the
# same reason the images there are (#1029).
S3_IMAGE="${HUBTASK_TEST_S3_IMAGE:-chrislusf/seaweedfs:4.47}"
BUCKET="hubtask-backups"
MEDIA_BUCKET="hubtask-media"
# The manifest is pinned by version *and* by checksum, like every other tool this project
# downloads: an unpinned install is a supply chain decision made by whoever is on the network
# (ADR-0015). CNPG_VERSION and CNPG_SHA256 come from the Makefile so there is one place to change.
CNPG_VERSION="${CNPG_VERSION:?CNPG_VERSION must be set (see the Makefile)}"
CNPG_SHA256="${CNPG_SHA256:?CNPG_SHA256 must be set (see the Makefile)}"
HELM="${HELM:-.tools/helm}"

# Drawn rather than written down, for the reason the other smoke scripts give: a literal here would
# be a credential in the repository even though it protects nothing that outlives this job (SG-7).
SECRET_KEY="$(head -c 32 /dev/urandom | base64)"
APP_PASSWORD="$(head -c 24 /dev/urandom | base64 | tr -d '/+=')"
S3_ACCESS_KEY="$(head -c 12 /dev/urandom | base64 | tr -d '/+=')"
S3_SECRET_KEY="$(head -c 24 /dev/urandom | base64 | tr -d '/+=')"

# fetch reads one path from a pod, from the runner rather than from inside a container.
#
# `kubectl exec ... curl` is the shorter spelling and it does not work here: the application's
# image is distroless and has neither a shell nor a fetcher, and the operator's PostgreSQL image is
# not required to carry one either. Port-forwarding asks nothing of the image.
#
# `curl -s` without `-f`: a 404 is an answer, and one of the callers below wants exactly that.
fetch() {
	local target="$1" port="$2" path="$3" out
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
	curl -s --max-time 30 "http://127.0.0.1:$local_port$path"
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

# A namespace left over from an earlier run may still be terminating, and creating objects inside
# one that is being finalised has them swept out from under the run - which reads as a rollout that
# never finishes rather than as a namespace that was not there. CI starts from a fresh cluster and
# never sees this; a laptop running the drill twice does.
for _ in $(seq 1 60); do
	phase="$(kubectl get namespace "$NAMESPACE" -o jsonpath='{.status.phase}' 2>/dev/null || true)"
	[ "$phase" = "Terminating" ] || break
	echo "  waiting for the previous namespace to finish terminating"
	sleep 5
done
kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

echo "--- the image this commit produces, inside the cluster ---"
kind load docker-image "$IMAGE:$TAG" --name "${KIND_CLUSTER:-hubtask}"

echo "--- an object store for the archive ---"
# SeaweedFS stands in for whatever S3-compatible storage the platform provides. One pod, two
# buckets, no persistence: it exists for the length of this job, and the archive it holds is
# written and read back inside it.
#
# It replaced MinIO when MinIO archived its open-source server and client and closed every
# registry that served them (#1029, test/s3test says the rest). The drill and the Go suites share
# that choice on purpose - an object store the tests never meet is an object store nobody proves.
#
# The server's whole authorisation surface is one identity in a JSON file, so the Secret carries
# that file alongside the two plain values the chart wants.
S3_IDENTITIES="{\"identities\":[{\"name\":\"drill\",\"credentials\":[{\"accessKey\":\"$S3_ACCESS_KEY\",\"secretKey\":\"$S3_SECRET_KEY\"}],\"actions\":[\"Admin\",\"Read\",\"Write\",\"List\",\"Tagging\"]}]}"
kubectl -n "$NAMESPACE" create secret generic object-store-credentials \
	--from-literal=access-key="$S3_ACCESS_KEY" \
	--from-literal=secret-key="$S3_SECRET_KEY" \
	--from-literal=s3.json="$S3_IDENTITIES" \
	--dry-run=client -o yaml | kubectl apply -f -

kubectl -n "$NAMESPACE" apply -f - <<MANIFEST
apiVersion: apps/v1
kind: Deployment
metadata:
  name: object-store
spec:
  replicas: 1
  selector: { matchLabels: { app: object-store } }
  template:
    metadata: { labels: { app: object-store } }
    spec:
      containers:
        - name: seaweedfs
          image: $S3_IMAGE
          args: ["server", "-s3", "-s3.config=/etc/seaweedfs/s3.json", "-dir=/data"]
          ports: [{ containerPort: 8333 }, { containerPort: 9333 }]
          readinessProbe:
            # /healthz on the S3 port, which promises the API is taking requests. The bucket
            # creation below is a write, and it is the next thing that happens.
            httpGet: { path: /healthz, port: 8333 }
            initialDelaySeconds: 5
            periodSeconds: 5
          volumeMounts:
            - { name: data, mountPath: /data }
            - { name: config, mountPath: /etc/seaweedfs, readOnly: true }
      volumes:
        - { name: data, emptyDir: {} }
        - name: config
          secret:
            secretName: object-store-credentials
            items: [{ key: s3.json, path: s3.json }]
---
apiVersion: v1
kind: Service
metadata:
  name: object-store
spec:
  selector: { app: object-store }
  ports:
    - { name: s3, port: 8333, targetPort: 8333 }
    # The master, which only the bucket job below talks to.
    - { name: master, port: 9333, targetPort: 9333 }
MANIFEST
kubectl -n "$NAMESPACE" rollout status deployment/object-store --timeout=300s

# The buckets have to exist before the first archive command runs; barman creates paths, not
# buckets. `weed shell` waits for the service by name rather than by address, which is why this is
# a Job in the cluster rather than a port-forward from the runner.
#
# The listing is the proof rather than the exit code: `weed shell` exits 0 on a command it does not
# know and prints the complaint instead, and it blocks rather than failing when it cannot reach the
# master - which is what activeDeadlineSeconds is for.
kubectl -n "$NAMESPACE" delete job create-bucket --ignore-not-found
kubectl -n "$NAMESPACE" apply -f - <<MANIFEST
apiVersion: batch/v1
kind: Job
metadata:
  name: create-bucket
spec:
  backoffLimit: 5
  activeDeadlineSeconds: 150
  template:
    spec:
      restartPolicy: OnFailure
      containers:
        - name: weed
          image: $S3_IMAGE
          command: ["/bin/sh", "-c"]
          args:
            - |
              set -e
              {
                echo "s3.bucket.create -name $BUCKET"
                echo "s3.bucket.create -name $MEDIA_BUCKET"
                echo "s3.bucket.list"
              } | weed shell -master=object-store:9333 | tee /tmp/buckets
              grep -q "$BUCKET" /tmp/buckets
              grep -q "$MEDIA_BUCKET" /tmp/buckets
MANIFEST
kubectl -n "$NAMESPACE" wait --for=condition=complete job/create-bucket --timeout=180s || fail "the bucket was not created"

echo "--- the release, with the database the chart owns ---"
# Two DSNs, the arrangement A-11 asks of Kubernetes (multi-tenancy.md §2.1): the migration runs as
# the owner - out of the Secret CloudNativePG generates, so that credential is never copied - and
# the application connects as hubtask_app, whose login the migrator grants from this password.
# The drill uses both: the owner writes the markers, and hubtask_app is the role whose bounds the
# isolation checks are about.
kubectl -n "$NAMESPACE" create secret generic hubtask-secrets \
	--from-literal=secret-key="$SECRET_KEY" \
	--from-literal=db-dsn="postgres://hubtask_app:$APP_PASSWORD@$CLUSTER-rw:5432/hubtask?sslmode=require" \
	--dry-run=client -o yaml | kubectl apply -f -

# The application role itself, which the operator creates because the migration may not: a
# basic-auth Secret the CloudNativePG Cluster's `managed.roles` reads. The password is the one in
# db-dsn above - two names for one credential, which is why they are drawn together here.
kubectl -n "$NAMESPACE" create secret generic hubtask-app-role \
	--type=kubernetes.io/basic-auth \
	--from-literal=username=hubtask_app \
	--from-literal=password="$APP_PASSWORD" \
	--dry-run=client -o yaml | kubectl apply -f -

# Two steps, and the reason is Helm's hook model rather than a preference.
#
# The migration is a `pre-install` hook, and a pre-install hook runs before *every* regular
# resource in the release - including the CloudNativePG Cluster this chart now renders. So on a
# first install with `database.enabled` there is nothing to migrate yet, and the hook fails looking
# for a Secret the operator has not generated. Argo CD does not have this problem: sync waves order
# a hook against resources, and the Cluster sits in an earlier wave (deployment.md §2.2). Helm has
# no equivalent, so a Helm-driven first install is: create the database, then migrate into it.
#
# Step one, therefore, is the database and the workloads with the migration switched off. The pods
# will not be ready yet - there is no schema - so this one does not wait for them.
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
	--set database.backup.endpointURL=http://object-store:8333 \
	--set database.backup.existingSecret=object-store-credentials \
	--set 'database.postgresql.parameters.archive_timeout=30s' \
	--set migration.dsnSecretName="$CLUSTER-app" \
	--set migration.dsnSecretKey=uri \
	--set database.appRole.passwordSecret=hubtask-app-role \
	--set migration.enabled=false \
	--set restoreDrill.enabled=false \
	--set roles.api.replicas=1 --set roles.worker.replicas=1 \
	--set roles.scheduler.replicas=1 --set roles.automation.replicas=1 \
	--set config.tenancyMode=single \
	--set storage.kind=s3 \
	--set storage.existingSecret=object-store-credentials \
	--set storage.bucket="$MEDIA_BUCKET" \
	--set storage.endpoint=http://object-store:8333 \
	--set networkPolicy.enabled=false \
	|| fail "the release could not be installed"

echo "--- the database, before anything tries to migrate it ---"
for _ in $(seq 1 60); do
	ready="$(kubectl -n "$NAMESPACE" get cluster "$CLUSTER" -o jsonpath='{.status.readyInstances}' 2>/dev/null || true)"
	[ "${ready:-0}" -ge 1 ] && break
	sleep 10
done
[ "${ready:-0}" -ge 1 ] || fail "the CloudNativePG cluster never reported a ready instance"

echo "--- step two: the migration, into a database that exists ---"
"$HELM" upgrade hubtask k8s \
	--namespace "$NAMESPACE" \
	--reuse-values \
	--set migration.enabled=true \
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
#
# The list is PITR_REQUIRED_METRICS below, and `test/observability` reconciles it with the rules
# file in both directions on every pull request - a list that drifts from the rules it claims to
# check is the failure this whole section exists to prevent, and it had already happened:
# `cnpg_collector_up` stood here for months and no rule has ever read it (#940).
cnpg_metrics="$(fetch "pod/$CLUSTER-1" 9187 /metrics)" || fail "the database's metrics port did not answer"
missing=0
for metric in $PITR_REQUIRED_METRICS; do
	# Into a variable and matched from there, never through a pipe. `grep -q` leaves on its first
	# match, the writer gets SIGPIPE, and `set -o pipefail` at the top of this script then turns a
	# *match* into a failed pipeline: a metric that is published is reported missing, and whether it
	# happens depends on where in the payload the match falls. That is how this check spent every
	# night since 2026-09-08 reporting three names the operator publishes perfectly well (#940).
	if grep -q "^# TYPE $metric " <<<"$cnpg_metrics"; then
		echo "  $metric"
	else
		echo "  MISSING: $metric is read by deploy/observability/alerts/prometheus-rules-pitr.yaml"
		missing=1
	fi
done
[ "$missing" -eq 0 ] || fail "a rule reads a metric this operator does not publish"

# And the names a rule reads that this installation is not expected to publish. They are named
# rather than omitted, because an unexplained absence from the list above is indistinguishable
# from a forgotten one - which is exactly how the list drifted in the first place.
for metric in $PITR_ABSENT_METRICS; do
	if grep -q "^# TYPE $metric " <<<"$cnpg_metrics"; then
		echo "  $metric (published after all - the reason it was excused no longer holds)"
	else
		echo "  $metric: absent by design, see PITR_ABSENT_METRICS"
	fi
done

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

# 2. The gauge itself, through the mounted record and out of the metrics endpoint - the whole
#    path, not just the file. On two roles, because the claim the chart makes is that the record
#    reaches *every* role's pod: a mount that only landed in the API would leave A-20 reading a
#    series that disappears whenever the scrape happens to hit a worker.
kubectl -n "$NAMESPACE" rollout restart deployment/hubtask-api deployment/hubtask-scheduler
kubectl -n "$NAMESPACE" rollout status deployment/hubtask-api --timeout=300s
kubectl -n "$NAMESPACE" rollout status deployment/hubtask-scheduler --timeout=300s

# The *newest running* pod of each, not the first one listed. A rolling restart leaves the previous
# pod terminating for a moment, and that one started before the record ConfigMap existed - so
# picking arbitrarily is a check that passes or fails on which pod the API server lists first.
newest_pod() {
	kubectl -n "$NAMESPACE" get pod -l "app.kubernetes.io/component=$1" \
		--field-selector=status.phase=Running --sort-by=.metadata.creationTimestamp \
		-o jsonpath='{.items[-1:].metadata.name}'
}

for role in api scheduler; do
	pod="$(newest_pod "$role")"
	[ -n "$pod" ] || fail "no $role pod is running after the restart"
	# With a short retry: a ConfigMap projection into a running pod is not instantaneous. A fresh
	# pod has it at mount time, which is what the restart is for - this is the margin.
	body=""
	for _ in $(seq 1 12); do
		body="$(fetch "pod/$pod" 9090 /metrics)" || fail "the $role operations port did not answer"
		printf '%s' "$body" | grep -q "^hubtask_restore_drill_last_success_timestamp_seconds " && break
		sleep 5
	done
	printf '%s' "$body" | grep -q "^hubtask_restore_drill_last_success_timestamp_seconds " \
		|| fail "the drill passed and the gauge A-20 reads is absent on the $role"
done
echo "  the gauge reports the drill, on every role the record is mounted into"

# And the families a production dashboard and the burn alerts are built on
# (observability-reliability.md §4), each asked of the role that owns it. The split is the chart
# README's: the queue lives in the scheduler and the worker, not in the API, which is why the
# ServiceMonitor selects a service spanning every role rather than the API's.
#
# With patience, because these appear at different moments. The RED families are a counter and a
# histogram and have no series until something is served - hence the request below. The queue depth
# is published on the scheduler's first tick after it takes the leader lock, which is seconds after
# a restart. A check without patience here fails on the clock rather than on the wiring.
assert_family() {
	local role="$1" family="$2" pod body
	pod="$(newest_pod "$role")"
	[ -n "$pod" ] || fail "no $role pod is running"
	for _ in $(seq 1 18); do
		body="$(fetch "pod/$pod" 9090 /metrics)" || fail "the $role operations port did not answer"
		if printf '%s' "$body" | grep -q "^# TYPE $family "; then
			return 0
		fi
		sleep 5
	done
	fail "$family is not on the $role's /metrics"
}

# Any path does: a 404 is recorded under `route=unmatched`, which is itself the thing being checked.
fetch "pod/$(newest_pod api)" 8080 /there-is-no-such-path > /dev/null || true

assert_family api hubtask_http_requests_total
assert_family api hubtask_http_request_duration_seconds
assert_family api hubtask_db_pool_connections
assert_family api hubtask_build_info
assert_family scheduler hubtask_job_queue_depth
echo "  the RED, pool and queue families are each on the role that owns them"

# 3. The temporary cluster is gone. A drill that leaves one behind is a drill that fails the next
#    one on quota, and it is the property most easily lost in a refactor of the teardown.
if kubectl -n "$NAMESPACE" get cluster "$DRILL_CLUSTER" >/dev/null 2>&1; then
	fail "the temporary cluster $DRILL_CLUSTER was left behind"
fi
echo "  the temporary cluster is gone"

echo
echo "pitr: a recovery to a point between two writes kept the first and not the second"
