#!/usr/bin/env bash
# Provision the integration environment on a fresh host (deployment.md §3.1).
#
# Run as root on Ubuntu, from this directory:
#
#   scp -r deploy/integration root@<host>:/root/  &&  ssh root@<host> /root/integration/bootstrap.sh
#
# Idempotent: a second run changes nothing it has already done. It deliberately never regenerates
# a password — a fresh secret against an already-initialised database is a database nobody can log
# into, and that failure looks like a bug in the application.
#
# What it does not do is deploy Hubtask. That is CI's job (.github/workflows/deploy.yml), and it
# being CI's job is the point: an environment somebody installs by hand once is an environment
# nobody can rebuild.
set -euo pipefail

readonly K3S_VERSION="v1.36.3+k3s1"
readonly K3S_INSTALL_SHA256="ed01f89fd977bf20ac1516bbebf8370bf3ddbaa55dac8aba610956a4c78cc00b"
readonly CERT_MANAGER_VERSION="v1.21.1"
readonly CERT_MANAGER_SHA256="5f6a499b8c1857d57f560f536e0dcc830914b45c420899fe7ad0692c8624e408"

# The name CI reaches the API server by. It resolves through the environment's wildcard record, so
# it costs no DNS entry of its own, and it means the endpoint in the kubeconfig is a name we
# control rather than an address the host provider assigned.
readonly API_HOST="k8s.integration.hubtask.eu"
readonly NAMESPACE="hubtask"
readonly HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# The alert rules the repository ships, one directory up. Prometheus evaluates these files and no
# copy of them: what fires on this cluster is what `promtool test rules` proves in the gate, and a
# rule that fires here is one somebody can find (observability-reliability.md §14).
readonly ALERT_RULES="${HERE}/../observability/alerts"

log() { printf '\n=== %s\n' "$*"; }

# Downloads a file and refuses to go on unless it is the one that was reviewed. Both of these are
# scripts and manifests from other people that run with root or cluster-admin rights; pinning the
# version is not enough, because a tag can be moved (CLAUDE.md: every dependency is a supply chain
# decision). When an upgrade is wanted, the hash changes here in the same commit that changes the
# version, and the diff says a human looked.
fetch_verified() {
  local url="$1" target="$2" expected="$3" actual
  curl -sfL --max-time 120 -o "$target" "$url"
  actual="$(sha256sum "$target" | cut -d' ' -f1)"
  if [[ "$actual" != "$expected" ]]; then
    echo "refusing $url: sha256 $actual, expected $expected" >&2
    echo "if this is an intended upgrade, change the version and the hash together." >&2
    exit 1
  fi
}

# ---------------------------------------------------------------------------------------------
log "k3s ${K3S_VERSION}"

if ! command -v k3s >/dev/null 2>&1; then
  fetch_verified "https://get.k3s.io" /tmp/k3s-install.sh "$K3S_INSTALL_SHA256"
  # --tls-san: the certificate has to be valid for the name CI dials, not only for the node's own
  #   address, or every kubectl call from outside fails verification.
  # --secrets-encryption: the release history and hubtask-secrets live in this datastore, on a
  #   disk in somebody else's building.
  # --disable-cloud-controller / --disable=local-storage are deliberately NOT passed: local-path
  #   is what the database and the media claim bind through.
  INSTALL_K3S_VERSION="$K3S_VERSION" INSTALL_K3S_EXEC="server \
    --tls-san ${API_HOST} \
    --secrets-encryption \
    --write-kubeconfig-mode 0600" sh /tmp/k3s-install.sh
else
  echo "k3s already installed: $(k3s --version | head -1)"
fi

export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
kubectl wait --for=condition=Ready node --all --timeout=300s

# ---------------------------------------------------------------------------------------------
log "cert-manager ${CERT_MANAGER_VERSION}"

if ! kubectl get deployment cert-manager -n cert-manager >/dev/null 2>&1; then
  fetch_verified \
    "https://github.com/cert-manager/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml" \
    /tmp/cert-manager.yaml "$CERT_MANAGER_SHA256"
  kubectl apply -f /tmp/cert-manager.yaml
fi
kubectl -n cert-manager rollout status deployment/cert-manager --timeout=300s
kubectl -n cert-manager rollout status deployment/cert-manager-webhook --timeout=300s

# The webhook is reachable a moment after its deployment reports ready, and a ClusterIssuer applied
# in that moment fails validation. Retrying is the documented way round it.
log "certificate issuers"
for attempt in 1 2 3 4 5 6; do
  if kubectl apply -f "${HERE}/issuer.yaml"; then break; fi
  echo "the webhook is not answering yet, attempt ${attempt}"
  sleep 10
done

# ---------------------------------------------------------------------------------------------
log "namespace, secrets and volumes"

kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

# Passwords are hex, not base64: they travel inside a DSN, and a `/` or `+` in a URL is a parsing
# question nobody should have to think about (deployment.md §6.1).
if ! kubectl -n "$NAMESPACE" get secret hubtask-db >/dev/null 2>&1; then
  kubectl -n "$NAMESPACE" create secret generic hubtask-db \
    --from-literal=owner-password="$(openssl rand -hex 24)"
