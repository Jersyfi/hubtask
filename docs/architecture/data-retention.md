# Retention and Lifecycle of Business Data

Configurable retention rules for completed tasks, trash, archive, comments, attachments, jumble,
notifications and the system's own records — the business view; [data-protection.md](./data-protection.md)
§5 is the data protection view. Decision: [ADR-0020](../adr/ADR-0020-retention-policies.md).

---

## 1. Why this is not a cron job

1. **Deletion is final**, so it needs a preview, a grace period, a warning, a way to object, and a log.
2. **Deletion collides with other commitments:** legal hold, data subject requests, offline clients that still know the object ([offline-sync.md](./offline-sync.md)), backups ([backup-restore.md](./backup-restore.md)).
3. **Retention is tenant-specific.** One tenant's tidying up is another's compliance violation.

Hence its own bounded context (`Lifecycle`) with rules as data, separate from
backup retention ([backup-restore.md](./backup-restore.md) §6): neither reads the other's tables,
and they are not to be merged into one engine.

---

## 2. The rule model

```json
{
  "scope": { "kind": "COLLECTION", "id": "…" },
  "data_kind": "COMPLETED_ITEM",
  "condition": "item.labels.exists(l, l == 'no-archive') == false",
  "retain_days": 365, "action": "ARCHIVE",
  "then_after_days": 730, "then_action": "TRASH",
  "grace_days": 14,
  "notify": { "before_days": 7, "recipients": ["ITEM_MEMBERS", "COLLECTION_ADMINS"] },
  "enabled": true,
  "justification": "Internal policy: keep cases for two years"
}
```

| Field | Meaning |
|---|---|
| `scope` | `TENANT`, `HUB`, `COLLECTION` — the narrower rule wins over the wider one |
| `data_kind` | See §3 |
| `condition` | Optional CEL on the automation engine's port and limits ([ADR-0009](../adr/ADR-0009-automation-rules-cel.md), [automation.md](./automation.md) §1.2), over only `item`, `now` and `tenant`. Compiled on write, evaluated per candidate; one that cannot be evaluated **stops the pass** rather than defaulting either way |
| `retain_days` | The period from the kind's anchor (§3) |
| `action` | `ARCHIVE`, `TRASH`, `ANONYMIZE`, `HARD_DELETE`, `EXPORT_THEN_DELETE`, `NOTIFY_ONLY` |
| `then_after_days` / `then_action` | A second stage (completed → archive after 1 year → delete after 2 more) |
| `grace_days` | Between announcement and execution |
| `notify` | Advance warning to those affected; can be switched off |

**Rules about rules** (enforced when a rule is written):

* **The anchor is the kind's, not the rule's.** A second stage counts from the column the first
  stage wrote — `ARCHIVE` leaves `archived_at`, `TRASH` leaves `deleted_at` — so an action that
  leaves no column cannot have a stage after it.
* **Bounds (§4 items 3–4):** a period below `min_days` is refused, not raised; no kind has a
  `max_days` by default.
* **A kind nothing removes is refused**, with a code saying so (§3, *Removed by*), and so is an
  action the kind cannot take.
* **The five-per-cent switch decides when the rule is written.** A rule whose first run would affect
  more than 5% of the holdings is stored as `NOTIFY_ONLY`, with a notice whose share comes from the
  same calculation a preview uses.
* **A warning must not arrive after the act:** a rule whose `notify.before_days` exceeds its grace
  is refused.

Rules live in `retention_rule`; `retention_policy` holds the operator's bounds per data kind.

---

## 3. Catalogue of data kinds

The code's copy, in the same order, is `core/domain/model/lifecycle/Catalogue.go`; a new kind goes
into both, needs no engine change, and needs something that removes it.

