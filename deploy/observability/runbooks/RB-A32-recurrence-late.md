<!-- SPDX-License-Identifier: Apache-2.0 -->
# RB-A32 — Recurring entries were created late

**Alert:** `HubtaskRecurrenceLate` · **Severity:** ticket · **Catalogue:** A-32

## The symptom

In the last hour at least one occurrence of a recurring entry was created more than an hour after
the moment it is for (`hubtask_recurrence_occurrence_lag_seconds`, by `mode`). Occurrences are
normally created ahead of their moment, so a late one means the work that creates them stood
still and has caught up - or is failing and retrying.

## Immediate action

* **A-30 fired or fires** → the scheduler stopped; follow RB-A30. This alert is its echo.
* **No scheduler gap** → the materialising jobs fail: look at
  `hubtask_job_failures_total{job_type=~"recurrence.*"}` and the dead letter (A-07).

## Afterwards

Nothing to repair: the occurrences exist now. A person who planned around one may have seen it
appear late; the entry's history shows when it was created.
