# ADR-0046 — Production runs in a platform namespace, and the platform's operator does PITR

**Status:** accepted · **Date:** 2026-09-04 · **Closes:** open points D-1 and D-2
(`deployment.md` §8), B-2 and B-3 (`backup-restore.md` §12)

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
