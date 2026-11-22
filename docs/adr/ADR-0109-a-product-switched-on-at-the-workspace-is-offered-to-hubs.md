# ADR-0109 — A product switched on at the workspace is offered to hubs, not switched on in them

**Status:** accepted · **Date:** 2026-11-22 · **Decided:** 2026-11-22, by the owner

**Rule lives in:** [domain-model.md](../architecture/domain-model.md) §2, "The settings catalogue"
(the modules); [UC-INS-21](../usecases/admin/UC-INS-21-switch-a-module-on.md) checks 3 and 13

## Context

A module switch is a levelled setting; a hub without a value of its own inherited the workspace's.
Switching a product on at the workspace therefore switched it on in every existing hub at once,
except hubs that had once switched it off. People shaping one hub found products they never chose.

## Decision

* **On at the workspace allows a product and is the default for hubs made afterwards.** No existing
  hub changes, except those the person chooses in the switch's question (none by default); each
  chosen hub must be one the person may shape.
* **Off at the workspace covers every hub**, as before, and deletes nothing.
* **A hub decides inside what the workspace allows.** The resolver is unchanged: at each workspace
  switch that turns what the workspace resolves to, every existing hub without a value of its own
  gets its current state written as its own, in the switch's transaction - archived and trashed
  hubs too. Switching on again gives each hub the state it had. A product brought along by another
  (`Key.Needs`) follows the same rule.
* **A hub written by a workspace's switch gets its own change record**, so the record a device
  holds of a hub is the hub's stored state ([ADR-0108](./ADR-0108-a-module-switch-and-a-board-comment-are-change-records.md)).

## Alternatives

* *A hub never inherits "on"*: no write per hub and one rule in the resolver, but a backfill over
  two releases and a new meaning for `INHERIT` at a hub.
* *A separate "default for new hubs" switch per product*: more than was decided, twice the rows.
* *Hubs keep only what they decided themselves across off and on*: a hub made while the product
  was on stores nothing and would lose it at the next off and on.

## Consequences

One row per hub and product, and one change record per hub written, at a workspace switch that
turns the product. The installation's and a plan's default still reach the hubs of a workspace
that decides nothing itself: such a change passes no switch of the workspace. Every hub that
existed at a switch shows the product as set there, with *Reset* to follow the workspace again,
although nobody set it there. A hub's configuration document changes its `modules` block without
an edit of that document. `ModuleSwitch` gains `hub_ids` and the configuration apply
`module_hub_ids`; no field is renamed or removed.
