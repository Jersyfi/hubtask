---
id: UC-AUD-07
title: Keep the trail for its period, and no longer
context: audit
actors: [PE-owner, PE-auditor, PE-operator]
deployments: [D3, D4, D5, D6, D7]
serves: [P-03, P-06, P-07, P-11]
state: partial
tasks: [H-06, H-13]
checked_by: [test/integration/audit_read_test.go]
---

# Keep the trail for its period, and no longer

## Goal

The trail is evidence: nobody can remove a single entry from it, and it is still kept only as long
as the evidence is needed — 400 days by default — after which it goes whole, and the going is
itself recorded.

## Story

An administrator who would like one embarrassing entry gone finds there is no way to remove it — not
in the web app, not through the API, not for the application at the database. After 400 days the
oldest month of the trail is removed as one piece, and an entry says how many entries of which
period went. A person who was erased is not removed from the trail either; everything that leaves
the trail names them by a pseudonym. When a workspace itself is deleted for good, its trail goes
with it, and the installation's own journal keeps the evidence that it happened.

## How to check

1. No door offers to edit or remove a single trail entry, and the application's database role can
   neither update nor delete one.
2. An erasure of a person leaves their entries in place; every read and export names them by a
   pseudonym, and the chain still verifies.
3. Entries older than the workspace's audit period (400 days by default) are removed by whole
   periods, never one by one.
4. Each removal writes an entry that names the period removed and the number of entries.
5. The audit period is shown where the other retention periods are, and a change to it is itself an
   audited action.
6. The hard deletion of a workspace removes its trail and leaves an entry in the installation's
   journal with the number of trail entries that went, and no content.

## Where it ends

* No selective deletion of entries for any reason, including a data subject request — the entry
  about the erasure is the one entry that has to survive it.
* No archiving of old entries to cold storage inside Hubtask; somebody who needs them beyond the
  period exports them first (*Hand a period of the trail to somebody outside*).

## Today

* **Checks 3, 4 and 5 fail.** The audit kind carries its 400-day default but no action: "nothing in
  this build removes an audit entry" (`core/domain/model/lifecycle/Catalogue.go`, `KindAudit`), and
  configuring it is refused. The partition duty only creates and repairs partitions
  (`infrastructure/postgres/AuditTrailRepository.go`); none is ever dropped. The trail therefore
  grows without end, which contradicts [audit.md](../../architecture/audit.md) §3 and the default
  in [data-protection.md](../../architecture/data-protection.md) §5.
