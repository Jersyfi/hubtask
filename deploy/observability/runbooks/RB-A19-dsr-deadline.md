# RB-A19 — A data subject request is approaching its deadline

**Alert:** `HubtaskDataSubjectDeadline` · **Severity:** ticket · **Metric:**
`hubtask_dsr_deadline_total{stage="approaching"|"overdue"}`

## What it means

Somebody exercised a right — access, erasure, portability, restriction, objection or rectification
— and the statutory period to answer it is running out, or has run out. The period is a legal one
rather than an internal target ([ADR-0018](../../../docs/adr/ADR-0018-privacy-by-design.md),
[data-protection.md](../../../docs/architecture/data-protection.md) §4). Thirty days is the default
this system records; the alert warns seven days ahead and keeps warning while a case stays late.

The counter is incremented by the deadline watch, which runs once a day for each workspace that has
an open case. `stage="overdue"` means a period has already passed.

## What to check

1. **What is owed.** `GET /privacy/requests?due_within_days=7` lists the cases falling due, soonest
   first, overdue ones included. `?status=RECEIVED` narrows it to the ones nobody has started.
2. **Whether anybody owns them.** A case with no `handled_by` is one nobody has picked up.
3. **Whether a case is stuck rather than forgotten.** An erasure or an export that was started
   moves to `IN_PROGRESS` and is carried out by a job; a case that has been `IN_PROGRESS` for hours
   means the job failed. Look for `privacy.request` in the dead letter (`RB-A07-dead-letter.md`).

## What to do

* **A case nobody started:** decide it. An erasure needs its mode - `ANONYMIZE` keeps the
  authorship and the workspace's content, `FULL_DELETE` takes the person's own contributions with
  them - and an access or portability case needs a backup target to write the archive to.
* **A case that cannot be answered:** refuse it, with the reason. `PATCH /privacy/requests/{id}`
  with `status: REJECTED` and a `rejection_reason`. A refusal within the period is an answer; silence
  is not.
* **A case whose job failed:** the failure is in the job row's `last_error` as a message code. Fix
  the cause - most often a backup target that is unreachable - and start the case again.
* **A period that has already passed:** answer it anyway, and record what happened. The case, its
  deadline and every step of it are in the audit trail under `dsr.*`, which is what an operator
  shows a supervisory authority.

## What not to do

* **Do not silence the alert to make it stop.** It keeps firing because the case is still open, and
  the case is still a breach in the making.
* **Do not delete the case.** A request that was recorded and then removed leaves an installation
  unable to show that it was answered at all.

## Related

* [data-protection.md](../../../docs/architecture/data-protection.md) §4 — the rights as use cases
* [audit.md](../../../docs/architecture/audit.md) §6 — why the trail pseudonymises rather than
  deleting when an erasure is carried out
