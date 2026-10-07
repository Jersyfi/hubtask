---
id: UC-AUD-06
title: See what the workspace recorded about me
context: audit
actors: [PE-member]
deployments: [D3, D4, D6]
serves: [P-05, P-08, P-11]
state: partial
tasks: [E-09]
checked_by: [core/application/service/audit/List_test.go]
---

# See what the workspace recorded about me

## Goal

An employee or a club member can see the trail entries the workspace keeps about their own actions
— the same record their administrator can read — without being able to see anybody else's.

## Story

A colleague wonders what the company can see about their work. In the web app they open a view of
their own trail and see the entries with themselves as the actor: when they signed in, what they
created, moved, deleted, which requests were refused. Nobody else's entries are there, and there is
no way to widen the view.

## How to check

1. A member without an administrator's or auditor's role who reads the trail is answered only the
   entries they are the actor of, whatever filter they send.
2. A member who names a colleague as the actor is refused with `access.not_permitted`, not answered
   an empty list.
3. A member whose only membership is on a hub, not on the workspace, can read their own entries.
4. The web app gives a member a place to read their own entries.
5. The same is available with `hubctl audit query` and through MCP for the member's own token.

## Where it ends

* No entries about the member written by somebody else (an administrator changing their role);
  those are the administrator's actions and are in the administrator's view.
* No export of one's own entries; the export is a period of the whole trail. A person who wants a
  copy of everything about them asks for it as a data subject request.

## Today

* Check 3: not met — reading one's own entries asks for read access at the workspace, which a member whose only membership is on a hub does not hold, so they are refused.
* Check 4: not met — the only audit screen is in the administration area, which the web app shows only to holders of a configuration right.
