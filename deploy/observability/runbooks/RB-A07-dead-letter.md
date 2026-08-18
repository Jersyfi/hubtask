<!-- SPDX-License-Identifier: BUSL-1.1 -->
# RB-A07 — A job was finally given up on

**Alert:** `HubtaskDeadLetter` · **Severity:** ticket · **Catalogue:** A-07

## The symptom

A job used up its attempts (eight by default, with exponential backoff) and moved to
`state = 'DEAD_LETTER'`. **Nothing will pick it up again.** Whatever it was going to do — deliver
events, send a reminder, run a retention pass — has not happened and will not happen without a
person.

This is a ticket rather than a page: the work is already lost time, and an hour more changes
nothing. But it does not resolve itself.

## Immediate action

```bash
psql -c "SELECT id, kind, tenant_id, attempts, last_error, finished_at, payload
         FROM job WHERE state = 'DEAD_LETTER' ORDER BY finished_at DESC LIMIT 20;"
```

`last_error` is a **code**, never a sentence — deliberately, because a message could carry user
content (rule 10). The codes and what they mean:

| Code | Meaning | Usual fix |
|---|---|---|
| `queue.handler_missing` | No handler for this kind in the running build | An old pod ran a new job kind — finish the rollout |
| `queue.handler_panicked` | A program defect | See [RB-A04](./RB-A04-panic.md); the stack trace is in the log |
| `dependency.unavailable` | Something the job needs stayed down through every retry | Fix the dependency, then requeue |
| `outbox.dispatch_without_tenant` | A malformed job row | A defect; keep the row for the issue |

## Requeueing, once the cause is fixed

```sql
UPDATE job SET state = 'PENDING', attempts = 0, run_at = now(), last_error = NULL
WHERE id = '<the job id>';
```

Safe for every job kind in this project: handlers write their effect in the same transaction that
completes the job, so a re-run either happens completely or not at all (test RT-3).

## Diagnostic queries

```promql
increase(hubtask_job_dead_letter_total[24h])
sum by (job_type, attempt_class) (increase(hubtask_job_failures_total[1h]))
```

A rising `attempt_class="retry"` without dead letters is the system working: it is retrying and
succeeding.

## Follow-up

Every dead letter deserves the question *why were eight attempts not enough?* If the answer is
"the dependency was down for an hour", raise `HUBTASK_JOB_MAX_ATTEMPTS` or `HUBTASK_JOB_RETRY_MAX`.
If it is "it would never have worked", the retry was pointless and the handler should classify that
failure as final — worth an issue.
