# Retention and Lifecycle of Business Data

Configurable retention rules for completed tasks, trash, archive, comments, attachments, jumble,
notifications and the system's own records. [data-protection.md](./data-protection.md) §5 covers
retention from the data protection angle; this is the business view. Decision:
[ADR-0020](../adr/ADR-0020-retention-policies.md).

---

## 1. Why this is not a cron job

1. **Deletion is final.** A misconfigured rule destroys people's work, so it needs a preview, a grace period, a warning, a way to object, and a log.
2. **Deletion collides with other commitments.** Legal hold, data subject requests, offline clients that still know the object ([offline-sync.md](./offline-sync.md)), and backups ([backup-restore.md](./backup-restore.md)) each impose their own requirements.
3. **Retention is tenant-specific.** What one tenant calls tidying up is a compliance violation for another.

Retention is therefore its own bounded context (`Lifecycle`) with rules as data. It is a separate
mechanism from backup retention ([backup-restore.md](./backup-restore.md) §6): neither reads the
other's tables, and they are not to be merged into one engine.

---

## 2. The rule model

```json
{
  "id": "…",
  "scope": { "kind": "COLLECTION", "id": "…" },
  "data_kind": "COMPLETED_ITEM",
  "condition": "item.completed_at != null && item.labels.exists(l, l == 'no-archive') == false",
  "retain_days": 365,
  "action": "ARCHIVE",
  "then_after_days": 730,
  "then_action": "TRASH",
  "grace_days": 14,
  "notify": { "before_days": 7, "recipients": ["ITEM_MEMBERS", "COLLECTION_ADMINS"] },
  "enabled": true,
  "justification": "Internal policy: keep cases for two years"
}
```

| Field | Meaning |
|---|---|
| `scope` | `TENANT`, `HUB`, `COLLECTION` — the narrower rule wins over the wider one |
| `data_kind` | See the catalogue in §3 |
| `condition` | An optional CEL expression on the automation engine's port and limits ([ADR-0009](../adr/ADR-0009-automation-rules-cel.md), [automation.md](./automation.md) §1.2). Its environment is only `item`, `now` and `tenant` — a retention pass has no event, actor or payload. Compiled when the rule is written, evaluated per candidate. A condition that cannot be evaluated **stops the pass** rather than defaulting either way |
| `retain_days` | The period from the kind's anchor (§3) |
| `action` | `ARCHIVE`, `TRASH`, `ANONYMIZE`, `HARD_DELETE`, `EXPORT_THEN_DELETE`, `NOTIFY_ONLY` |
| `then_after_days` / `then_action` | A second stage: completed → archive after 1 year → delete after 2 more years |
| `grace_days` | The grace period between announcement and execution |
| `notify` | Advance warning to those affected; can be switched off |

**Example:** "keep completed to-dos for at most a year, then delete them" is
`data_kind: COMPLETED_ITEM`, `retain_days: 365`, `action: HARD_DELETE` — better with an `ARCHIVE`
stage first.

**Rules about rules** (enforced when a rule is written):

* **The anchor is the kind's, not the rule's.** A rule cannot point its period at another column. A
  second stage counts from the column the first stage wrote — `ARCHIVE` leaves `archived_at`,
  `TRASH` leaves `deleted_at` — so an action that leaves no column cannot have a stage after it.
* **The lower bound is a refusal; the upper bound needs a justification.** A period below `min_days`
  is refused, not raised. Above the operator's `max_days` (no kind has one by default) the rule
  needs a `justification` and writes an audit entry.
* **A kind nothing removes is refused** rather than configured, with a code saying so (§3, column
  *Removed by*). So is an action the kind cannot take.
* **The five-per-cent switch decides when the rule is written.** A rule whose first run would affect
  more than 5% of the holdings is stored as `NOTIFY_ONLY`, with a notice whose share comes from the
  same calculation a preview uses.
* **A warning must not arrive after the act:** a rule whose `notify.before_days` exceeds its grace
  is refused.

Rules live in `retention_rule`; `retention_policy` holds the operator's bounds per data kind.

---

## 3. Catalogue of data kinds

