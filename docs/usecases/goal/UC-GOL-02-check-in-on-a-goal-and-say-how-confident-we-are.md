---
id: UC-GOL-02
title: Check in on a goal and say how confident we are
context: goal
actors: [PE-planner, PE-member, PE-admin, PE-integrator, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-11, P-18]
state: built
tasks: []
checked_by: [core/domain/model/goal/Goal_test.go, core/application/service/goal/Goal_test.go, test/integration/goal_repository_test.go, apps/webapp/src/lib/data/goals.test.ts]
---

# Check in on a goal and say how confident we are

## Goal

Whoever works on a goal says from time to time how it stands, how confident they are and where its
key results are, the history of those check-ins stays, and when the period closes every goal gets a
grade that no longer changes.

## Story

Ana checks in on *Grow*: at risk, confidence 4 of 10, revenue at 3 of 10, "slow quarter". The goal
now shows at risk with 30 % progress, and the check-in is the first line of its history. When the
admin closes *2026 Q4*, *Grow* is graded 0.3, its progress, because nobody gave it another grade;
a check-in on it afterwards is refused.

## How to check

1. A check-in gives a status (on track, at risk, off track, done or dropped), a confidence from 0 to
   10, an optional note and new values for the key results reported by hand; a value for a key result
   fed by a view or a milestone is refused (`check_ins.value_not_manual`).
2. The goal takes the check-in's status, confidence and values; the check-ins are listed the latest
   first and none changes.
3. Closing a period (`STRUCTURE` at the workspace) grades each of its goals 0 to 1 - the grade given
   for it, else its progress as the closer sees it - and a graded goal takes no check-in and no change
   (`goals.graded`).
4. A check-in's note never reaches the audit trail.
5. REST, MCP and hubctl check in and list check-ins alike.
6. The web app checks in on a goal from its page and shows its history.

## Where it ends

* No reminders to check in and no digest to stakeholders.
* No AI summary of check-ins.
