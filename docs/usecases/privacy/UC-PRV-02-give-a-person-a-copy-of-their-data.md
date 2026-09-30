---
id: UC-PRV-02
title: Give a person a copy of their data
context: privacy
actors: [PE-admin, PE-owner, PE-scripter]
deployments: [D3, D4, D5, D6, D7]
serves: [P-08, P-09, P-11]
state: built
tasks: [E-10, E-11, F4-20]
checked_by: [core/application/service/privacy/Export_test.go, test/privacy/PG3_export_test.go]
---

# Give a person a copy of their data

## Goal

A person who asks what the workspace holds about them — or wants to take it elsewhere — receives it
complete, in a documented, machine-readable form they can open without Hubtask.

## Story

The administrator records a request for access (or for portability) and names the backup target
the copy is to be written to. They start it; a job collects everything about the person in this
workspace — their account, what they created and wrote, what they were assigned, their media, the
trail entries they are the actor of — and writes one archive to the target. The case closes itself
and shows where the archive lies. The administrator fetches it there and hands it to the person.

## How to check

1. A copy case cannot be started without a target; it is refused with
   `privacy.export_target_required`.
2. Starting the case writes one archive at the named target, with a name that carries the case, and
   completes the case with the archive's location shown on it.
3. The archive is in the documented export format — a manifest, JSON Lines per entity, and the
   media — and is not encrypted.
4. The archive holds the person's account, the entries and comments they created or are assigned
   to, their media and the trail entries they acted in; it holds no password hash, token, session
   or second-factor secret.
5. Access and portability produce the same archive.
6. Starting the case writes `dsr.exported` in the workspace; a plain member cannot start one.

## Where it ends

* No download in the browser and no mail to the person: the archive is at the target, and handing
  it over is the controller's act.
* No PDF or human-readable report; the format is the documented one
  ([tenant-export.md](../../architecture/tenant-export.md)).
* Content of other people that merely mentions the person is not searched for.
* The copy across every workspace of an installation is its own use case.

See [data-protection.md](../../architecture/data-protection.md) §4.