| `data_kind` | Time anchor | Default | Removed by | Note |
|---|---|---|---|---|
| `COMPLETED_ITEM` | `completed_at` | off | The sweep | Completed tasks, work packages, activities |
| `OPEN_ITEM_STALE` | `updated_at` | off | Nothing yet — a rule is refused | Untouched open items. Only `NOTIFY_ONLY` is ever offered: deleting open work automatically is dangerous |
| `TRASH` | `deleted_at` | 30 days | The sweep | Lower bound 7 days. No marking phase (§5) |
| `ARCHIVED_ITEM` | `archived_at` | off | The sweep | The archive is permanent; deletion only by an explicit rule |
| `COMMENT` | `created_at` | off | Nothing yet — a rule is refused | Configurable separately, because comments often stay relevant longer than the case |
| `ATTACHMENT` | `created_at` | off | Nothing yet — a rule is refused | Deleting the attachment would leave the item in place |
| `JUMBLE_ENTRY` | `created_at` | 90 days | The sweep | Inbox entries **never converted**. An entry that became a work item is its provenance (`origin_jumble_id`) and is never due. No marking phase: nobody can take an entry out of the period. A tenant-wide legal hold stops the sweep |
| `NOTIFICATION` | `created_at` | 90 days | The sweep | Notification history. No marking phase |
| `AI_SUGGESTION` | `created_at` | 30 days | The sweep | What AI proposed. Short because a suggestion is about a state of an entry that moves on. Accepted and dismissed suggestions age out too — an accepted one is already the entry's history. No marking phase; a tenant-wide legal hold stops the sweep |
| `ACTIVITY_ENTRY` | `occurred_at` | Follows the item | With the item | Item history |
| `RULE_RUN` | `started_at` | 30 days | The leader, as month partitions | Automation log |
| `WEBHOOK_DELIVERY` | `created_at` | 30 days | **Nothing yet** — only with its subscription or the tenant | Delivery log. The 30 days are not enforced today |
| `OUTBOX_EVENT` | `occurred_at` | 7 days | The sweep; whole months fall as partitions | Dispatched events ([ADR-0007](../adr/ADR-0007-events-outbox-cloudevents.md)). An event nobody has consumed is never due. A polling trigger's cursor older than this period is refused, not restarted ([automation.md](./automation.md) §3.2) |
| `SESSION` | `last_seen_at` | 30 days | The sweep | No marking phase |
| `DEVICE` | `last_seen_at` | 30 days | The sweep | Synchronising devices. A device silent past the period loses its sign-in — its session is revoked before the row goes ([offline-sync.md](./offline-sync.md) §6). No marking phase |
| `SYNC_LOG` | `occurred_at` | 90 days, the offline window | The leader (change log months) and the sweep (operation log, tombstones) | On one clock, `HUBTASK_TOMBSTONE_WINDOW`. The window is also the **lower bound**: shorter, a device that was offline could recreate what was deleted or apply a half-finished push twice ([offline-sync.md](./offline-sync.md) §7). A tenant may keep the records longer, never shorter. No marking phase |
| `AUDIT` | `occurred_at` | 400 days | **Nothing yet** — no `audit_log` partition is dropped today | Pseudonymisation instead of deletion ([audit.md](./audit.md) §6) |
| `MEDIA_ORPHAN` | `created_at` | 7 days | Nothing under this kind — unreferenced media go by the reconciliation's own graces ([data-protection.md](./data-protection.md) §5) | Unreferenced objects |
| `DELETED_ACCOUNT_RESIDUE` | `deleted_at` | 30 days | Nothing yet | Residual data after account deletion |

A new data kind is added here and to `core/domain/model/lifecycle/Catalogue.go` (same order) and is
then configurable through the API with no change to the engine; what it needs besides is something
that removes it.

---

## 4. Limits and precedence

Evaluated in this order; the first that applies wins:

