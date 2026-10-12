---
id: UC-INS-09
title: Decide defaults and locks for every workspace
context: admin
actors: [PE-operator, PE-selfhoster]
deployments: [D1, D4, D5, D6, D7]
serves: [P-06, P-07, P-08, P-10, P-12]
state: verified
tasks: [SI-05, SI-12, SI-17, SC-07]
checked_by: [core/application/service/admin/InstanceLevels_test.go, core/application/service/admin/Instance_test.go, core/application/service/admin/InstanceDoors_test.go, test/integration/instance_values_test.go, cmd/hubctl/AdminInstance_test.go, apps/webapp/e2e/instancesettings.test.mjs, apps/webapp/e2e/settings.test.mjs, docs/evidence/SC-07-2026-10-12.md, apps/webapp/e2e/quotas.test.mjs, test/integration/quota_test.go, core/application/service/quota/Quota_test.go, docs/evidence/SC-15-D1-2026-10-13.md]
---

# Decide defaults and locks for every workspace

## Goal

An operator sets, once, what every workspace starts with — sign-in rules, legal links, limits,
language — and decides for each value whether workspaces may change it, on a screen that reads
like the workspace's own settings.

## Story

*Installation → Defaults* has the same groups and the same controls as a workspace's *Sign-in*
screen: numbers are number fields, switches are switches, choices are choices. Beside each value:
*Workspaces may* — "make it stricter" or "not change it" (for limits: "go higher" or "not go
higher"; for language: nothing, because a language is never locked). Saving asks for a fresh proof
and lands in the instance journal.

## How to check

1. Every value is edited with a control of its kind, labelled in words — never a raw key such as
   `sign_in.min_length` and never a text box for a number or a list.
2. Every value can be saved from the screen, and a saved number is stored as a number.
3. The lock column says in words what a workspace may still do, specific to the kind of value.
4. Language, time zone and week start take a default and never a lock.
5. A workspace's screen shows each of these values as "set by the installation" where locked, and
   as the default where not.
6. The same values can be read and written with `hubctl admin settings` and through the instance
   file, with the same refusals.
7. In `enforce` mode, the screen shows the values and the file's path, and offers no control.

## Where it ends

* Plans, which carry the same values for a group of workspaces, are UC-INS-13.
* Rate limits and the lockout curve stay in the environment; they are not defaults.
