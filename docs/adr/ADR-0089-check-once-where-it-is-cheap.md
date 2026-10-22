# ADR-0089 — Check once, where it is cheap; the worker checks its own premises

**Status:** accepted · **Date:** 2026-10-22 · **Decided:** 2026-10-22, by the owner

**Rule lives in:** [AGENTS.md](../../AGENTS.md) (working rules, the loop, the command table),
[ci-cd.md](../architecture/ci-cd.md) §1, §3.4,
[engineering-guidelines.md](../architecture/engineering-guidelines.md) §3,
[support-matrix.md](../architecture/support-matrix.md) §5,
[docs/backlog/README.md](../backlog/README.md)

Amends [ADR-0079](./ADR-0079-a-draft-is-checked-locally.md) (`make verify-pr` before leaving draft)
and [ADR-0081](./ADR-0081-how-work-is-organised.md) (the readiness record before the code).

## Context

The use cases (from 2026-09-30) and the readiness records (from 2026-10-07) raised quality: 73 of
156 merges from 2026-10-13 to 10-22 were fixes, mostly product defects the use-case checks found.
They also made a feature slow and expensive. Measured over those ten days (`make flowstats`; the
figures by hand are on the review issue of the merge flow):

* A `feat` pull request took a median 10.2 h from first commit to merge; before the use cases,
  1.3 h. Ready → merged stayed at 0.3 h: CI and the merge were not the bottleneck.
* `make verify-pr` took a median 20.6 min (max 101 min), ran about five times per worker session
  (375 starts), and was red in about 45 % of them. It ran the gates CI runs again after *Ready*,
  one after another, under a machine-wide lock for the containers; parallel workers queued behind
  each other. Its description check came last, after twenty minutes. `selftest` alone was
  8.4 min, and failed neither locally nor in CI in those ten days.
* A third of the workers' active time went to `sleep`/`until` loops waiting on those runs; 82 % of
  the prompt-cache writes were whole contexts written again after such a wait; 63 % of the input
  tokens were read in calls with more than 400k tokens of context.
* `gate-pr` failed 161 times inside sessions, 136 of them over the Definition of Done checklist —
  ticks, not checks. `gate-docs` failed 77 times, most of them over the use-case index's copy of
  each use case's state; the index was touched by 41 of 160 merges.
* The readiness records found something real: 53 of 120 found at least one false premise. Their
  matrix and decision sections were the bulk (a median 85 of 156 lines), and the review was a
  second agent run on a plan.
* On every push to a draft, `ci-required` started a runner to report "CI not run" (729 runs).
* CI on a ready pull request: 34 jobs in parallel, a median 9.5 min. The repository is public,
  every run's billable time was 0 ms, and the plan's 20 concurrent jobs were reached for 0.7 h.
  Firefox and WebKit in the browser job failed sixteen times, by the runner far more often than by
  the client, and each failure on `main` held every pull request.

## Decision

1. **A draft runs nothing.** `ci-required` carries the draft condition like every root job; a
   draft's run starts no runner, and After CI ignores a skipped run. The draft's name for the check
   stays, so a skip never reports a `CI required`.
2. **Check once, where it is cheap.** `make check` replaces `make verify-pr`: the checks of what the
   branch changed, about a minute, no lock. CI checks the rest once, after *Ready*. The hook only
   holds `--draft` on creation and a pushed `HEAD` on *Ready*. `make gate-quick` is no longer run
   at every commit: the merge is a squash.
3. **A pull request runs what it can break; `main` runs everything.** `selftest` runs when a gate
   changes, the connectors when they change, the browser job in Chromium only — Firefox and WebKit
   are best effort. The privacy tests run once, in the data job.
4. **The worker checks its own premises.** No readiness record and no review of a plan: the worker
   checks every premise the task, the use cases' *Today* and the documents state against the code,
   corrects a false one in the same pull request and names it under *Premises*. The Definition of
   Done stays as the standard in engineering-guidelines.md §3, not as a checklist to tick.
5. **The use-case index names a use case and its file**; its state lives only in the file.
6. **Waiting is a notification, not a loop**, and a worker hands over before its context passes
   about 250k tokens.

## Consequences

* A defect `make check` misses is found by CI about ten minutes after *Ready* instead of before it;
  nothing merges red, so the risk is a red pull request, bounded by `CI required`.
* A gate that stops biting because of a product change is found on `main`, which runs `selftest` on
  every commit, not on the pull request.
* A browser defect only Firefox or WebKit shows is found by a person, not by CI.
* A false premise is no longer caught by a second agent before the code; the worker answers for it
  in the description, and the reviewer of the pull request reads it there.
* The review of the merge flow (on or after 2026-11-05) compares the next window with these figures
  by the same command. If `fix` pull requests against fresh features rise, or `main` is red more
  often, a step is taken back.

## Alternatives

* **Keep `verify-pr`, only faster** (fail fast, `selftest` filtered): smaller, but every worker
  still queues on one machine for what CI runs in parallel for nothing.
* **The record by risk class** — full for migrations, access and tenancy, light otherwise: rejected
  by the owner, because quality is to be the same everywhere, and a second class is a second rule.
* **Firefox and WebKit nightly**: rejected by the owner; Chromium on pull requests and `main` is
  enough.
