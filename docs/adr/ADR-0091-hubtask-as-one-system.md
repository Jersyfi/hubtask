# ADR-0091 — Hubtask as one system

**Status:** accepted · **Date:** 2026-10-23 · **Decided:** 2026-10-23, by the owner ("Bringe das
Konzept gebündelt auf main mit einem PR")

**Rule lives in:** [design-system.md](../design/design-system.md) §11.2, §11.3, §11.6, §11.8,
§11.9, §11.10 and §13

## Context

Milestone UX moved every control to its new place and proved each use-case check with a test. It
was cut by control and by area, so no task owned the experience as a whole: how one act feels from
every place it starts, whether two screens follow the same pattern, whether a page reads well at
every width. The tests proved that a control exists and works, never that the layout, the
wrapping, the alignment or the path is right. The owner's walk of a fresh installation on
2026-10-23 found the result: three ways to add an entry, a capture field that does not help, menus
that mix the entry, the collection and the hub, a *Structure* sheet that feels foreign, a board
whose drag loses cards, a detail pane the person cannot shape, a *Today* without life, and controls
of one bar in two sizes.

## Decision

The product is held to one pattern per act, one place per object and one layout grammar for every
screen (design-system.md §13, G1–G8), and the grammar is measured by walks, not left to judgement.
Concretely: capture is the one way to add an entry and every add control opens it pointed at where
it stands; a hub's and a collection's head acts on the place and a toolbar on its entries, each menu
on one object, with the place also managed from its own row in the navigation; settings are pages;
the person chooses whether an entry opens beside the list or whole, and the navigation's and the
panel's widths are the person's to drag and the device's to keep; the bar is one size of control.

## Alternatives

* **Fix the findings one by one.** Each fix would be right and the next screen would still be built
  alone; the walk found patterns, not defects.
* **A visual review per pull request only.** Kept as well (AGENTS.md, "Working with the owner"), but
  a review sees one screen; the grammar's walks see every screen at every width on every change.

## Consequences

Some walks that measured the old drawing change with it (the bar's centred field is now the field
and *Capture*, the avatar is alone at every width). A screen added later gets its layout walk with
it.
