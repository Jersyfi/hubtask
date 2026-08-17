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

## Checking a change

```bash
make gate-chart
```

`helm lint` plus two renders — the defaults, and everything optional switched on. The chart
declares `kubeVersion: >=1.28.0-0`, so a manual `helm template` needs `--kube-version 1.30.0` or
it renders against helm's much older default and refuses.
