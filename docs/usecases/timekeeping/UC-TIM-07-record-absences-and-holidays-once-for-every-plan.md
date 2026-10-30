---
id: UC-TIM-07
title: Record absences and holidays once for every plan
context: timekeeping
actors: [PE-member, PE-planner, PE-integrator, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-14, P-18]
state: partial
tasks: []
checked_by: [core/domain/model/timekeeping/Resources_test.go, core/application/service/timekeeping/Resources_test.go, test/integration/time_resources_test.go]
---

# Record absences and holidays once for every plan

## Goal

A person records when they are away once, and every plan's workload counts it; the team sees a
shared calendar of who is away when - by a kind, never a reason. Public holidays are the business
calendar's, recorded once for everybody.

## Story

Ana records her leave from 9 to 13 November. Ben's calendar shows her away that week as *Leave*;
the workload of every hub she works in gives her no minutes then. When she falls ill she records
*Other*: the team sees that she is away, and nothing about why.

## How to check

1. A person records a span of at most a year they are away - LEAVE, TRAINING or OTHER, wholly or
   half of each day; another person's absence needs `MANAGE_MEMBERS` at the workspace.
2. A span sharing a day with another of the same person's absences is refused
   (`time.absence_overlaps`); an unknown kind is refused (`time.absence_kind_unknown`).
3. Every member reads the absences of a range: who, from when to when, half or whole, and the kind
   - no other field.
4. A person changes or removes their absence, held to the version read.
5. Absences stay in their workspace.
6. REST, MCP and hubctl record and read absences alike.
7. The web app shows the shared calendar of absences by month, and a person records, changes and
   removes their own there.

## Today

* Check 7: not met - the web app shows no calendar of absences yet (S-20).

## Where it ends

* There is no leave balance, entitlement or approval of leave: that is a human resources system's.
* An absence never says why beyond its kind (GDPR Art. 9).
