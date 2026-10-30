---
id: UC-AGT-04
title: Evaluate an agent before it goes live
context: agent
actors: [PE-admin, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-18]
state: partial
tasks: []
checked_by: [core/domain/model/agent/Agent_test.go, core/application/service/agent/Agent_test.go, test/integration/agent_repository_test.go]
---

# Evaluate an agent before it goes live

## Goal

An administrator keeps a set of golden cases for an agent - a ticket's words and what the agent
should answer - and the agent goes live only once a run of that set on its model as it now stands
reached the bar the administrator set; the answers are scored by the workspace, not by whoever sends
them.

## Story

The admin writes forty past tickets with the queue and labels each should get and sets the bar at
90 %. *Triage* answers them before it is live; it reaches 92 % and the admin puts it live. After the
model changes, it must pass again.

## How to check

1. A case holds a title, notes and the expected answer - any keys, a list compared as a set; a set
   holds at most 500 cases.
2. Recording a run compares each answer with its case's expectation; a case left unanswered fails;
   the run keeps the cases, the passed count and the model revision, never an answer.
3. An agent with a bar goes live only when its newest run is on its model's present revision and
   passed at least that share (`agents.evaluation_required` otherwise).
4. The agent itself reads its set and records its run before it is live; an administrator may too.
5. Cases and runs stay in their workspace and join its archive.
6. The web app shows an agent's set and runs and lets an administrator add and remove cases.

## Where it ends

* The shipped agents do not run their set themselves yet; a run is recorded from their answers.
* No accuracy, escalation or override rates measured in production.

## Today

* Check 6: the web app's agents screens are not built (S-22).
