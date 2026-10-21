# ADR-0088 — A long press opens the entry menu

**Status:** accepted · **Date:** 2026-10-21 · **Decided:** 2026-10-21, by the owner (#1685)

**Rule lives in:** [design-system.md](../design/design-system.md) §11.9

## Context

[ADR-0063](./ADR-0063-navigation-and-the-working-surface.md) decisions 8 and 13 gave a long press on
a coarse pointer to selection. Milestone UX (ADR-0087) gives every entry one menu, reached by right
click, by its ⋯ and by a long press ([UC-WRK-28](../usecases/work/UC-WRK-28-act-on-an-entry-the-same-way-everywhere.md)
check 1), while its check 4 still let a long press select. One gesture cannot do both.

## Decision

A still long press (the coarse pointer's 300 ms hold, without movement) opens the entry menu when the
finger lifts; the menu's first item is *Select*, which enters selection. A hold that then moves drags,
as before. This supersedes ADR-0063 decisions 8 and 13 on what a long press does; the rest of
ADR-0063 stands.

## Consequences

* On touch, selecting takes one tap more than before; every action of an entry takes one long press
  instead of finding its ⋯.
* The drag helper decides on release, not when the timer fires, so a hold can still become a drag.
