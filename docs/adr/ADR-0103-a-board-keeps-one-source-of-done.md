# ADR-0103 — A board maps its columns to statuses and keeps one source of done

**Status:** accepted · **Date:** 2026-11-13 · **Decided:** 2026-11-12, by the owner (the concept
[work-surface](../backlog/work-surface/README.md), [build plan](../backlog/work-surface/build-plan.md) §5)

**Rule lives in:** [domain-model.md](../architecture/domain-model.md) §3.5a (lists, boards and
workflows). Amends D-02 and §3.5 (`Bucket.isDoneBucket`).

## Context

A board's done column was stored and reported, and the server completed nothing because of it: the
web board wrote completion itself, other doors (sync, bulk, automation, the API) did not, so an
entry could stand in *Erledigt* and be open. Under a workflow the board drew one column per status
and ignored the collection's own columns.

## Decision

* Three levels per entry type, derived, never a setting: list (L0), columns (L1), workflow (L2).
* At L1 a board has exactly one done column, and **the server** keeps column and completion
  together through every door. No client writes the coupling.
* At L2 the board keeps its own columns and maps each to one or more statuses (`bucket_status`);
  unmapped statuses stand folded in *Ohne Spalte*; the column follows the status; a drop takes the
  one reachable step, offers several, or refuses none with a reason.
* What existing boards break of this is repaired once, by an administrator's idempotent job.

## Alternatives

* *Keep the done column advisory.* Rejected: the same entry would be done in one view and open in
  another.
* *A board per workflow, columns = statuses.* Rejected: a team loses its own columns the moment a
  workflow is named.

## Consequences

Every writer of a column, a completion or a status reads the board (`work.Boards`). The web board
drops its own done-column writes (build plan §6, Phase 2c).

## Amendment — 2026-11-15: no repair

The owner decided on 2026-11-15 that existing boards are not repaired: nobody but the owner uses
Hubtask yet, so the break costs nobody and the one-off job (`RepairBoards`, `POST /boards:repair`)
would be code kept for no one. The job, its endpoint and its audit event `boards.repaired` are
removed; the last bullet of the decision no longer holds.
