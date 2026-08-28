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
| `condition` | An optional CEL expression using the same expression language as automation ([ADR-0009](../adr/ADR-0009-automation-rules-cel.md)) — not a second filtering mechanism, and literally the same port and the same limits ([automation.md](./automation.md) §1.2). Its environment is smaller: `item`, `now` and `tenant`, because a retention pass has no event, no actor and no payload — the clock caused it, and declaring names nothing can fill would let somebody write a condition that is never true. Compiled when the rule is written and evaluated per candidate; the entry is read only for a rule that has one. A condition that cannot be evaluated stops the pass rather than defaulting either way — defaulting to true would delete what the condition was written to protect, and defaulting to false would quietly retain everything and look like a working rule |
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
| `OUTBOX_EVENT` | `occurred_at` | 7 days | Dispatched events (ADR-0007). A row nobody has consumed yet is never due, whatever the period says. It is also the window a polling trigger may reach back into: a cursor older than this period is refused rather than restarted, because the events it names are gone (automation.md §3.2) |
| `SESSION` | `last_seen_at` | 30 days | |
| `AUDIT` | `occurred_at` | 400 days | Special case: pseudonymisation instead of deletion ([audit.md](./audit.md) §6) |
| `MEDIA_ORPHAN` | `created_at` | 7 days | Unreferenced objects |
| `DELETED_ACCOUNT_RESIDUE` | `deleted_at` | 30 days | Residual data after account deletion |

New data kinds are added here and are then immediately configurable through the API — with no code
change to the engine.

---

## 4. Limits and precedence

Evaluated in this order; the first matching rule wins:

1. **Legal hold** on a tenant, container, or item → no deletion, no anonymisation. Lifting it is auditable. Placed and lifted through `/legal-holds` since E-08, and both ends carry a reason and an author: a hold is never deleted, it gains an end, because an auditor has to be able to tell "there was never a hold" from "somebody lifted it". An entry a hold is keeping back says so - it carries the rule, the action, and `blocked_by: legal_hold`, with no date, because it is not waiting for a moment, it is being overruled.

   **The `ACCOUNT` scope is refused.** The schema's check constraint accepts it and `Holds.Blocking` deliberately ignores it: a hold on an account is about one person's own data, which is erased where a data subject request is answered rather than kept where a workspace's entries are, and E-10 is the task that answers one. Storing one meanwhile would store a hold nothing honours, which is worse than none - somebody believes it is in force. The value stays in the model and in the constraint, so E-10 needs no migration to start honouring it (E-08).
2. **A restriction of processing** (GDPR Art. 18) → processing is restricted, and the object is neither deleted nor changed.

   The wording here said "a data subject request with **status** `RESTRICTION`" until E-10, and an
   implementation following it would have looked for a value that cannot occur: `RESTRICTION` is a
   *kind* of request in the schema (`data_subject_request.kind`), and the statuses are `RECEIVED`,
   `IN_PROGRESS`, `COMPLETED` and `REJECTED`. What carries the restriction is the **account**, whose
   status becomes `RESTRICTED` when `RestrictProcessing` is called - a technical state rather than
   an open case, because the case is closed once the restriction is in place and the restriction
   goes on standing (`data-protection.md` §4, `identity.AccountStatus.ProcessingAllowed`).
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
* `GET /retention-policies?container_id=…&effective=true` answers the question "which rules actually apply here?", including where each came from (inherited from the hub or the tenant). This document named a `:effective` sub-resource and `openapi.yaml` implemented the same question as query parameters; the two said the same thing, and E-07 made the specification's wording the wording. Each rule in the answer carries `in_force`, so a caller can see both what exists and which of it applies - and a rule switched off in a collection lets the wider one through rather than stopping it, because "off here" means "the wider rule applies" rather than "nothing does".
* An automatic export before deletion is possible (`EXPORT_THEN_DELETE`) — the archive lands at the configured backup target, written as an ordinary backup run under the trigger `PRE_DELETE`. One archive per target per pass: the archive format's scope is a tenant or a container ([backup-restore.md](./backup-restore.md) §3), so forty entries going in one pass are one export of the tenant they were in. A rule that cannot write its archive **stops** rather than proceeding - "export then delete" with the export missing is just a deletion.

