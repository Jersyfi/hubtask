<!-- SPDX-License-Identifier: Apache-2.0 -->
# RB-A35 — Durable timers fail to fire

**Alert:** `HubtaskTimerFailing` · **Severity:** ticket · **Catalogue:** A-35

## The symptom

Within the last hour at least one durable timer of the named purpose (SLA, OLA, APPROVAL,
EFFECTIVENESS_CHECK, PAGE_REVIEW) failed to fire (`hubtask_timer_failures_total`). The firing pass
fires each timer under its own savepoint: the failing one is undone and put back, due again after
a delay that doubles from a minute up to six hours, and every other timer of the workspace still
fires. What the failing timer stands for - a breach recorded, an approval escalated, a review
asked for - does not happen while it keeps failing.

## Immediate action

The worker logs each failure at warn: `a timer's firing failed and was put back`, with the
purpose, the `timer_id` and the failure's code - never the subject's content. Find the timers that
failed more than once:

```sql
SELECT id, tenant_id, purpose, subject_id, name, due_at, fire_attempts
FROM durable_timer
WHERE state = 'RUNNING' AND fire_attempts > 0
ORDER BY fire_attempts DESC
LIMIT 20;
```

* **A code that names a refusal** (a conflict, a constraint) → the subject is in a state the
  handler does not expect. Read the subject (the entry, the approval, the page) and repair it, or
  stop the timer through the use case that owns it; a defect in the handler is an issue.
* **`postgres.query_failed` or `dependency_unavailable`** → the database was unwell while the pass
  ran; the timer fires on its next attempt once it recovers.

## Afterwards

A timer that fires counts nothing more; the alert clears an hour after the last failure. A timer
whose failure stays is retried every six hours, and the alert keeps the ticket open until it is
repaired.