fi
if ! kubectl -n "$NAMESPACE" get secret hubtask-secrets >/dev/null 2>&1; then
  owner="$(kubectl -n "$NAMESPACE" get secret hubtask-db -o jsonpath='{.data.owner-password}' | base64 -d)"
  app="$(openssl rand -hex 24)"
  # sslmode=disable: the connection never leaves the node, and a certificate the cluster would
  # have to issue to itself buys nothing here. A database on another host makes this require.
  kubectl -n "$NAMESPACE" create secret generic hubtask-secrets \
    --from-literal=db-dsn-owner="postgres://hubtask:${owner}@hubtask-db:5432/hubtask?sslmode=disable" \
    --from-literal=db-dsn="postgres://hubtask_app:${app}@hubtask-db:5432/hubtask?sslmode=disable" \
    --from-literal=db-app-password="${app}" \
    --from-literal=secret-key="$(openssl rand -base64 32)"
fi

# The chart mounts this claim and does not create it: a chart that creates storage is a chart that
# can delete it (k8s/values.yaml, storage.persistence).
kubectl -n "$NAMESPACE" apply -f - <<'PVC'
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: hubtask-media
spec:
  accessModes: ["ReadWriteOnce"]
  storageClassName: local-path
  resources:
    requests:
      storage: 10Gi
PVC

# ---------------------------------------------------------------------------------------------
log "PostgreSQL"
kubectl apply -f "${HERE}/postgres.yaml"
kubectl -n "$NAMESPACE" rollout status statefulset/hubtask-db --timeout=300s

# ---------------------------------------------------------------------------------------------
log "monitoring"

if [[ ! -d "$ALERT_RULES" ]]; then
  echo "the alert rules are not beside this script: expected ${ALERT_RULES}" >&2
  echo "copy the whole deploy/ directory to the host, not deploy/integration alone." >&2
  exit 1
fi

kubectl apply -f "${HERE}/monitoring.yaml"

# The rules are a ConfigMap built from the shipped files rather than a copy pasted into a manifest.
# Two copies of an alert rule is one copy nobody updates.
kubectl -n monitoring create configmap prometheus-rules \
  --from-file="$ALERT_RULES" --dry-run=client -o yaml | kubectl apply -f -

# A ConfigMap change alone restarts nothing, and Prometheus reads its rules at startup. Stamping
# the pod with the rules' own checksum is what makes `bootstrap.sh` after a rules change actually
# load them - and what makes running it twice with unchanged rules roll nothing.
checksum="$(cat "$ALERT_RULES"/*.yaml | sha256sum | cut -d' ' -f1)"
kubectl -n monitoring patch deployment prometheus --type=merge \
  -p "{\"spec\":{\"template\":{\"metadata\":{\"annotations\":{\"hubtask.eu/rules-checksum\":\"${checksum}\"}}}}}"

kubectl -n monitoring rollout status deployment/mailpit --timeout=180s
kubectl -n monitoring rollout status deployment/alertmanager --timeout=180s
kubectl -n monitoring rollout status deployment/prometheus --timeout=300s

# ---------------------------------------------------------------------------------------------
log "the deploy identity"
kubectl apply -f "${HERE}/deployer-rbac.yaml"

# The token controller fills the secret in a moment, not instantly.
for attempt in 1 2 3 4 5 6; do
  token="$(kubectl -n "$NAMESPACE" get secret github-deployer-token -o jsonpath='{.data.token}' 2>/dev/null || true)"
  [[ -n "$token" ]] && break
  echo "waiting for the token, attempt ${attempt}"
  sleep 5
done
[[ -n "${token:-}" ]] || { echo "the ServiceAccount token was never issued" >&2; exit 1; }

cat > /root/deploy-kubeconfig.yaml <<KUBECONFIG
apiVersion: v1
kind: Config
clusters:
  - name: integration
    cluster:
      server: https://${API_HOST}:6443
      certificate-authority-data: $(kubectl -n "$NAMESPACE" get secret github-deployer-token -o jsonpath='{.data.ca\.crt}')
contexts:
  - name: integration
    context:
      cluster: integration
      namespace: ${NAMESPACE}
      user: github-deployer
current-context: integration
users:
  - name: github-deployer
    user:
      token: $(echo "$token" | base64 -d)
KUBECONFIG
chmod 600 /root/deploy-kubeconfig.yaml

log "done"
cat <<SUMMARY
The cluster is up and the namespace is ready. Two things are left, and neither belongs in a script:

  1. Put /root/deploy-kubeconfig.yaml into the GitHub environment 'integration' as the secret
     KUBE_CONFIG. It is the credential; it is not in the repository and must not be.
  2. Let CI deploy. Nothing here installs the chart on purpose.

Monitoring is up in the namespace 'monitoring' and reachable from the node only - there is no
Ingress in front of it on purpose. From the host:

  kubectl -n monitoring port-forward svc/prometheus 9090:9090     # targets, rules, ALERTS
  kubectl -n monitoring port-forward svc/alertmanager 9093:9093   # what is routed right now
  kubectl -n monitoring port-forward svc/mailpit 8025:8025        # what was delivered

What fired and when is a query rather than an inbox: ALERTS{alertstate="firing"} in Prometheus
keeps the history, /api/v2/alerts in Alertmanager has the present, and /api/v1/messages in Mailpit
has the mail that went out.
SUMMARY
