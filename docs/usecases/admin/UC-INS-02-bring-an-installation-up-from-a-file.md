---
id: UC-INS-02
title: Bring an installation up from a file, operators included
context: admin
actors: [PE-operator]
deployments: [D4, D5, D6, D7]
serves: [P-08, P-09]
state: partial
tasks: [SI-05, SI-17, SC-04]
checked_by: [core/application/service/admin/InstanceLevels_test.go]
---

# Bring an installation up from a file, operators included

## Goal

A managed service provider or a platform team describes an installation in files and environment
variables — its defaults, its locks, and who runs it — and a new installation comes up in that
state without a person at a browser.

## Story

`HUBTASK_INSTANCE_FILE` holds the defaults and locks; `seed` writes it once, `enforce` writes it at
every start and makes the level read-only. `HUBTASK_OPERATORS` names the operators as
"address@workspace". An operator named there becomes one as soon as that account exists; the
health report lists the ones still waiting.

## How to check

1. A file in `seed` mode is written once on the first start that finds the level empty, and then
   left alone.
2. A file in `enforce` mode is written at every start, and every writing door — screen, `hubctl`,
   API — refuses with the file's path in the refusal; the screen does not offer the controls.
3. The file goes through the same checks as the other doors; a value the API refuses, the file
   cannot set.
4. `HUBTASK_OPERATORS` puts the named accounts into the operator register at start, recorded in the
   instance journal; an entry whose account does not exist yet is reported in the health report and
   applied when it does.
5. Removing an address from `HUBTASK_OPERATORS` does not remove an operator added through the
   screen; the environment adds, it does not own the register.

## Where it ends

* Workspaces themselves are not described in the file: they are provisioned through the API.
* No secrets in the file; keys stay in the environment ([ADR-0045](../../adr/ADR-0045-master-key-in-the-environment.md)).

## Today

* **Checks 4 and 5 fail:** `HUBTASK_OPERATORS` was left out of SI (ADR-0070 "does differently");
  the register can only be filled through the API by somebody already in it, or by SQL.
