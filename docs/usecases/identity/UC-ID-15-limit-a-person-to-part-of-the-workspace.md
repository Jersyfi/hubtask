---
id: UC-ID-15
title: Let a person see only part of the workspace
context: identity
actors: [PE-owner, PE-admin, PE-guest, PE-child]
deployments: [D2, D3, D4, D6]
serves: [P-01, P-05]
state: partial
tasks: [B-02]
checked_by: [core/domain/service/Authorization_test.go]
---

# Let a person see only part of the workspace

## Goal

A child sees only their own hub, a client sees only their project, a craftsman sees only the one
task they were asked to do — and nothing else of the workspace shows, not even that it exists.

## Story

The administrator gives Lena *Contributor* on the hub *Lena's school*. Lena signs in and sees one
hub. The list of hubs does not hint at others; a link to someone else's task tells her it was not
found.

## How to check

1. A person with a membership on one hub sees exactly the hubs they have a membership on, with no
   error and no placeholder for the others.
2. An entry outside their reach answers "not found", not "forbidden".
3. A Contributor can change only entries assigned to them; what they create is assigned to them.
4. A Viewer can read and not change; a Guest on one task sees that task only.
5. Navigation, search, the overview and the calendar feed show only what the person may see.

## Where it ends

* Rights only add up; nothing *removes* access that a higher role grants. Keeping a hub private
  from an administrator is UC-ID-16.

## Today

* Check 1: not met in the web app — an administrator cannot give a membership on one hub there: the invitation grants the whole workspace, and the members dialog offers only people who already hold a membership along the path, tracked in #1079.
* Check 2: not met in the web app — nobody can be limited to part of the workspace from the screens (check 1), so nothing is outside a person's reach, tracked in #1079.
* Check 3: not met in the web app — a Contributor cannot be given one hub from the screens (check 1), tracked in #1079.
* Check 4: not met in the web app — a Viewer or a Guest cannot be given one hub or one task from the screens (check 1), tracked in #1079.
* Check 5: not met in the web app — nobody can be limited from the screens (check 1), tracked in #1079.
