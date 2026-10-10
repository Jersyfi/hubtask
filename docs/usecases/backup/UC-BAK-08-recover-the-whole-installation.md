---
id: UC-BAK-08
title: Recover the whole installation after losing the server
context: backup
actors: [PE-selfhoster, PE-operator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-09, P-10, P-11]
state: partial
tasks: [H-10, H-13]
checked_by: [core/application/service/backup/Apply_test.go]
---

# Recover the whole installation after losing the server

## Goal

Whoever runs the installation can bring all of it back — every workspace, every file — after the
database or the server is lost, by a procedure one person can follow alone, and has evidence before
the day it is needed that the procedure works.

## Story

On a platform cluster the operator keeps the database's continuous archive; after a loss they
follow the runbook and recover to a moment before it, and before letting traffic in they place the
legal holds placed since that moment again and re-apply the erasures completed since. A self-hoster
on Docker Compose keeps a nightly database dump and a copy of the media, and puts both back into an empty installation with the documented
commands. Either way, a drill has been run beforehand and left a dated record of what it restored
and how long it took. From inside the web app, a request to restore *the installation* is refused
with a sentence that names this procedure and who runs it.

## How to check

1. A request to restore an installation-wide archive from inside the application is refused with
   `backup.restore_instance_is_the_operators`, whose message names the operator's procedure.
2. The operator's point-in-time recovery is a documented runbook one person can follow without the
   project's automation.
3. Before traffic is admitted after a point-in-time recovery, the legal holds placed after the
   restore point are placed again, and then the erasures completed after it are re-applied; a hold
   released after the restore point stays in force until its owner releases it again.
4. A self-hoster on Compose has a documented dump-and-restore procedure for the database and the
   media together.
5. The system backups are kept 35 days and no longer.
6. A restore drill has been run against a real installation and its dated evidence is in the
   repository.

## Where it ends

* Hubtask does not restore the database it runs on; the database's own tooling does.
* No monthly or yearly generations of system backups: 35 days is the promise to data subjects.
* A workspace's own restore is *Reset the whole workspace to an earlier backup*.

See [backup-restore.md](../../architecture/backup-restore.md) §8.5–8.6 and
[data-protection.md](../../architecture/data-protection.md) §5.

## Today

* Check 3: not met — the runbook re-places no legal hold, tracked in #1239.
* Check 5: not met on Compose — the 35 days are enforced by the platform's database cluster policy and object lock only; on Compose they are the operator's own rotation of the dump.
* Check 6: not met — `docs/evidence/` holds no restore-drill record; the first real drill is blocked on the production namespace.
