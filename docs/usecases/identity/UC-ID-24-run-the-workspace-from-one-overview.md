---
id: UC-ID-24
title: Run the workspace from one overview
context: identity
actors: [PE-owner, PE-admin, PE-auditor]
deployments: [D2, D3, D4, D5, D6, D7]
serves: [P-05, P-06, P-11]
state: specified
tasks: []
checked_by: []
---

# Run the workspace from one overview

## Goal

An administrator opening the administration sees first what needs attention — limits close to their ceiling, rules failing, backups not run, requests owed — and reaches every setting in at most two steps.

## Story

The club's admin opens *Administration*: one card says a rule has failed three times, another that people are at 28 of 30. Each card links to its page.

## How to check

1. *Administration* opens on an overview, not on the first settings page.
2. The overview shows the workspace's limits with use and ceiling, rules that fail or need attention, the last backup and whether it opened, privacy requests owed soon, and degraded features, each with a link to its page.
3. Every administration page is reachable from the navigation in at most two steps and carries a breadcrumb.
4. Choosing a person in the trail, in privacy requests and in exports is done by picking from the people of the workspace, not by typing an identifier.
5. An auditor sees the overview read-only.

## Where it ends

* The installation's overview stays [UC-INS-12](../admin/UC-INS-12-see-the-installation-at-a-glance.md).

## Today

* Check 1: not met — not built yet, tracked in milestone UX.
* Check 2: not met — not built yet, tracked in milestone UX.
* Check 3: not met — not built yet, tracked in milestone UX.
* Check 4: not met — not built yet, tracked in milestone UX.
* Check 5: not met — not built yet, tracked in milestone UX.
