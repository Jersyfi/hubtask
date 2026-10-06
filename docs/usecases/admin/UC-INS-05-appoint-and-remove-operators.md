---
id: UC-INS-05
title: Appoint and remove the people who run the installation
context: admin
actors: [PE-operator]
deployments: [D4, D5, D6, D7]
serves: [P-04, P-05, P-08, P-12]
state: partial
tasks: [SI-05, SI-17]
checked_by: [core/application/service/admin/Instance_test.go]
---

# Appoint and remove the people who run the installation

## Goal

An operator adds a colleague by their workspace and address, sees at a glance who runs the
installation — by name, with themselves marked — and cannot lock the installation, or themselves,
out by accident.

## Story

*Installation → Operators* lists names, addresses, workspaces and since when; "you" marks the
reader's own row. *Add an operator*: pick the workspace, type the address. On a private
installation the first added operator would end the "owner of the only workspace" rule, so adding
the first one adds the person doing it as well, and the screen says so. *Remove* asks first; removing
yourself asks twice.

## How to check

1. The list shows each operator's name, address and workspace — never a bare identifier — and
   marks the reader's own row.
2. Adding by workspace and address works in the screen and in `hubctl`; an address that matches no
   account in that workspace is refused with one sentence.
3. Adding the first entry to an empty register also adds the person adding it, and the screen says
   so beforehand.
4. Only an active account can be added.
5. Removing asks for confirmation naming the person; removing oneself says that the installation
   will be closed to you at once. The last operator cannot be removed.
6. A service account can be added from the screen by choosing it from a list — see UC-INS-06.
7. Every addition and removal is in the instance journal.

## Where it ends

* No roles among operators: every operator can do everything the installation level allows.
* Operators never see the list of a workspace's accounts; the address is typed, not picked.

## Today

* Check 1: not met — the list shows account and workspace identifiers, tracked in #1061.
* Check 3: not met — adding a colleague first ends the owner's own access on the next request, tracked in #1061.
* Check 4: not met — an invited or disabled account can be added, tracked in #1061.
* Check 5: not met — *Remove* has no confirmation, tracked in #1061.
* Check 6: not met — the screen adds operators only by address, which a service account does not have (UC-INS-06), tracked in #1061.