1. **Legal hold** on a tenant, a container or an item → no deletion, no anonymisation. A hold on a
   container reaches everything below it; a hold on an item reaches the item and what hangs off it,
   and nothing beside it. Placed and lifted through `/legal-holds`; both ends carry a reason and an
   author, and lifting is audited. A hold is never deleted — it gains an end, so an auditor can tell
   "there was never a hold" from "somebody lifted it". An entry a hold keeps back carries the rule,
   the action and `blocked_by: legal_hold`, with no date.

   **A hold on an `ACCOUNT` is refused** (`lifecycle.hold_account_scope_unavailable`). The value
   stays in the model and the schema's check constraint so no migration is needed to honour it.
   What it will cover is decided and not yet built
   ([data-protection.md §4.1](./data-protection.md#41-three-decisions-of-2026-09-30)): the person's
   contributions and the account, stopping their erasure and deletion but not their sign-in.
2. **A restriction of processing** (GDPR Art. 18) → the object is neither deleted nor changed. The
   restriction is carried by the **account** (status `RESTRICTED`, set by `RestrictProcessing`), not
   by an open case: `RESTRICTION` is a kind of data subject request, and the case closes once the
   restriction is in place ([data-protection.md](./data-protection.md) §4).
3. **Lower bounds per data kind** (`min_days`) → no accidental immediate deletion; trash is at least
   7 days.
4. **Upper bounds per data kind** (`max_days`, where the operator has set one) → exceeding it requires
   a `justification` and writes an audit entry.
5. **The minimum tombstone period** → an object may disappear for good only once every known offline
   device has had the chance to learn of the deletion ([offline-sync.md](./offline-sync.md) §7).
6. **Referential safeguards** → **a parent is kept back for any descendant that is not going in the
   same pass**, whatever its period. A shorter-lived child still present is one something holds — a
   legal hold, a `:retain`, a restriction, or a rule that has not reached it — and deleting its parent
   would remove the context of something deliberately kept. The engine observes "not going in this
   pass", not a comparison of periods; the parent goes on the pass after its last child.

---

## 5. Execution

* A job per tenant (`retention.sweep`), seeded by trashing an entry or a container and then
  rescheduling itself ([multi-tenancy.md](./multi-tenancy.md) §2.1), throttled and in batches (1,000 objects per
  transaction by default).
* **Two-phase:** phase 1 marks and notifies (`retention_pending_until`); phase 2 executes once the
  grace period has elapsed. In between, anyone with permission can take the object out by editing
  it, moving it, or `:retain`.
* **No marking phase where it would announce nothing actionable.** A kind that has a trash gets no
  marking phase: the trash is its own grace period, and a second one would delay without warning
  anybody more. The same holds for kinds nobody can take out of a period (jumble entries,
  notifications, suggestions) and for machine records (sessions, devices, sync log) — the *Note*
  column of §3 says which.
* **Preview without effect:** `POST /retention-policies/{id}:preview` returns the count and sample
  objects.
* **Completeness:** a hard delete covers every storage location in the data catalogue (row, media,
  search index, vectors, derived counters). A test checks for orphans after a deletion run.
* **Log:** one `retention_run` per run with the scope, duration, result and the rule; a summary in
  the audit, never every object.
* **Partitioned kinds:** a month of `activity_entry`, `outbox_event`, `rule_run` or `change_log`
  whose every row has aged out for every tenant is dropped as one partition by the leader, with
  evidence in the instance journal; the tenant sweeps keep deleting rows inside newer months.
* **Metrics:** `hubtask_retention_pending`, `hubtask_retention_deleted_total{data_kind}`,
  `hubtask_retention_run_duration_seconds`, `hubtask_retention_blocked_total{reason}`.

---

## 6. Visibility for users

* An object in its grace period carries `retention: { action, effective_at, policy_id, can_retain }`
  in the API, and therefore in every client.
* `GET /retention-policies?container_id=…&effective=true` answers "which rules apply here?",
  including where each came from (inherited from the hub or the tenant). Each rule carries
  `in_force`. A rule switched off in a collection lets the wider rule through — "off here" means
  "the wider rule applies", not "nothing does".
* `EXPORT_THEN_DELETE` writes the archive to the configured backup target as an ordinary backup run
  with trigger `PRE_DELETE` — one archive per target per pass, scoped to the tenant
  ([backup-restore.md](./backup-restore.md) §3). A rule that cannot write its archive **stops**; an
  export-then-delete without the export is just a deletion.

**The advance warning.** The people the rule names are messaged through the notification path
(preference honoured, a record written whether or not it is sent, delivery a job):

* **It goes out when the entry is marked**, not `notify.before_days` later. `before_days` bounds how
  late a warning may be, which marking satisfies by construction: seven days' notice inside a
  fourteen-day grace gets fourteen.
* **An entry something holds back is not warned about.** It is not going; it carries `blocked_by`.
* **`RETENTION` is its own notification category**, so a preference set for reminders cannot
  silence the one message about work that is about to stop existing.
* **Recipients** are resolved through what knows them: the entry's member list, and for
  `COLLECTION_ADMINS` and `TENANT_ADMINS` the administrator roles of the matrix along the entry's
  whole path (a role held on the hub administers its collections). Somebody who qualifies twice is
  told once.

---

## 7. Evidence

| Test | Contents |
|---|---|
| RE-1 | A rule with `retain_days` deletes exactly the objects past the period and no others (time boundaries ±1 day, the tenant's time zone, DST) |
| RE-2 | A legal hold and a restriction of processing reliably prevent deletion |
| RE-3 | Lower bounds cannot be undercut, upper bounds enforce a `justification` |
| RE-4 | Grace period: a marked object can be taken out and is then not deleted |
| RE-5 | A hard delete leaves no orphans in media, the search index, vectors, or counters |
| RE-6 | The minimum tombstone period is observed; a device offline for 60 days does not resurrect a deleted object — SY-5 of [offline-sync.md](./offline-sync.md) §11 is the same test: the full synchronisation does not bring the deleted entry back, a push naming it is `sync.gone`, and a cursor past the window is `cursor_too_old` |
| RE-7 | The first activation of a broadly matching rule warns rather than deletes |
| RE-8 | Cross-tenant: one tenant's rule never affects another tenant's objects |
| RE-9 | A chained rule (completed → archive → deletion) passes correctly through every stage |

---

## 8. Retired

Retired — the rule-model rules are now in §2.

---

## 9. Open points

| # | Point | Needed by |
|---|---|---|
| R-4 | `retention.sweep` is seeded only by trashing an entry or a container. A workspace that never trashes anything is never swept, so its sessions, devices, notifications, jumble entries, suggestions and configured rules do not age out. Every write that creates due work (a rule, a session, a jumble entry) should seed it | Before `1.0.0` |

Closed points cited elsewhere: R-1 (the advance warning) is §6; R-2 (a parent waits for every descendant) is §4 item 6;
R-3 (what an `ACCOUNT` hold stops) is decided in
[data-protection.md](./data-protection.md) §4.1 and not yet built.
