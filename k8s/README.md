# Helm Chart

The chart is published as an OCI artefact with every release:

```bash
helm install hubtask oci://ghcr.io/jersyfi/charts/hubtask --version 0.1.0 \
  --values my-values.yaml
```

## What it installs

| Object | Per | Note |
|---|---|---|
| `Deployment` | role | `api`, `worker`, `scheduler`, `automation` from one image (ADR-0014) |
| `Service` | api | the interactive port only; the operations port is not routable from outside |
| `Service` (headless) | release | every role, for scraping — the queue metrics live in the worker |
| `Job` | release | the migration, as a `pre-install,pre-upgrade` hook |
| `ConfigMap` | release | every `HUBTASK_*` that is not a secret; a change rolls the pods |
| `ServiceAccount` | release | without an automounted token — nothing here talks to the Kubernetes API |
| `PodDisruptionBudget` | role | only for roles with more than one replica |
| `HorizontalPodAutoscaler` | role | when `roles.<role>.autoscaling.enabled` |
| `Ingress` | release | when `ingress.enabled`; needs a host |
| `NetworkPolicy` | release | on by default |
| `ServiceMonitor` | release | when `serviceMonitor.enabled` (Prometheus operator) |
| `Cluster` (CloudNativePG) | release | when `database.enabled`; the chart owns the database, with WAL archiving and a daily base backup to object storage (ADR-0046). Kept on uninstall and never pruned |
| `ScheduledBackup` | release | with the database: the daily base backup, and one at creation |
| `Service` + `ServiceMonitor` | database | with the database and `serviceMonitor.enabled`: the instance's metrics port, which A-12's PITR half reads |
| `Job` | release | when `restoreDrill.enabled`: RT-9 as a `post-install,post-upgrade` hook (`PostSync` under Argo CD) — a point-in-time restore between two writes, checked and torn down |
| `CronJob` | release | with `restoreDrill.schedule`: the same drill between releases |
| `ServiceAccount`, `Role`, `RoleBinding` | release | for the drill alone — the one pod that talks to the Kubernetes API; `restoreDrill.rbac.create` off where the deploy identity may not create RBAC |
| `ClusterPolicy` (Kyverno) | cluster | when `imagePolicy.enabled`: the signature at the door (CI-3) — see below |

## The one thing that is mandatory

```bash
kubectl create secret generic hubtask-secrets \
  --from-literal=db-dsn='postgres://user:password@host:5432/hubtask?sslmode=require' \
  --from-literal=secret-key="$(openssl rand -base64 32)"

helm install hubtask oci://ghcr.io/jersyfi/charts/hubtask \
  --set existingSecret=hubtask-secrets
```

The chart refuses to render without `existingSecret`. That is deliberate: a secret passed as a
value ends up in the release history, in whatever repository the values file lives in, and in the
output of `helm get values`.

`db-dsn` should carry the **application role** — `hubtask_app`, which the migration creates
without `SUPERUSER` or `BYPASSRLS`, so that row level security is the last boundary
(multi-tenancy.md §2.1). Give the migration an owner DSN under a second key and name it in
`migration.dsnSecretKey`; the migrator can also grant `hubtask_app` its login itself when it is
handed `HUBTASK_DB_APP_PASSWORD`.

## Principles that must not be watered down in the templates

* `/healthz` is the liveness probe and checks **no** dependencies — otherwise a database outage
  takes down every pod at once (ADR-0016). `/readyz` is readiness and does check them.
* `terminationGracePeriodSeconds` ≥ the longest job timeout, and the process waits for its
  background loops before it exits — a grace period that only covers requests is not one.
* No secrets in `values.yaml` — only references to existing secrets.
* The operations port (9090) carries the metrics and the health report. It is reachable from
  inside the cluster for scraping and is never routed by the ingress.
* Egress excludes link-local even when an operator names no allowlist: `169.254.169.254` is where
  a cloud provider keeps credentials.

## Checking a rolling update (RT-8)

`helm lint` and the Compose smoke test are automated (`make gate-chart`, `make gate-compose`); a
rolling update under load is not, because it needs a cluster. It stays a manual check before a
release, and observability-reliability.md §12 keeps it in the nightly column rather than the
per-pull-request one.

The load generator is [`test/resilience/rt8`](../test/resilience/rt8/main.go). It writes, reads
back what it wrote, and looks for all of it again at the end, because "no 5xx" and "no data loss"
are two questions and only the first one can be answered by watching.

```bash
# 1. The load, for longer than the rollout will take. It needs a collection to write into and
#    several tokens, each of which carries its own rate budget (deployment.md §6.1) - one
#    credential would spend its budget in seconds and be refused for the rest of the run.
HUBTASK_TOKEN="<token>,<token>,..." go run ./test/resilience/rt8 \
  --url https://api.integration.hubtask.eu --collection <uuid> \
  --duration 6m --workers 8 --rate 40 --out rt8.json &

# 2. The rollout, while that runs.
helm upgrade hubtask oci://ghcr.io/jersyfi/charts/hubtask --version <new> --reuse-values
kubectl rollout status deployment/hubtask-api --timeout=10m

# 3. What it must show afterwards: no 5xx, and no restart that was not a rollout.
kubectl exec deploy/hubtask-api -- wget -qO- localhost:9090/metrics \
  | grep 'hubtask_http_requests_total{.*status_class="5xx"'
kubectl get pods -l app.kubernetes.io/instance=hubtask
```

The run is only worth anything if the two versions differ in their schema. A rollout between two
builds of the same migration state exercises the readiness gate and the grace period and says
nothing at all about expand/contract — which is the property the whole thing exists to check.

Results live in [`docs/evidence/`](../docs/evidence/), dated, one file per run.

The three things that make it pass are in the chart already: `maxUnavailable: 0` with a readiness
gate, a `PodDisruptionBudget`, and a grace period longer than the longest job. What it proves is
that the migration of that release really was expand/contract-safe (deployment.md §5).

## The signature at the door (CI-3)

`imagePolicy.enabled` renders a Kyverno `ClusterPolicy` that refuses any pod in the release's
namespace (or the `imagePolicy.namespaces` named) whose image under `ghcr.io/jersyfi/hubtask` was
not signed under one of the identities in `imagePolicy.subjects`: keyless, so what is checked is
GitHub's OIDC issuer and the workflow that signed — the release workflow for published versions,
the integration deploy for the per-commit image an environment runs from `main`. Both are the
defaults, and a fork that signs under its own repository replaces them.

What it needs: **Kyverno installed** on the cluster, and **cluster scope** — a `ClusterPolicy` is
not a namespaced object, so installing it is the cluster owner's decision rather than a side
effect of installing this chart, which is why it is off by default. What it refuses when it is
on: an unsigned image, an image signed under any other identity, and — with
`failurePolicy: Fail` — every pod while the admission webhook or the transparency log is out of
reach. That last one is deliberate: a policy that lets a pod through when it cannot check is a
policy that protects against nothing on the day it matters. Set `imagePolicy.webhookTimeoutSeconds`
with the log's reach in mind.

The same verification runs in `.github/workflows/deploy.yml` before `helm upgrade`, with `cosign
verify` under the deploy workflow's identity, so the integration environment is protected whether
or not its cluster runs Kyverno.

## Checking a change

```bash
make gate-chart
```

`helm lint` plus two renders — the defaults, and everything optional switched on. The chart
declares `kubeVersion: >=1.28.0-0`, so a manual `helm template` needs `--kube-version 1.30.0` or
it renders against helm's much older default and refuses.
