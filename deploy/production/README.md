# The production environment

A namespace on a Kubernetes cluster this project does not operate. What it is and why it is that
rather than a second node of our own is in
[deployment.md §3.2](../../docs/architecture/deployment.md#32-where-production-runs) and
[ADR-0046](../../docs/adr/ADR-0046-production-on-a-platform-namespace.md); this directory is the
shape it takes.

| | |
|---|---|
| API | `https://hubtask.prho.cloud` |
| Namespace | `hubtask` |
| Database | `postgres.yaml` — a CloudNativePG `Cluster`, ours, on the platform's operator |
| Values | `values.yaml` |
| Deployed by | the tag `v*`, through the `production` environment's manual approval |

Unlike [`../integration`](../integration/README.md) there is no `bootstrap.sh`, and that is the
point: the cluster, the ingress controller, the certificate issuer, the operator and the object
storage already exist. What we bring is a Helm release and a database resource.

## What is ours and what is not

**Ours, inside the namespace:** every application manifest and rollout, the CNPG `Cluster` and its
backup stanza, the migrations, the metrics endpoints and the alert rules, the resource requests and
limits, and secrets the owner creates and we reference by name.

**Never assumed:** cluster-admin, a second namespace, any public exposure beyond the one hostname,
or that the platform runs our migrations or our application-level restores. Cluster-scoped
resources are outside the deploy identity's RBAC by design.

## The arithmetic behind the sizing

The namespace has a quota, so the chart's defaults do not fit — they assume a cluster with room.
What `values.yaml` asks for:

| | replicas | cpu request | memory request | memory limit |
|---|---|---|---|---|
| api | 2 | 400m | 768Mi | 1536Mi |
| worker | 1 | 200m | 384Mi | 768Mi |
| scheduler | 2 | 100m | 256Mi | 512Mi |
| automation | 1 | 150m | 256Mi | 512Mi |
| database (`postgres.yaml`) | 1 | 500m | 1Gi | 2Gi |
| **total** | | **1350m** | **~2.6Gi** | **~5.3Gi** |

Against a provisional quota of 2 CPU / 4Gi requested and 4 CPU / 6Gi limited, with room left for
the migration job — which runs beside the deployments during a rollout rather than instead of them.

**Storage is the tighter one.** 20Gi holds the database's 8Gi *and* the temporary cluster a restore
drill bootstraps from the object store into this same namespace, because there is no second
namespace to restore into. So the live database cannot pass roughly 9Gi without the drill failing
first — a loud failure, and years away for one workspace, but the reason 8Gi is written down rather
than guessed.

## What the platform still owes

These are named unknowns rather than guesses. `PLATFORM_*` placeholders in the files below are
meant to fail loudly if anybody deploys before they are filled in from `PLATFORM-CONTRACT.md`.

| # | Unknown | Where it goes |
|---|---|---|
| 1 | The `serviceMonitorSelector` labels the platform's Prometheus matches on | `values.yaml`, `serviceMonitor.labels` |
| 2 | Whether `PrometheusRule` objects from this namespace are evaluated, and under what selector | the alert rules of `deploy/observability/alerts/` |
| 3 | The `cert-manager` cluster issuer's name | `values.yaml`, the ingress annotation |
| 4 | The media bucket's name, endpoint, and the secret holding its credentials | `values.yaml`, `storage.*` |
| 5 | The backup bucket's path and endpoint, its secret's name, and its Object Lock retention | `postgres.yaml`, `backup.barmanObjectStore` |
| 6 | The `imagePullSecret`'s name, if the image is private | `values.yaml` |
| 7 | The SMTP relay: host, port and user (`noreply@hubtask.eu` is decided) | `values.yaml`, `config.extraEnv` |
| 8 | The authoritative ResourceQuota | the table above |

**And one question back to the platform.** If the quota sets `limits.cpu`, every pod must carry a
CPU limit or the namespace refuses it — and this chart deliberately sets none, because a CPU limit
on a latency-sensitive path buys throttling rather than safety. Either the quota leaves
`limits.cpu` out, or a `LimitRange` supplies a default. It is worth settling before the first
rollout rather than during it.

## The one manual step

A credential does not belong in a script's output, in a repository, or in a chat window. The owner
creates the secrets and this directory only names them:

```bash
kubectl -n hubtask create secret generic hubtask-secrets \
  --from-literal=db-dsn='postgres://…' \
  --from-literal=db-dsn-owner='postgres://…' \
  --from-literal=secret-key='…' \
  --from-literal=smtp-password='…'
```

The two DSNs come from the credentials CloudNativePG generates for the cluster: the owner role runs
the migrations, the application role runs everything else, and the application must not be able to
create objects (multi-tenancy.md §2.1). The media and backup bucket credentials are two more
secrets, named in the table above.

## What is not here yet

The second half of H-10: a production deploy job, A-12's PITR half emitting, and **RT-9** — the
per-release drill that restores to a point between two writes, proves the first survived and the
second did not, and writes its evidence. It waits for the namespace to exist rather than for a
decision. Until then [#267](https://github.com/Jersyfi/hubtask/issues/267) stays open.

**The restore runbook has to be executable by a person alone.** The platform does not do app-level
restores, and a runbook whose only operator is a session cannot be paged.
