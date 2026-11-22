# ADR-0108 — A module switch and a board comment are change records

**Status:** accepted · **Date:** 2026-11-22 · **Decided:** 2026-11-22, by the owner

**Rule lives in:** [offline-sync.md](../architecture/offline-sync.md) §3.1 (the walk's kinds and
who receives each)

## Context

Two writes changed what other signed-in devices show and told none of them. `SetModule` stored a
switch and an audit entry: a product switched off stayed in every other device's navigation until
a reload, and its screens then failed against the module gate. A whiteboard's comments wrote audit
entries only: a comment appeared on another open client after a reload. The stream withholds every
entity the walk does not name, because a client cannot place a record of a kind it never holds, so
neither could be added to the stream alone.

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
kind added to the walk cannot be dropped by the client unnoticed. A workspace's switch does not
rewrite each hub's record: a client re-reads the places' modules on any `setting` record.
