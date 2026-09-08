# Blocked: the production namespace

**What this is.** [H-10](./milestone-0.6.0.md) is built and
proved as far as it can be without a production environment. This file names exactly the part that
is left, so that "the task is finished" and "the task is proved" stay two different statements —
and so that the issue staying open is a fact somebody can check rather than an oversight somebody
has to remember.

**Why it cannot be closed by working harder.** Production is a namespace that does not exist yet.
Everything below needs the real archive, the real operator and the real data; nothing below is
waiting on a decision, a design or an unwritten line of code.

**What is *not* blocked**, so that nobody re-does it: the ADR and its amendment, the CloudNativePG
`Cluster` and its `ScheduledBackup` as chart templates, the drill program and its Job, CronJob and
RBAC, the gauge A-20 has waited for since `0.4.5`, A-12's point-in-time recovery half with its
rules test, the rules and dashboards travelling with the chart, the `INSTANCE` refusal pointing at
the operator procedure, the documents, and the whole path proved end to end in CI on kind with the
CloudNativePG operator and MinIO (`make gate-pitr`) — a real archive, a real recovery to a point
between two writes, and the build going red if the wrong marker survives.

---

## What only production can close

| # | Item | What closes it | Where the evidence goes |
|---|---|---|---|
| 1 | **The first real drill.** The `PostSync` hook runs against the production cluster and passes: markers written, a temporary cluster recovered to a point between them, the checks green, the cluster torn down | One passing run after the first sync | The operator's evidence location; the pass/fail line in the namespace's logs |
| 2 | **The measured RPO.** How far the archive actually lags a write on this cluster, against the ≤ 5 minutes [observability-reliability.md §2](../architecture/observability-reliability.md#2-service-level-objectives) names | A drill's `rpo` on production data, read more than once — a single sample is an anecdote | Internal (decision 7). Never this repository |
| 3 | **The measured RTO.** How long a recovery takes with the real database's size behind it, against the ≤ 60 minutes of §2 | The same, from a drill's `rto` | Internal (decision 7) |
| 4 | **A-12 against the live metric.** The point-in-time recovery rules read the CloudNativePG operator's own series, and a rule reading a name the operator does not publish is silent rather than noisy | The names verified against a production scrape, and one of the conditions driven to fire deliberately — the `gate-selftest` discipline, against the real stack | The rules file, if a name has to change |
| 5 | **A-20 going quiet on its own.** The gauge is fed by the record ConfigMap through a mounted file; that the mount reaches every pod in the real namespace is a thing to see rather than to render | `hubtask_restore_drill_last_success_timestamp_seconds` present in a production scrape after drill 1 | Nothing to record; the alert not firing is the record |
| 6 | **The quota holds two databases.** A drill bootstraps a temporary cluster beside the live one, and whether the namespace's real quota admits both is the platform's number ([PLATFORM-INTERFACE.md §4](../../deploy/production/PLATFORM-INTERFACE.md#4-what-the-quota-has-to-hold)) | Drill 1 passing is itself the proof; a quota refusal is how it fails | The interface note, if the quota has to move |
| 7 | **The `AppProject` admits what the chart renders.** Above all the drill's two namespaced RBAC objects, or the decision to create them by hand and set `restoreDrill.rbac.create: false` ([§3](../../deploy/production/PLATFORM-INTERFACE.md#3-two-things-the-operator-has-to-decide-before-the-first-sync)) | The first sync completing without a refused resource | — |

## One migration that is already visible

**CloudNativePG deprecated the in-core Barman Cloud integration in 1.26**, in favour of the Barman
Cloud plugin, and `spec.backup.retentionPolicy` with it. Everything here uses the deprecated form,
deliberately: it still works, it is still the operator's default backup method for backward
compatibility, and the alternative means installing a plugin component into a cluster this project
does not operate — which is the platform's decision to take, not a chart's to assume.

Three things move together on the day it is taken, which is why it is one task and not three
surprises:

* the `Cluster`'s backup stanza and the `ScheduledBackup`'s method;
* `database.backup.retentionPolicy`, whose replacement lives in the plugin's own retention
  configuration;
* the two deprecated series A-12's rules read — `cnpg_collector_last_available_backup_timestamp`
  and `cnpg_collector_first_recoverability_point` — which the operator's notice says keep working
  for the in-core method and volume snapshots, and therefore stop meaning what they mean today.

The restore drill itself does not move: it names an external cluster's object store and a recovery
target, which is the shape a plugin-based archive keeps.

## Two questions that are the owner's, not the cluster's

* **Whether the quota sets `limits.cpu`.** If it does, every pod needs a CPU limit or the namespace
  refuses it, and this chart deliberately sets none — a CPU limit on a latency-sensitive path buys
  throttling rather than safety. Either the quota leaves it out or a `LimitRange` supplies a
  default. Settled before the first rollout rather than during it.
* **Whether the backup writer can be prevented from deleting.** B-3 requires it, and it is a bucket
  policy on the platform's side rather than anything this repository can assert. A lock a
  compromised writer can lift protects against accidents only.

## How this file ends

Item 1 closes the issue; items 2–7 are recorded where each says. When all seven are done this file
is deleted in the pull request that does it, and nothing replaces it — a checklist of things that
are finished is a document that misleads the next reader about what is still open.
