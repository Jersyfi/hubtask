# ADR-0108 — A module switch and a board comment are change records

**Status:** accepted · **Date:** 2026-11-22 · **Decided:** 2026-11-22, by the owner

**Rule lives in:** [offline-sync.md](../architecture/offline-sync.md) §3.1 (the walk's kinds and
who receives each)

## Context

Two writes changed what other signed-in devices show and told none of them. `SetModule` stored a
switch and an audit entry: a product switched off stayed in every other device's navigation until
a reload, and its screens then failed against the module gate. A whiteboard's comments wrote audit
entries only: a comment appeared on another open client after a reload.

A record alone would not have been enough for either. The workspace's switch is filed under no
container, and the stream sends such a record only when its kind is one the walk names - nothing
else says who may read it. A board's comment is filed under its board's hub, where READ on the hub
decides whatever the kind, so the stream would have sent it; but a device that starts from
nothing, or walks again after an epoch, holds only what the walk hands it, and would hold a board
without the comments written before.

## Decision

* **`setting` and `whiteboard_comment` are regular entities of the change log and of the walk**,
  like every other: no stream-only record.
* **`setting`** is one record per place - the workspace's under no container, received by every
  member; a hub's filed under the hub, received by whoever may read it. It carries each module and
  whether it is on there, nothing else.
* **`whiteboard_comment`** is filed under its board's hub and carries what
  `ListWhiteboardComments` answers. A comment taken back is an upsert without its words.
* **The sweeps record no comment.** A board's comments leave a device with the board.
* The epoch rule for a module that narrows the replica is unchanged.

## Alternatives

* *A stream-only "re-read" signal*: a second kind of record with its own rules, and a device that
  was offline during the switch would learn of it only by chance.
* *The module states' sources and locks in the record*: they are `ListModules`' answer, for whoever
  may shape the place; a record every member receives must not carry more than the capabilities do.
* *One record per module*: a switch that brings on what a module needs would be several records
  for one decision, and a client re-reads the same lists for each.
* *A `DELETE` for a comment taken back*: the thread keeps the comment's place, so the list still
  answers it; a deletion would make a device's copy differ from the server's answer.

## Consequences

The web app's `pathsFor` names both entities, and a test holds its list to the walk's kinds, so a
kind added to the walk cannot be dropped by the client unnoticed. A workspace's switch records
each hub it wrote a value into ([ADR-0109](./ADR-0109-a-product-switched-on-at-the-workspace-is-offered-to-hubs.md))
and no other hub's - also not a hub that stores a value and whose state turned with the
workspace's. A hub's record in a device's copy is therefore the hub's state when it was written or
walked, not its state now, and no client derives a hub's modules from it: the web app re-reads the
workspace's switches, the manifest and every place's modules on any `setting` record and answers
no module read from the copy.