| `data_kind` | Time anchor | Default | Removed by | Note |
|---|---|---|---|---|
| `COMPLETED_ITEM` | `completed_at` | off | The sweep | Tasks, work packages, activities |
| `OPEN_ITEM_STALE` | `updated_at` | off | Nothing — a rule is refused | Only `NOTIFY_ONLY` is ever offered: deleting open work automatically is dangerous |
| `TRASH` | `deleted_at` | 30 days | The sweep | Lower bound 7 days. No marking phase |
| `ARCHIVED_ITEM` | `archived_at` | off | The sweep | Permanent; deletion only by an explicit rule |
| `COMMENT` | `created_at` | off | Nothing — a rule is refused | Comments often outlive the case's relevance |
| `ATTACHMENT` | `created_at` | off | Nothing — a rule is refused | Deleting the attachment would leave the item in place |
| `JUMBLE_ENTRY` | `created_at` | 90 days | The sweep | Entries **never converted**; one that became a work item is its provenance (`origin_jumble_id`) and never due. No marking phase. A tenant-wide legal hold stops the sweep |
| `NOTIFICATION` | `created_at` | 90 days | The sweep | No marking phase |
| `AI_SUGGESTION` | `created_at` | 30 days | The sweep | Accepted and dismissed suggestions age out too. No marking phase; a tenant-wide legal hold stops the sweep |
| `ACTIVITY_ENTRY` | `occurred_at` | Follows the item | With the item | Item history |
| `RULE_RUN` | `started_at` | 30 days | The leader, as month partitions | Automation log |
| `WEBHOOK_DELIVERY` | `created_at` | 30 days | **Nothing yet** — only with its subscription or the tenant | The 30 days are not enforced today |
| `OUTBOX_EVENT` | `occurred_at` | 7 days | The sweep; whole months fall as partitions | ([ADR-0007](../adr/ADR-0007-events-outbox-cloudevents.md)). An event nobody has consumed is never due. A polling trigger's cursor older than this is refused, not restarted ([automation.md](./automation.md) §3.2) |
| `SESSION` | `last_seen_at` | 30 days | The sweep | No marking phase |
| `DEVICE` | `last_seen_at` | 30 days | The sweep | A silent device's session is revoked before its row goes ([offline-sync.md](./offline-sync.md) §6). No marking phase |
| `SYNC_LOG` | `occurred_at` | 90 days, the offline window | The leader (change log months) and the sweep (operation log, tombstones) | One clock, `HUBTASK_TOMBSTONE_WINDOW`, which is also the **lower bound** ([offline-sync.md](./offline-sync.md) §7). No marking phase |
| `AUDIT` | `occurred_at` | 400 days | **Nothing yet** — no `audit_log` partition is dropped today | Pseudonymisation instead of deletion ([audit.md](./audit.md) §6) |
| `MEDIA_ORPHAN` | `created_at` | 7 days | Nothing — the media reconciliation's own graces ([data-protection.md](./data-protection.md) §5) | Unreferenced objects |
| `DELETED_ACCOUNT_RESIDUE` | `deleted_at` | 30 days | Nothing yet | Residual data after account deletion |

---

## 4. Limits and precedence

Evaluated in this order; the first that applies wins:

