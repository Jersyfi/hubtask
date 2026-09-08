# The platform interface

**This file is the whole of what the operator side needs from this repository.** Everything else
here is explanation; this is the list.

Production is a namespace on a cluster this project does not operate, and the deploy is
[pull-based](../../docs/architecture/deployment.md#4-push-or-pull): Argo CD renders this chart at a
pinned tag against a values file that lives in the operator's own, private repository. So the
values below are named here and **set there** — none of them is committed in this repository, which
is public.

Two rules make that workable rather than merely stated:

* **A value is named, never guessed.** Where the chart needs something only the platform knows, it
  is a values key with an empty default and the chart refuses to render without it. An empty key is
  a question, and a wrong default would be an answer nobody checked.
* **A credential is a Secret's *name*.** This repository never carries a credential, and it never
  creates a Secret either. The owner creates them in the namespace; the chart references them.

---

## 1. Secrets the owner creates

Created once, by the owner, in the namespace. The chart reads them by the name it is given.

| Values key | What the Secret holds | Keys inside it |
|---|---|---|
| `existingSecret` | The application's own secrets | `db-dsn` (the application role), `secret-key`, and `smtp-password` when SMTP is configured |
| `migration.dsnSecretName` | The owner role's DSN, for the migration | the key named in `migration.dsnSecretKey` |
| `database.appRole.passwordSecret` | The **application role**'s password, read by the database operator to create the role. Type `kubernetes.io/basic-auth`, and the password must be the same one `db-dsn` above carries — two names for one credential | `username`, `password` |
| `storage.existingSecret` | The media bucket's credentials | `access-key`, `secret-key` |
| `database.backup.existingSecret` | The **backup** bucket's credentials | `access-key`, `secret-key` |
| `restoreDrill.evidence.existingSecret` | The evidence location's credentials | `access-key`, `secret-key` |
| `ingress.tls.secretName` | The platform's wildcard certificate | as the platform issues it |

**Where CloudNativePG generates the DSN itself,** `migration.dsnSecretName` is the operator's own
`<cluster>-app` Secret and `migration.dsnSecretKey` is `uri`, so the owner's credential is never
copied anywhere by hand.

**Why the application's role needs a Secret of its own.** The first migration creates both database
roles and catches the refusal a *managed* PostgreSQL answers with — CloudNativePG is one, and its
owner may not `CREATE ROLE`. The migration says so and carries on, and every grant after it would
then land on a role that does not exist. So the database resource declares `hubtask_app` itself
(`managed.roles`, no `SUPERUSER`, no `BYPASSRLS`), and the owner of the database is
`hubtask_migrator` — the role migration 0001 was written for, which is what makes its
`ALTER DEFAULT PRIVILEGES` apply to the role that actually owns the objects.

**The backup credential has a condition, and it is the whole of B-3**
([ADR-0046](../../docs/adr/ADR-0046-production-on-a-platform-namespace.md)): the identity that
writes backups must be able to write them and must **not** be able to delete them or shorten their
retention. A lock a compromised writer can lift protects against accidents only, which is not what
the threat is.

## 2. Values only the platform knows

Every one of these is empty in the chart and set in the operator's values file.

| Values key | What it is |
|---|---|
| `config.baseUrl`, `ingress.host`, `config.extraEnv.HUBTASK_CORS_ALLOWED_ORIGINS` | The public host name. The origin must be exactly what a browser sends, scheme included |
| `ingress.className` | The cluster's ingress class |
| `ingress.tls.enabled`, `ingress.tls.secretName` | The platform-issued wildcard. **The chart creates no `Certificate`** and needs no issuer annotation |
| `serviceMonitor.labels` | What the Prometheus Operator's `serviceMonitorSelector` matches on |
| `prometheusRules.labels` | What its `ruleSelector` matches on |
| `dashboards.labels` | The Grafana sidecar's label, e.g. `grafana_dashboard: "1"` |
| `dashboards.annotations` | The folder annotation the sidecar files dashboards under |
| `storage.bucket`, `storage.endpoint`, `storage.region` | The media bucket |
| `database.backup.destinationPath`, `.endpointURL` | The backup bucket and its endpoint. `destinationPath` is `s3://<bucket>/<path>` |
| `database.backup.serverName` | The archive's name inside that path. Changing it starts a new archive |
| `database.storage.storageClass`, `database.storage.size` | The storage class and the size the quota admits |
| `restoreDrill.evidence.bucket`, `.endpoint`, `.prefix`, `.region` | Where the drill's measured RPO and RTO are kept. Opaque to this repository |
| `image.pullPolicy`, `imagePullSecrets` | Only if the image is private |
| `config.extraEnv.HUBTASK_SMTP_HOST` / `_PORT` / `_USER` | The relay. The sender address is decided and lives in the reference values |
| `roles.*.replicas`, `roles.*.resources`, `database.resources` | Sized against the namespace's quota, which is the platform's number |

## 3. Two things the operator has to decide before the first sync

**The `AppProject`'s kinds have to admit what the chart renders.** Beyond the kinds already agreed
(core, apps, batch, `networking.k8s.io`, `policy`, `autoscaling`, `postgresql.cnpg.io`,
`monitoring.coreos.com`), the restore drill needs two namespaced RBAC objects — a `Role` and a
`RoleBinding` in `rbac.authorization.k8s.io` — because it creates and deletes a CloudNativePG
`Cluster` from inside the namespace and writes one ConfigMap. Either:

* admit those two namespaced kinds, and leave `restoreDrill.rbac.create` at `true`; or
* create the two objects once by hand, and set `restoreDrill.rbac.create: false`. The
  ServiceAccount's name is then `restoreDrill.serviceAccount.name`, and the rules it needs are
  exactly: `postgresql.cnpg.io/clusters` — `create, get, list, watch, delete`; `configmaps` —
  `get, create, update`.

Nothing here is cluster-scoped either way.

**Whether the quota sets `limits.cpu`.** If it does, every pod must carry a CPU limit or the
namespace refuses it — and this chart deliberately sets none, because a CPU limit on a
latency-sensitive path buys throttling rather than safety. Either the quota leaves `limits.cpu`
out, or a `LimitRange` supplies a default. It is worth settling before the first rollout rather
than during it.

## 4. What the quota has to hold

**Two databases, not one.** There is no second namespace to restore into, so the restore drill
bootstraps a temporary CloudNativePG cluster beside the live one and removes it again
([backup-restore.md §8.5](../../docs/architecture/backup-restore.md#85-the-operator-procedure-point-in-time-recovery)).
While a drill runs, the namespace holds two clusters of `database.storage.size` each, plus the
drill's own small pod. A quota that fits one exactly is a quota in which the drill fails — and the
drill failing is the alert saying this installation cannot prove it can recover.

## 5. The order of the first sync

1. The owner creates the Secrets of §1.
2. The Argo CD Application is pointed at this chart's tag and the operator's values file.
3. The first sync creates the CloudNativePG `Cluster` in sync wave −10, the migration runs as a
   `Sync` hook in wave −5, the workloads follow in wave 0, and the restore drill runs as a
   `PostSync` hook once everything is up.

   **This ordering is Argo CD's, and it is the reason a first sync works in one step.** Helm has no
   equivalent: a `pre-install` hook runs before *every* resource of the release, including the
   database, so a first install driven by `helm install` is two steps — create the database with
   `migration.enabled=false`, wait for it, then `helm upgrade` with the migration on. `helm
   rollback` and `helm uninstall` are likewise not the operator's path here; the Application's tag
   is.
4. That first drill has nothing to restore until the first base backup exists. The
   `ScheduledBackup` takes one immediately on creation, and the drill waits for it
   (`HUBTASK_DRILL_BACKUP_WAIT`, 15 minutes by default) before it gives up.

A drill that fails does **not** fail the release. The record keeps the previous success, A-20 keeps
counting, and `HubtaskRestoreDrillNeverRan` becomes a ticket after a day — because a release whose
recovery proof failed is something to look at, not a release that should not have happened.

## 6. What this repository will never contain

No host name, bucket, endpoint, quota number, label value, Secret name, IP address or credential of
the production environment. No measured RPO or RTO, and no drill evidence: those are recorded
internally, and a threshold or a figure in a public repository is that figure published (decision 7
of [milestone 0.6.0](../../docs/backlog/milestone-0.6.0.md)).

No kubeconfig, token or cluster endpoint reaches GitHub Actions, and no workflow deploys to
production. If anything in this repository ever appears to need one, that is the finding — not the
missing credential.
