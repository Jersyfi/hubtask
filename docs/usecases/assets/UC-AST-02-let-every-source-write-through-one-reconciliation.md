---
id: UC-AST-02
title: Let every source write through one reconciliation
context: assets
actors: [PE-admin, PE-integrator, PE-agent, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-18]
state: verified
tasks: []
checked_by: [core/domain/model/cmdb/Cmdb_test.go, core/application/service/cmdb/Cmdb_test.go, test/integration/cmdb_repository_test.go, apps/webapp/src/lib/data/cmdb.test.ts, core/application/service/cmdb/Configuration_test.go]
---

# Let every source write through one reconciliation

## Goal

People, programs, files and connectors write objects through one reconciliation: each reported item is identified before anything is written, the better source keeps what it wrote, what cannot be identified waits for a person, and every decision can be read back.

## Story

The network team's inventory tool reports the servers every night; a scanner reports the same servers with fewer details. Dana ranks the inventory tool above the scanner. The scanner's model numbers no longer overwrite the inventory tool's, and a server the scanner reports without a hostname or a serial waits in *Held* instead of becoming an object nobody can find again.

## How to check

1. A source's payload is identified by the source's own key first, then by its object type's identifier rules in order (inherited, nearest first); running the same payload again changes nothing and makes no second object.
2. An item that carries neither a source key nor every attribute of one rule is held with `cmdb.hold_insufficient_identifiers`; one from a source that may not create is held with `cmdb.hold_source_may_not_create`, and one of an object type the source may not write with `cmdb.hold_class_not_written_by_source`.
3. An attribute a source of a lower rank number last wrote is not overwritten by a higher one, until the writer has gone silent for the object type's staleness period; the run names each attribute kept and by which source.
4. A dry run answers the same decisions and writes no object, relation, run, hold or duplicate.
5. A run's explanation names, for every item, its outcome, its object, the rule that matched and the attributes taken or kept - by name, never by value - and is read back by `ReadCiRun`.
6. An administrator defines a source's kind, rank, the object types it may write and whether it may make new objects; a person writing by hand writes as the source `manual`.
7. The *Inventory* screen shows the runs, a run's decisions and the held items, and a member who may write dismisses a held item.

## Where it ends

* Connectors that pull from other systems and the signed push intake are UC-AST-07's; relations no source confirms any more do not expire yet - the following release.
