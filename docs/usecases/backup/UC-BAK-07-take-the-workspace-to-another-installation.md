---
id: UC-BAK-07
title: Take the whole workspace to another installation
context: backup
actors: [PE-owner, PE-operator, PE-platform, PE-selfhoster, PE-integrator]
deployments: [D1, D4, D5, D6, D7]
serves: [P-01, P-08, P-09, P-15]
state: built
tasks: [H-07, F4-02]
checked_by: [test/integration/tenant_export_test.go, core/application/service/admin/Export_test.go, test/integration/restore_foreign_test.go]
---

# Take the whole workspace to another installation

## Goal

A workspace is never locked in: everything it holds leaves as one documented archive that another
Hubtask installation can take in as a workspace of its own, and that any other tool can read.

## Story

A customer of a provider (`D6`) moves to their own server. The provider's operator exports the
workspace — whether it is active, suspended or about to be deleted — to a target; the archive is
the documented format (a manifest, JSON Lines per kind of record, the media), always complete and
never encrypted, with every credential column left out. The customer, as the operator of their new
installation, puts it on a target there and restores it as a *new workspace*. The people sign in
again and make new tokens; the tasks, comments, labels, files and the trail are all there. A private
self-hoster (`D1`) does the same as the operator of their own installation.

## How to check

1. The operator of the installation — in a single-workspace installation, its owner — can export
   any workspace, in any state, to a target, in the instance area, through the API, with
   `hubctl admin tenant export` and through MCP; nobody else can.
2. The export is one complete, unencrypted archive named as an export, including media and the
   trail, and never pruned as a backup generation.
3. The archive contains no password hash, token, session, second-factor secret, signing secret or
   feed token; those fields are absent, not empty.
4. The archive can be read without Hubtask by following the documented format.
5. On another installation, the archive can be restored as a new workspace, whose content matches
   the exported one.
6. Exporting writes `tenant.exported` in the workspace's trail.

## Where it ends

* No direct HTTP download of the export; it goes to a target.
* No export or import of credentials, sessions, live automation plumbing or the compliance
  machinery's own records (data subject cases, anchors); see
  [tenant-export.md](../../architecture/tenant-export.md) §9.
* Importing from other tools (CSV, Trello, Google Tasks, Microsoft To Do) belongs to the importer
  context.
* No billing or contract steps around a move ([NG-billing](../../vision/non-goals.md)).
