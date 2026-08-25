<!-- SPDX-License-Identifier: BUSL-1.1 -->
# RB-A08 — Reminders are arriving late

**Alert:** `HubtaskReminderDelay` · **Severity:** ticket · **Catalogue:** A-08 · **SLO:** SLO-5

## The symptom

The delay between the moment a reminder promised and the moment it fired (P95) is above 120
seconds. SLO-5 asks for 99% of reminders within 60 seconds, so this is the objective at risk
rather than broken.

Nothing is lost. A reminder that has not fired is still `PENDING` with its `fire_at` in the past,
and the pass that eventually runs fires it — late, and the delay lands in this histogram. What a
person experiences is a reminder arriving after the moment it was for, which for a deadline is
most of its value gone.

The usual cause is that **nothing is firing**: the `worker` role is not running, or the tenant's
`reminder.fire` job is stuck or was never seeded.

## Immediate action

```bash
# Is a worker running at all, and is it claiming this kind?
docker compose logs app --since 15m | grep -E "worker ready|job failed"

# What is overdue and by how much? (as the owner role - this reads across tenants)
psql -c "SELECT tenant_id, count(*), min(fire_at), now() - min(fire_at) AS behind
         FROM reminder WHERE state = 'PENDING' AND fire_at <= now() GROUP BY tenant_id;"

# Is the wake-up itself alive, or did it give up?
psql -c "SELECT tenant_id, state, attempts, last_error, run_at
         FROM job WHERE kind = 'reminder.fire';"
```

Four shapes, four answers:

* **No `reminder.fire` row for a tenant that has overdue reminders** → the wake-up was lost: the
  job finished when the tenant owed nothing and a later write failed to seed a new one. Writing
  any reminder or moving any due date in that tenant seeds one; that is also the fix, and the
  cause is worth an issue.
* **`state = 'DEAD_LETTER'`** → the pass failed repeatedly and `last_error` carries the code. Fix
  the cause, then delete the row — the next write in that tenant enqueues a fresh job.
* **`state = 'PENDING'` with `run_at` in the past** → nothing is claiming it: the `worker` role is
  not in `HUBTASK_ROLES`, or the process is not running.
* **`state = 'RUNNING'` with `locked_until` in the past** → a worker died mid-pass. The lease
  expires and another worker claims it; nothing was sent twice, because a reminder leaves
  `PENDING` exactly once and does so in the transaction that wrote the notification.

## Was the delay the firing or the sending?

They are different failures with different fixes, and two metrics tell them apart:

```promql
# How late the schedule was - this alert
histogram_quantile(0.95, sum(rate(hubtask_reminder_delivery_delay_seconds_bucket[30m])) by (le))

# How long a message then took to leave, and whether it left at all
histogram_quantile(0.95, sum(rate(hubtask_notification_send_duration_seconds_bucket{category="REMINDER"}[30m])) by (le))
increase(hubtask_notification_failures_total{category="REMINDER"}[30m])
hubtask_job_queue_depth{job_type="notification.deliver"}
```

A schedule that is on time with a mail server that is down is `notification.deliver` backing up,
not this alert. `/meta/health` says so as well: an installation with no SMTP configured and the
worker role enabled reports `config.smtp_missing_with_reminders`.

## Escalation

None. A backlog after an outage drains by itself — the pass comes straight back while its batch
keeps filling, and every missed reminder fires exactly once, late. Watch the P95 fall rather than
intervening.

## Follow-up

A delay that was not an outage means the schedule is drifting: check
`hubtask_scheduler_tick_lag_seconds` and the queue depth of `reminder.fire`. A tenant whose
wake-up was lost rather than late is a defect in the seeding, and belongs in an issue with the
`job` row that was there when it happened.
