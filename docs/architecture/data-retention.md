# Retention and Lifecycle of Business Data

Configurable retention rules for the main features: completed tasks, trash, archive, comments,
attachments, jumble, notifications. Complements [data-protection.md](./data-protection.md) §5
(which covers retention from the data protection angle) with the business view.
Decision: [ADR-0020](../adr/ADR-0020-retention-policies.md).

---

## 1. Why this is not a cron job

The obvious approach would be a script that deletes old rows overnight. Three things argue against
it:

1. **Deletion is final.** A misconfigured rule destroys people's work. So it needs a preview, a grace period, a warning, a way to object, and a log.
2. **Deletion collides with other commitments.** Legal hold, ongoing data subject requests, offline clients that still know the object ([offline-sync.md](./offline-sync.md)), and backups ([backup-restore.md](./backup-restore.md)) each impose their own requirements.
3. **Retention is tenant-specific.** What one tenant considers tidying up is, for another, a compliance violation in one direction or the other.

Retention is therefore its own bounded context (`Lifecycle`) with rules as data, not as code.

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
| `condition` | An optional CEL expression using the same expression language as automation ([ADR-0009](../adr/ADR-0009-automation-rules-cel.md)) — not a second filtering mechanism |
| `retain_days` | The period from the relevant point in time (`completed_at`, `deleted_at`, `archived_at`, `created_at` — defined per data kind) |
| `action` | `ARCHIVE`, `TRASH`, `ANONYMIZE`, `HARD_DELETE`, `EXPORT_THEN_DELETE`, `NOTIFY_ONLY` |
| `then_after_days` / `then_action` | Multi-stage chains: completed → archive after 1 year → delete after 2 more years |
| `grace_days` | The grace period between announcement and execution |
| `notify` | Advance warning to those affected; can be switched off |

**The example from the requirements:** "keep completed to-dos for at most a year, then delete them"
is `data_kind: COMPLETED_ITEM`, `retain_days: 365`, `action: HARD_DELETE` — or, preferably, with an
intermediate `ARCHIVE` stage and a later `HARD_DELETE`, because completed work has a habit of
turning out to be relevant after all.

---

## 3. Catalogue of data kinds

| `data_kind` | Time anchor | Default | Note |
|---|---|---|---|
| `COMPLETED_ITEM` | `completed_at` | off | Completed tasks/work packages/activities |
| `OPEN_ITEM_STALE` | `updated_at` | off | Untouched open items — only `NOTIFY_ONLY` as a default; automatically deleting open work is dangerous |
| `TRASH` | `deleted_at` | 30 days | Trash (F-09), lower bound 7 days |
| `ARCHIVED_ITEM` | `archived_at` | off | The archive is permanent per F-10; deletion only on an explicit rule |
| `COMMENT` | `created_at` | off | Configurable separately, because comments often stay relevant longer than the case |
| `ATTACHMENT` | `created_at` | off | Storage cost; deleting the attachment leaves the item in place |
| `JUMBLE_ENTRY` | `created_at` | 90 days | Inbox entries never converted |
| `NOTIFICATION` | `created_at` | 90 days | Notification history |
| `ACTIVITY_ENTRY` | `occurred_at` | Follows the item | Item history |
| `RULE_RUN` | `started_at` | 30 days | Automation log |
| `WEBHOOK_DELIVERY` | `created_at` | 30 days | Delivery log |
| `SESSION` | `last_seen_at` | 30 days | |
| `AUDIT` | `occurred_at` | 400 days | Special case: pseudonymisation instead of deletion ([audit.md](./audit.md) §6) |
| `MEDIA_ORPHAN` | `created_at` | 7 days | Unreferenced objects |
| `DELETED_ACCOUNT_RESIDUE` | `deleted_at` | 30 days | Residual data after account deletion |

New data kinds are added here and are then immediately configurable through the API — with no code
change to the engine.

---

## 4. Limits and precedence

Evaluated in this order; the first matching rule wins:

1. **Legal hold** on a tenant, container, or item → no deletion, no anonymisation. Lifting it is auditable.
2. **An ongoing data subject request** with status `RESTRICTION` (GDPR Art. 18) → processing is restricted, and the object is neither deleted nor changed.
3. **Lower bounds per data kind** (`min_days`) → prevent accidental immediate deletion; trash, for example, is at least 7 days.
4. **Upper bounds per data kind** (`max_days`) → prevent effectively unlimited storage where the operator has set a maximum period; exceeding it requires a `justification` and produces an audit entry.
5. **The minimum tombstone period** → an object may only disappear for good once every known offline device has had the chance to learn about the deletion ([offline-sync.md](./offline-sync.md) §7). Otherwise it comes back on the next sync.
6. **Referential safeguards** → a work package is not deleted while activities hang off it that have their own, longer period; the chain is worked from the bottom up.

---

## 5. Execution

* Runs in the `scheduler`/`worker` as a job per tenant, throttled and in batches (1,000 objects per transaction by default), so that large deletion runs do not disrupt operation.
* **Two-phase:** phase 1 marks and notifies (`retention_pending_until`), phase 2 executes once the grace period has elapsed. In between, anyone with permission can take the object out by editing it, moving it, or issuing a `:retain` command.
* **Preview without effect:** `POST /retention-policies/{id}:preview` returns the count and sample objects before a rule is activated. A new rule always starts in `NOTIFY_ONLY` mode if its first run would affect more than 5% of the holdings — with a clear notice rather than a silent mass deletion.
* **Completeness:** a hard delete covers every storage location from the data catalogue (row, media, search index, vectors, derived counters). A test checks for orphans after every deletion run.
* **Log:** one `retention_run` per run with the scope, duration, result, and a reference to the rule; a summary in the audit (not every individual object — otherwise the audit grows faster than the payload data).
* **Metrics:** `hubtask_retention_pending`, `hubtask_retention_deleted_total{data_kind}`, `hubtask_retention_run_duration_seconds`, `hubtask_retention_blocked_total{reason}`.

---

## 6. Visibility for users

Retention nobody can see will eventually surprise somebody. So:

* An object in its grace period carries a machine-readable field `retention: { action, effective_at, policy_id, can_retain }` and appears in the API, and therefore in every client.
* Those affected get an advance warning (7 days by default) through the chosen channel.
* `GET /retention-policies:effective?container_id=…` answers the question "which rules actually apply here?", including where each came from (inherited from the hub or the tenant).
* An automatic export before deletion is possible (`EXPORT_THEN_DELETE`) — the archive lands at the configured backup target.

---

## 7. Evidence

| Test | Contents |
|---|---|
| RE-1 | A rule with `retain_days` deletes exactly the objects past the period and no others (time boundaries ±1 day, the tenant's time zone, DST) |
| RE-2 | Legal hold and `RESTRICTION` reliably prevent deletion |
| RE-3 | Lower bounds cannot be undercut, upper bounds enforce a `justification` |
| RE-4 | Grace period: a marked object can be taken out and is then not deleted |
| RE-5 | A hard delete leaves no orphans in media, the search index, vectors, or counters |
| RE-6 | The minimum tombstone period is observed; a device offline for 60 days does not resurrect a deleted object |
| RE-7 | The first activation of a broadly matching rule warns rather than deletes |
| RE-8 | Cross-tenant: one tenant's rule never affects another tenant's objects |
| RE-9 | A chained rule (completed → archive → deletion) passes correctly through every stage |
