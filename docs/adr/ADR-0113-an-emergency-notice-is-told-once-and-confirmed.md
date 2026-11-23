# ADR-0113 — An emergency notice is told once and confirmed, not drawn as a banner

**Status:** accepted · **Date:** 2026-11-23 · **Decided:** 2026-11-23, by the owner (#1858)

**Rule lives in:** [identity.md](../architecture/identity.md) §22, and
[design-system.md](../design/design-system.md) §11.6

## Context

When the owner opens a private hub through the emergency access, every member is told
([ADR-0073](./ADR-0073-private-hubs.md) §4). In the app this was a banner on every view for thirty
days. The banner pushed every view down, and a reader either stopped seeing it or saw it again on
every screen. Nothing showed that the member had read it.

## Decision

* **Told once, when the app opens**, in a dialog that says who, which hub, when, until when and
  why. The member confirms it. The confirmation is stored, is the member's own and is shown to
  nobody else.
* **Afterwards readable for thirty days** behind the notice mark in the app bar and on the hub's
  row in the navigation. The hub's own notice stays.
* **No view draws a banner for it.**

This supersedes [ADR-0073](./ADR-0073-private-hubs.md) §4 in how a member is told in the app
(the mail and the hub's list of accesses stay), and the scope of
[ADR-0065](./ADR-0065-the-second-walk-of-the-shell.md) decision 4: the emergency notices of the
last thirty days move into the bar's notice mark beside the health report.

## Consequences

* The notice reaches the member once, without taking a row of every view.
* A stored confirmation proves the member was told, which the banner could not.
* The notice mark is also drawn while the reader has an emergency notice of the last thirty days,
  quiet once every one is confirmed.
