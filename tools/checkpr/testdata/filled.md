## What and why

Moves the privacy gates into the pull request check, because four tables reached main
uncatalogued.

Closes #246

## Use cases

- UC-PRV-08: check 5 — met — the data job runs PG-7

## Premises

- The task said the gate ran on drafts; it never did - corrected in ci-cd.md §3.4.

## Review

- **Reviewed by:** the reviewer subagent (model r-1), 2026-11-21
- **Findings:** found 3, fixed 2, open 1
  - the job's name in the dashboard is unchanged: the dashboard is the next task's
- **Outside reviewer:** comments 4, fixed 1, declined 3

## Does this need an ADR?

- [x] No — this implements a decision that is already recorded. Which one: ADR-0015.
- [ ] Yes, and it is in this pull request or already merged: ADR-….
- [ ] It deviates from an existing ADR

## Impact

- **Breaking change:** no
- **Security:** none
- **Data protection:** none
- **Operations:** the data job takes a few seconds longer