**The advance warning is not sent yet.** The first bullet of this section is built and the second is
not: an object in its grace period carries `retention` and every client can see it, and nobody is
messaged about it. Sending one needs a notification category the schema does not have, a template the
message catalogue does not have, and a way to resolve "the collection's administrators" that nothing
asks for yet - the notification context's work rather than the retention engine's. So a rule that
asks to warn somebody is **refused** rather than stored, which is the standard
`lifecycle.history_not_wired` already sets: a configuration nothing enforces looks like a working
installation until the day somebody is waiting for it. Open point R-1.

---

## 7. Evidence

| Test | Contents |
|---|---|
| RE-1 | A rule with `retain_days` deletes exactly the objects past the period and no others (time boundaries ±1 day, the tenant's time zone, DST) |
| RE-2 | A legal hold and a restriction of processing reliably prevent deletion |
| RE-3 | Lower bounds cannot be undercut, upper bounds enforce a `justification` |
| RE-4 | Grace period: a marked object can be taken out and is then not deleted |
| RE-5 | A hard delete leaves no orphans in media, the search index, vectors, or counters |
| RE-6 | The minimum tombstone period is observed; a device offline for 60 days does not resurrect a deleted object |
| RE-7 | The first activation of a broadly matching rule warns rather than deletes |
| RE-8 | Cross-tenant: one tenant's rule never affects another tenant's objects |
| RE-9 | A chained rule (completed → archive → deletion) passes correctly through every stage |

---

## 8. What E-07 decided

Five things the rule model needed settled, recorded here so that nobody re-derives them:

* **The rule is a table beside `retention_policy`, not on top of it.** That table's key allows one
  period per kind per tenant, and the model is scoped — two rows for one kind is the whole point.
  Dropping its primary key in one release would break a statement an old pod is still running, so
  the old table keeps working for the length of a rolling update, its rows are carried into the new
  one by the first sweep after the upgrade, and a later release contracts it away.
* **Anchors are the kind's, not the rule's.** "A year after what" is not something a tenant chooses:
  a rule that could point the period at another column could keep completed work for a year after it
  was *created* and delete it while it was still open. A chain's second stage counts from the column
  the first stage wrote — `ARCHIVE` leaves `archived_at`, `TRASH` leaves `deleted_at` — and an action
  that leaves no column cannot have a stage after it.
* **The lower bound is a refusal and the upper bound is a justification.** The old path *raised* a
  period below the floor, which was right for a row nobody had reviewed; a period a tenant
  configures is a decision they are making, and answering it with a number they never asked for is
  worse than telling them the one they may not go below. The upper bound is the operator's
  (`retention_policy.max_days`), because §4.4 says "where the operator has set a maximum period" —
  no kind carries one by default.
* **The five-per-cent switch decides when the rule is written, not at its first run.** The rule that
  gets stored is the safe one, so an installation whose engine never runs still cannot have a broad
  rule waiting to fire — and the share it reports comes from the same calculation a preview uses, so
  the notice is checkable.
* **A kind nothing sweeps is refused rather than configured.** Every kind §3 names is in the
  catalogue, and the ones this build can actually remove are marked; a period configured against
  nothing would look like a working installation until somebody checked. The same standard held the
  CEL condition back until an engine could evaluate one — accepted since `0.5.0` (G-06), compiled
  when the rule is written and evaluated per candidate — and still holds back the advance warning
  (§6).

---

## 9. Open points

| # | Point | Needed by |
|---|---|---|
| R-1 | The advance warning of §6: a notification category, a template, and the resolution of `COLLECTION_ADMINS` and `TENANT_ADMINS`. Until it exists a rule that asks to warn somebody is refused | `0.5.0` |
| R-2 | Whether the referential safeguard of §4.6 should keep a parent back for a *shorter*-lived child as well, or only for a longer-lived one. Today anything below an entry that is not going in the same pass keeps it back, which is the conservative reading | `0.5.0` |
| R-3 | What an `ACCOUNT` legal hold should stop. §4.1 names the tenant, the container and the item; the schema also allows an account, and until E-10 answers one it is refused rather than stored (E-08) | `0.4.5`, with E-10 |
