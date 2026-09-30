---
id: UC-PRV-04
title: Restrict the processing of a person's data
context: privacy
actors: [PE-admin, PE-owner, PE-scripter]
deployments: [D3, D4, D5, D6, D7]
serves: [P-08, P-11, P-14]
state: partial
tasks: [E-10, F4-20]
checked_by: [core/application/service/privacy/Restriction_test.go, test/integration/privacy_test.go]
---

# Restrict the processing of a person's data

## Goal

While a dispute about a person's data is open, the workspace stops acting on that person
automatically — no automatic assignment, no automation, no AI — and the person can still sign in
and work.

## Story

A colleague contests the accuracy of what is recorded about them. The administrator restricts their
account in *Data subject requests*, giving a reason. The colleague keeps working as before, but
nothing automatic acts on them any more: the auto-assignment passes them over, rules do not act on
their entries, no model is asked about them. When the dispute is settled, the administrator lifts
the restriction.

## How to check

1. An administrator restricts an account with a reason, and lifts the restriction again, in the web
   app, through the API, with `hubctl` and through MCP.
2. A restricted person can still sign in, read and change what they could before.
3. Automatic assignment never assigns an entry to a restricted person; the entry stays unassigned.
4. Automation rules do not act on behalf of, or assign to, a restricted person.
5. No AI suggestion is requested for an entry assigned to, or created by, a restricted person.
6. Restricting and lifting write `dsr.processing_restricted` and `dsr.processing_resumed`.
7. A plain member cannot restrict anybody.

## Where it ends

* A restriction is not a lock: sign-in, reading and editing continue (Art. 18 restricts the
  controller, not the person).
* No hiding of the person's data from others in the workspace.
* A *restriction* case closes when the restriction is in place; the restriction itself goes on
  standing.

See [data-protection.md](../../architecture/data-protection.md) §4.

## Today

* **Check 1 fails in part.** The web app can restrict but not lift (`PrivacyView.svelte`), and
  `hubctl` has no command for either (`cmd/hubctl/Privacy.go`).
* **Checks 4 and 5 fail.** Only automatic assignment asks whether processing is allowed
  (`core/application/service/work/AutoAssignWorkItem.go`, `withoutRestricted`); the automation
  engine and the suggestion service do not, although the screen's note
  (`app.privacy.restrict_note`) says they do.
