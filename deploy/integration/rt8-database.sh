#!/usr/bin/env bash
# RT-8's database on integration (observability-reliability.md §12): a second, empty database in
# the live CloudNativePG cluster, so that a rolling update can be measured from the previous
# schema without touching the environment's own database.
#
#   deploy/integration/rt8-database.sh up     # the database hubtask_rt8 and the secret for it
#   deploy/integration/rt8-database.sh down   # both gone again
#
# Run on the host (KUBECONFIG=/etc/rancher/k3s/k3s.yaml) or anywhere with the admin kubeconfig.
# The database is a CloudNativePG `Database` object owned by hubtask_migrator, so the operator makes
# and drops it: the cluster's superuser stays switched off, and a superuser enabled for a test is a
# superuser left enabled. The secret carries the two DSNs a release needs, both to hubtask_rt8 -
# the owner's for the migration and hubtask_app's for the application - built from the credentials
# the cluster and bootstrap.sh already hold, and never printed.
#
# A run then installs a release of its own against it, as RT-8's evidence records:
#   --set existingSecret=hubtask-secrets-rt8
#   --set migration.dsnSecretName=hubtask-secrets-rt8 --set migration.dsnSecretKey=db-dsn-owner
set -euo pipefail

readonly NAMESPACE="hubtask"
readonly CLUSTER="hubtask-db"
readonly DATABASE="hubtask_rt8"
readonly SECRET="hubtask-secrets-rt8"

up() {
  kubectl -n "$NAMESPACE" apply -f - <<DATABASE
apiVersion: postgresql.cnpg.io/v1
kind: Database
metadata:
  name: rt8
spec:
  cluster:
    name: ${CLUSTER}
  name: ${DATABASE}
  owner: hubtask_migrator
  ensure: present
  # Dropped when this object is deleted: the database exists for the run and nothing else.
  databaseReclaimPolicy: delete
DATABASE
  for _ in $(seq 1 30); do
    applied="$(kubectl -n "$NAMESPACE" get database.postgresql.cnpg.io rt8 -o jsonpath='{.status.applied}' 2>/dev/null || true)"
    [ "$applied" = "true" ] && break
    sleep 5
  done
  [ "${applied:-}" = "true" ] || { echo "the operator never created ${DATABASE}" >&2; exit 1; }

  # The owner's DSN is the operator's own, pointed at the second database; hubtask_app's password
  # is the one bootstrap.sh generated for the environment's database.
  owner="$(kubectl -n "$NAMESPACE" get secret "${CLUSTER}-app" -o jsonpath='{.data.uri}' | base64 -d)"
  owner="${owner%/*}/${DATABASE}"
  app="$(kubectl -n "$NAMESPACE" get secret hubtask-app-role -o jsonpath='{.data.password}' | base64 -d)"
  secret_key="$(kubectl -n "$NAMESPACE" get secret hubtask-secrets -o jsonpath='{.data.secret-key}' | base64 -d)"
  kubectl -n "$NAMESPACE" create secret generic "$SECRET" \
    --from-literal=db-dsn-owner="$owner" \
    --from-literal=db-dsn="postgres://hubtask_app:${app}@${CLUSTER}-rw:5432/${DATABASE}?sslmode=require" \
    --from-literal=secret-key="$secret_key" \
    --dry-run=client -o yaml | kubectl apply -f - >/dev/null
  echo "${DATABASE} and ${SECRET} are ready"
}

down() {
  kubectl -n "$NAMESPACE" delete database.postgresql.cnpg.io rt8 --ignore-not-found
  kubectl -n "$NAMESPACE" delete secret "$SECRET" --ignore-not-found
  echo "${DATABASE} and ${SECRET} are gone"
}

case "${1:-}" in
  up) up ;;
  down) down ;;
  *) echo "usage: $0 up|down" >&2; exit 2 ;;
esac
