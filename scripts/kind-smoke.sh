#!/usr/bin/env bash
# SPDX-License-Identifier: BUSL-1.1
# Copyright (c) 2026 Jérôme Bastian Winkel
#
# The chart, installed into a real cluster.
#
# `helm template` renders YAML; only an API server *validates* it. A field that does not exist, a
# value of the wrong type, a probe pointing at a port the container never opens - none of those
# show up in a render, and all of them show up here. It is the cheapest way to have the chart
# meet Kubernetes before a cluster somebody depends on does.
#
# It expects a kind cluster to exist (the workflow creates one) and leaves the release behind for
# the job's logs; the cluster is thrown away with the runner.

set -euo pipefail

cd "$(dirname "$0")/.."

TAG="${1:?usage: kind-smoke.sh <image tag>}"
IMAGE="${HUBTASK_IMAGE:-ghcr.io/jersyfi/hubtask}"
RELEASE="hubtask"
NAMESPACE="hubtask-smoke"
HELM="${HELM:-.tools/helm}"

# Drawn rather than written down, for the same reason as in the Compose smoke test: a literal here
# would be a credential in the repository even though it protects nothing (SG-7).
DB_PASSWORD="$(head -c 24 /dev/urandom | base64 | tr -d '/+=')"
APP_PASSWORD="$(head -c 24 /dev/urandom | base64 | tr -d '/+=')"
SECRET_KEY="$(head -c 32 /dev/urandom | base64)"

echo "--- the image this commit produces, inside the cluster ---"
kind load docker-image "$IMAGE:$TAG" --name hubtask

kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

echo "--- a throwaway database ---"
# Not part of the chart, and deliberately so: Hubtask expects a PostgreSQL it does not manage
# (ADR-0003). This one exists for the length of the job.
kubectl -n "$NAMESPACE" create secret generic postgres-smoke \
  --from-literal=password="$DB_PASSWORD" --dry-run=client -o yaml | kubectl apply -f -
kubectl -n "$NAMESPACE" apply -f - <<MANIFEST
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgres
spec:
  replicas: 1
  selector: { matchLabels: { app: postgres } }
  template:
    metadata: { labels: { app: postgres } }
    spec:
      containers:
        - name: postgres
          image: postgres:16-alpine
          env:
            - name: POSTGRES_DB
              value: hubtask
            - name: POSTGRES_USER
              value: hubtask
            - name: POSTGRES_PASSWORD
              valueFrom: { secretKeyRef: { name: postgres-smoke, key: password } }
            - name: PGDATA
              value: /var/lib/postgresql/data/pgdata
          ports: [{ containerPort: 5432 }]
          readinessProbe:
            exec: { command: ["pg_isready", "-U", "hubtask"] }
            initialDelaySeconds: 5
            periodSeconds: 5
          volumeMounts: [{ name: data, mountPath: /var/lib/postgresql/data }]
      volumes: [{ name: data, emptyDir: {} }]
---
apiVersion: v1
kind: Service
metadata:
  name: postgres
spec:
  selector: { app: postgres }
  ports: [{ port: 5432, targetPort: 5432 }]
MANIFEST
kubectl -n "$NAMESPACE" rollout status deployment/postgres --timeout=180s

echo "--- the claim local storage needs ---"
# storage.kind=local is a supported configuration and the chart refuses to render it without a
# claim - deliberately, because media on an ephemeral disk disappears with the pod. Creating one
# here is what lets this job exercise that branch rather than only the S3 one.
kubectl -n "$NAMESPACE" apply -f - <<MANIFEST
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: media-smoke
spec:
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 1Gi
MANIFEST

echo "--- the secrets the chart refuses to render without ---"
# Two DSNs, which is the arrangement A-11 asks of Kubernetes as well: the migration runs as the
# owner, the application as hubtask_app, and the migrator grants that role its login.
kubectl -n "$NAMESPACE" create secret generic hubtask-secrets \
  --from-literal=db-dsn="postgres://hubtask_app:$APP_PASSWORD@postgres:5432/hubtask?sslmode=disable" \
  --from-literal=db-dsn-owner="postgres://hubtask:$DB_PASSWORD@postgres:5432/hubtask?sslmode=disable" \
  --from-literal=app-password="$APP_PASSWORD" \
  --from-literal=secret-key="$SECRET_KEY" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "--- install ---"
"$HELM" install "$RELEASE" k8s \
  --namespace "$NAMESPACE" \
  --set image.repository="$IMAGE" \
  --set image.tag="$TAG" \
  --set image.pullPolicy=Never \
  --set existingSecret=hubtask-secrets \
  --set migration.dsnSecretKey=db-dsn-owner \
  --set migration.appPasswordSecretKey=app-password \
  --set roles.api.replicas=1 \
  --set roles.worker.replicas=1 \
  --set roles.scheduler.replicas=1 \
  --set roles.automation.replicas=1 \
  --set config.tenancyMode=single \
  --set storage.kind=local \
  --set storage.persistence.enabled=true \
  --set storage.persistence.existingClaim=media-smoke \
  --set networkPolicy.enabled=false \
  --wait --timeout 10m || {
    echo "FAILED: the install did not become ready"
    kubectl -n "$NAMESPACE" get pods
    kubectl -n "$NAMESPACE" describe pods | tail -60
    kubectl -n "$NAMESPACE" logs -l app.kubernetes.io/instance="$RELEASE" --tail=80 --all-containers || true
    exit 1
  }

echo "--- what a rollout has to be able to do ---"
for role in api worker scheduler automation; do
	ready="$(kubectl -n "$NAMESPACE" get deployment "$RELEASE-$role" -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo 0)"
	if [ "${ready:-0}" -lt 1 ]; then
		echo "FAILED: the $role deployment has no ready replica"
		kubectl -n "$NAMESPACE" describe deployment "$RELEASE-$role" | tail -30
		exit 1
	fi
	echo "  $role: ready"
done

# The readiness probe already proves /readyz answers - that is what --wait waited for. What it
# does not prove is that the migration ran as its own job rather than as a side effect.
if ! kubectl -n "$NAMESPACE" get job "$RELEASE-migrate" >/dev/null 2>&1; then
	echo "FAILED: the migration hook left no job behind"
	exit 1
fi

echo "chart: installed into a real cluster, every role ready"