1. **Legal hold** on a tenant, a container or an item → no deletion, no anonymisation. A hold on a
   container reaches everything below it; on an item, the item and what hangs off it, nothing
   beside it. Placed and lifted through `/legal-holds`, each end with a reason and an author;
   lifting is audited. A hold is never deleted — it gains an end, so an auditor can tell "never a
   hold" from "somebody lifted it". An entry it keeps back carries the rule, the action and
   `blocked_by: legal_hold`, with no date. A comment has no trash: deleted under a hold, it reads as
   deleted to everybody but keeps its text until the last hold covering it is lifted, and the sweep
   clears it then. A deleted template, which has no trash either, stays the same way until its
   last hold is lifted, and the sweep removes the row then; without a hold, deleting it removes the
   row at once. What reaches a template is a hold on the workspace or on the hub or collection it is
   defined on — it sits on no entry and names no author, so neither an entry hold nor an account
   hold does.

   **A hold also stops the workspace's own deletion and a destructive restore**: while any hold in
   the workspace is in force, a deletion request is refused, a workspace already pending deletion
   stays pending until the last hold is lifted, and `REPLACE_TENANT` is refused; a restore into a
   living workspace — a replace, a merge, a selective restore — keeps the workspace's hold records,
   never the archive's, while a restore as a new workspace takes the archive's along
   ([data-protection.md](./data-protection.md) §5, [backup-restore.md](./backup-restore.md) §8.2).
   A restore into a living workspace also leaves an account's restriction as it is (item 2).

   **A hold on an `ACCOUNT`** covers that person's account and what they contributed: an entry they
   created, commented on or attached a file to goes only with their data, so every deletion path
   reads the contributors of what it removes while such a hold stands. What a hold names must exist
   in the workspace (`lifecycle.hold_target_not_found`). A purge judges every entry it removes, and
   emptying the trash keeps back a parent or a container while anything below it stays (item 6).
   An erasure under a hold:
   [data-protection.md §4.1](./data-protection.md#41-three-decisions-of-2026-09-30).
2. **A restriction of processing** (GDPR Art. 18) → the object is neither deleted nor changed. The
   **account** carries it (status `RESTRICTED`), not an open case
   ([data-protection.md](./data-protection.md) §4).
3. **Lower bounds per data kind** (`min_days`) → no accidental immediate deletion; trash at least 7
   days.
4. **Upper bounds per data kind** (`max_days`, where the operator set one) → exceeding it requires a
   `justification` and writes an audit entry.
5. **The minimum tombstone period** → an object disappears for good only once every known offline
   device could learn of the deletion ([offline-sync.md](./offline-sync.md) §7).
6. **Referential safeguards** → **a parent is kept back for any descendant that is not going in the
   same pass**, whatever its period — a child still present is held by a legal hold, a `:retain`, a
   restriction or a rule that has not reached it, and its parent is its context. The engine observes
   "not going in this pass", not a comparison of periods; the parent goes on the pass after its last
   child.

---

## 5. Execution

* A job per tenant (`retention.sweep`), seeded by trashing an entry or a container, then
  rescheduling itself ([multi-tenancy.md](./multi-tenancy.md) §2.1); throttled, 1,000 objects per
  transaction by default.
* **Two-phase:** phase 1 marks and notifies (`retention_pending_until`); phase 2 executes after the
  grace period. In between, anyone with permission can take the object out by editing, moving, or
  `:retain`.
* **No marking phase where it would announce nothing actionable:** a kind with a trash (the trash is
  its own grace period), kinds nobody can take out of a period (jumble entries, notifications,
  suggestions) and machine records (sessions, devices, sync log) — §3 says which.
* **Preview without effect:** `POST /retention-policies/{id}:preview` returns the count and sample
  objects.
* **Completeness:** a hard delete covers every storage location in the data catalogue (row, media,
  search index, vectors, derived counters); a test checks for orphans after a run.
* **Log:** one `retention_run` per run (scope, duration, result, rule); a summary in the audit,
  never every object.
* **Partitioned kinds:** a month of `activity_entry`, `outbox_event`, `rule_run` or `change_log`
  aged out for every tenant is dropped as one partition by the leader, with evidence in the instance
  journal; tenant sweeps delete rows inside newer months.
* **Metrics:** listed in [observability-reliability.md](./observability-reliability.md).

---

## 6. Visibility for users

* An object in its grace period carries `retention: { action, effective_at, policy_id, can_retain }`
  in the API.
* `GET /retention-policies?container_id=…&effective=true` answers "which rules apply here?" and
  where each came from; each carries `in_force`. A rule switched off in a collection lets the wider
  rule through — "off here" means "the wider rule applies", not "nothing does".
* `EXPORT_THEN_DELETE` writes the archive to the configured backup target as an ordinary backup run
  with trigger `PRE_DELETE` — one archive per target per pass, scoped to the tenant
  ([backup-restore.md](./backup-restore.md) §3). A rule that cannot write its archive **stops**.

**The advance warning** goes through the notification path (preference honoured, a record written
whether or not it is sent, delivery a job):

* **It goes out when the entry is marked**, not `notify.before_days` later; `before_days` bounds how
  late a warning may be, which marking satisfies by construction.
* **An entry something holds back is not warned about.** It is not going; it carries `blocked_by`.
* **`RETENTION` is its own notification category**, so a reminder preference cannot silence it.
* **Recipients:** the entry's members, and for `COLLECTION_ADMINS` and `TENANT_ADMINS` the
  administrator roles along the entry's whole path (a hub role administers its collections).
  Somebody who qualifies twice is told once.

---

## 7. Evidence

| Test | Contents |
|---|---|
| RE-1 | A rule with `retain_days` deletes exactly the objects past the period and no others (time boundaries ±1 day, the tenant's time zone, DST) |
| RE-2 | A legal hold and a restriction of processing reliably prevent deletion |
| RE-3 | Lower bounds cannot be undercut, upper bounds enforce a `justification` |
| RE-4 | Grace period: a marked object can be taken out and is then not deleted |
| RE-5 | A hard delete leaves no orphans in media, the search index, vectors, or counters |
| RE-6 | The minimum tombstone period holds; a device offline for 60 days does not resurrect a deleted object — SY-5 of [offline-sync.md](./offline-sync.md) §11: a full sync does not bring it back, a push naming it is `sync.gone`, a cursor past the window is `cursor_too_old` |
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

Closed points cited elsewhere: R-1 (the advance warning) → §6; R-2 (a parent waits for every
descendant) → §4 item 6; R-3 (what an `ACCOUNT` hold stops) → [data-protection.md](./data-protection.md) §4.1.
