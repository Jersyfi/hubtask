# ADR-0046 — Production runs in a platform namespace, and the platform's operator does PITR

**Status:** accepted · **Date:** 2026-09-04 · **Amended:** 2026-09-07 ([below](#amendment-2026-09-07-the-platforms-facts-and-what-they-change)) ·
**Closes:** open points D-1 and D-2 (`deployment.md` §8), B-2 and B-3 (`backup-restore.md` §12)

## Context

[§3.1](../architecture/deployment.md#31-where-integration-runs) settled where `integration` runs and
said, in the same breath, what it deliberately does not answer: one node rehearses no node failure
and its database shares a disk with the workload, and **neither may be carried into production
unexamined**. That is what D-1's remaining half was for, and D-2 — own container, operator, or
managed service — is the question it was coupled to, because the answer decides who does
point-in-time recovery. B-2 has held since `0.4.5`: whether Hubtask orchestrates system backups at
all. B-3 rides along: whether object lock is recommended or required.

The four were always the owner's, and the answer arrived as an offer rather than as a purchase: a
Kubernetes cluster somebody else operates, with a written division of responsibility. That changes
the shape of the question. It is no longer "which server do we buy" but "what does this project own
inside a namespace, and what must it never assume".

## Decision

**D-1 — `production` is a namespace on a platform-operated Kubernetes cluster**, reached by a tag
with the manual approval [§7](../architecture/deployment.md#7-what-happens-during-a-release) already
requires. The deploy identity is a namespace-bound ServiceAccount, exactly as `integration`'s is;
nothing this project deploys is cluster-scoped, and one hostname is public.

**D-2 — PostgreSQL through the CloudNativePG operator the platform provides.** The operator is
theirs; the `Cluster` resource is ours, in our namespace, with its backup stanza pointed at the
object storage the platform provides. So the database is neither a container we hand-roll nor a
service whose recovery we cannot reach: **PITR is ours to configure and theirs to host.**

**B-2 — Hubtask does not orchestrate system backups.** Continuous WAL archiving is the CNPG
`Cluster`'s, and the platform's volume snapshots are a second net for what is not a database. The
`INSTANCE` restore scope stays refused, and its refusal now points at the operator procedure
instead of at an open question. What Hubtask keeps is what it already had: tenant-scoped archive
backups, which are a different promise to a different party.

**B-3 — object lock is required for the system backup target**, recommended for a tenant's own
targets, and it comes with two conditions without which it is theatre:

* **The credential that writes backups must not be able to delete them or shorten their
  retention.** A lock a compromised writer can lift is a lock against accidents only.
* **The lock retention equals P-5's 35 days.** Longer, and the backup plan's own cleanup fails
  against the lock; shorter, and the promise in `data-protection.md` §12 is not kept by the storage
  that has to keep it.

## Options

1. **A namespace on a platform-operated cluster (chosen).** No hardware to buy, an operator and a
   backup target provided, and a written contract about what is ours. The cost is that the cluster,
   the alert routing and the bucket policy belong to somebody else.
2. **A second own node with k3s**, the shape `integration` has. It was the recommendation before the
   offer existed, and it is still the fallback: it costs a monthly bill and a bootstrap, and it
   answers no question the platform does not answer.
3. **Managed Kubernetes plus a managed database.** The most expensive, and the one that would make
   `integration`'s rehearsals prove least — a production shaped unlike the environment that
   rehearses for it is a production nothing rehearses for.
4. **No production until `1.0.0`.** Refused: the milestone's own definition of done asks for a
   drilled point-in-time recovery, and a recovery drilled against an environment holding nothing
   proves nothing. Real data is what makes RPO and RTO honest.

## What is deliberately not assumed

The contract is explicit, and so is this decision: **no cluster-admin, no second namespace, no
public exposure beyond the one hostname, and the platform runs neither our migrations nor our
application-level restores.** Two things follow that are easy to get wrong later:

* **The restore drill restores into our own namespace**, as a second CNPG cluster bootstrapped from
  the object store — not into a fresh namespace, because we cannot create one. The resource quota
  is therefore what bounds how large the live database may grow: it has to hold the original and the
  temporary cluster at once.
* **The restore runbook must be executable by a human alone.** Not by this project's automation, and
  not by the platform's — the platform does not do app-level restores, and a runbook whose only
  operator is an AI session is a runbook with a single point of failure that cannot be paged.

## Consequences

* `deployment.md` §3.2 describes the environment; the values live in `deploy/production/` and a
  deploy is a diff rather than a hand-edit on a cluster.
* RPO comes from WAL archiving, not from the daily volume snapshot. A drill that measured the
  snapshot would report a figure an order of magnitude worse than the one the system can achieve,
  and A-12's PITR half watches the archive's age for the same reason.
* Several facts this decision depends on are the platform's to state — the Prometheus selector
  labels, the resource quota, the bucket names and their lock retention, the secret and image-pull
  secret names. They are named unknowns in `deploy/production/README.md` rather than guesses, and
  they are corrected from `PLATFORM-CONTRACT.md` when it lands.
* The second half of H-10 — the CNPG backup stanza, A-12's PITR half, and RT-9 as a per-release
  drill — waits for the namespace to exist. That is a scheduling fact, not an open decision.

## Amendment 2026-09-07: the platform's facts, and what they change

The first half (#370) was written against an offer. Between it and the second half the platform
stated its facts, and four of them change the shape of what was decided without changing any of
the four decisions. They are recorded here rather than in a new ADR because D-1, D-2, B-2 and B-3
stand exactly as decided; what moves is how each is reached.

**The facts.** Production is a namespace on a single private k3s cluster, run from a separate,
*private* operator repository; this repository is public. The deploy is **pull-based**: Argo CD in
the cluster renders this chart at a pinned tag with a values file that lives in the operator's
repository, and a tag bump on that side *is* the deploy. There is no CI-to-cluster path and there
will be none — no kubeconfig, no token, no endpoint is ever available to GitHub Actions. The Argo
`AppProject` admits our namespace and a fixed list of kinds, nothing cluster-scoped. The
CloudNativePG operator and a Prometheus Operator are provided; TLS is a platform-issued wildcard
secret; the application's one hostname is reachable through the VPN only. Object storage exists
with Object Lock, its retention at most the backup retention window. Measured RPO and RTO and the
drill's evidence are recorded internally (decision 7 of the 0.6.0 backlog).

### What changes against #370

1. **D-1's deploy path is pull, not push.** #370 wrote "reached by a tag with the manual approval,
   the deploy identity a namespace-bound ServiceAccount, its kubeconfig a GitHub secret". The
   approval stays — it is what lets an image and a chart be *published* under a version — but no
   workflow deploys anything, and there is no kubeconfig anywhere in GitHub. The operator bumps the
   tag in its Application; the cluster pulls. [ADR-0023](./ADR-0023-deployment-strategy.md)'s push
   path is what `integration` does and keeps doing; open point D-3 is answered for production by the
   platform rather than by us. It cost nothing because the chart was already what GitOps needs: a
   versioned OCI artefact with the values per environment in their own file.

   The consequence is the one that shapes this half: **anything that must run against production
   runs in-cluster, from the chart.** The migration is a hook in a sync wave after the database;
   the restore drill is a `PostSync` hook of every release and a `CronJob` between releases.

2. **"One hostname is public" becomes "one hostname, and it is not written here".** The
   repository is public and the environment is private, so nothing production-specific is
   committed: no hostname, bucket, endpoint, quota, label value, secret name or credential. Where
   the chart needs such a value it is a values key marked *set by the operator*, and the whole set
   is listed in [`deploy/production/PLATFORM-INTERFACE.md`](../../deploy/production/PLATFORM-INTERFACE.md)
   — keys and Secret *names* only — which is the one artefact the operator side reads from here.
   The hostname #370 committed is removed, and so is the cert-manager annotation: the chart creates
   no `Certificate`, the Ingress names the platform's TLS secret from values.

3. **The drill runs where the data is, and its numbers stay there.** RT-9 is
   `hubtask-restore-drill`, a program in the image: two marker rows with a recorded moment between
   them, a temporary CNPG cluster bootstrapped from the object store to that moment, the first
   marker present and the second absent, T-20's consistency and isolation checks against the
   restored instance, RPO as the archive lag measured at drill time and RTO as the time from
   creating the temporary cluster to its readiness, and a teardown that runs even on failure. Its
   evidence goes to a location the operator names in values and this repository treats as opaque.
   What the repository keeps is the mechanism and a redacted pass/fail line — never a measured
   number. The backlog's "record the evidence under `docs/evidence/`" is amended for RT-9 to
   exactly that: `docs/evidence/` records the proof on kind, which measures a CI runner and proves
   the path, not the target.

4. **What emits `hubtask_restore_drill_last_success_timestamp_seconds`.** Nothing did. Decided:
   the drill writes its result to a ConfigMap in the namespace; the chart mounts that ConfigMap
   into every role's pod; the process reads one file and reports the gauge through the metrics
   endpoint the `ServiceMonitor` already scrapes. No platform component, no Kubernetes client in
   the application — [ADR-0015](./ADR-0015-security-baseline.md)'s "nothing here talks to the
   Kubernetes API" holds, because a mounted file is not the API — and the same mechanism works
   under Compose, where a script writes the file. Rejected: a sidecar exporter (a second image, or
   a second entry point, for one number); Pushgateway (a platform component we may not assume, and
   one that turns a push into a series that never goes stale); kube-state-metrics on the Job's
   completion time (a platform component we may not assume, and a job that completed is not a
   drill that passed); an API endpoint (a use case, an authorisation decision and a contract
   change for one gauge).

### What is deliberately not assumed, continued

* **That the `AppProject`'s kind list is complete.** The drill creates and deletes a CNPG
  `Cluster` from inside the cluster, so it needs a `Role` and a `RoleBinding` — namespaced, and in
  `rbac.authorization.k8s.io`, which the list does not name. The chart ships both behind
  `restoreDrill.rbac.create`; the interface note says the operator either admits the two namespaced
  kinds or creates them once by hand. Which of the two is the operator's call, and the drill does
  not run until it is made.
* **That Argo CD knows what a healthy CNPG `Cluster` looks like.** A sync wave waits for health
  only where a health check exists, so the migration hook waits for the database itself
  (`migration.connectWait`) rather than trusting the wave to have waited for it.
* **That the archive retention and the lock are two numbers.** The platform states the lock is at
  most the backup retention; P-5 states the retention is at most 35 days. The chart documents the
  inequality on the key (`database.backup.retentionPolicy` must stay ≥ the bucket's Object Lock)
  and the decided shape is both at 35 days, as B-3 said.

### Consequences of the amendment

* `deployment.md` §3, §4, §7 and §8 say pull for production and push for integration, and
  `deploy/production/` holds a public-safe values file, the README and the interface note instead
  of a values file with a hostname in it.
* The CNPG `Cluster` is a chart template rather than a manifest applied by hand, so the first sync
  needs the database before the migration: the `Cluster` sits in an earlier sync wave, and a
  Helm-driven first install is two steps (`backup-restore.md` §8.5, `PLATFORM-INTERFACE.md`).
* A-12's PITR half reads the operator's own metrics (`cnpg_collector_*`) and ships as a fourth rule
  file, loaded only where CloudNativePG runs; A-20 reads the gauge the drill now feeds.
* The issue stays open until the first drill has run against production: the backlog carries the
  list of what only production can close.
