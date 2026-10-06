---
id: UC-INS-12
title: See the installation at a glance and read what happened
context: admin
actors: [PE-operator, PE-selfhoster]
deployments: [D1, D4, D5, D6, D7]
serves: [P-01, P-11, P-12]
state: partial
tasks: [SI-17]
checked_by: [core/application/service/admin/InstanceOverview_test.go]
---

# See the installation at a glance and read what happened

## Goal

An operator sees in one screen how many workspaces and accounts there are, in which states, how
healthy the installation is, and the latest entries of the instance journal — counts and states,
never a customer's content.

## Story

*Installation → Overview*: "248 active · 3 suspended · 1 pending deletion", "4 102 accounts",
health, the sign-in defaults in one line, operators, the last journal entry. *Journal* lists what
was provisioned, suspended, changed and by whom, newest first.

## How to check

1. The overview shows counts and states that agree with each other at one instant.
2. The number of operators is right, including "you are the operator because this installation has
   one workspace" when the register is empty, with correct singular and plural.
3. The journal lists provisioning, lifecycle changes, default and lock changes, operator changes and
   raised sessions, newest first, with who did it.
4. Nothing on either screen names a workspace's hubs, tasks, comments or files
   ([NG-operator-reads-content](../../vision/non-goals.md)).
5. An operator workspace used only to hold a service account is not counted as a customer.

## Where it ends

* No per-workspace activity statistics; usage for billing is UC-INS-14.

## Today

* Check 2: not met — with an empty register the overview says "0 accounts may run this installation" to the person who runs it, without a plural form, tracked in #1063.
* Check 5: not met — the bootstrap's `operator` workspace is counted, and can be suspended or deleted, tracked in #1060.
