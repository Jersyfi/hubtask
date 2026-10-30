---
id: UC-AGT-04
title: Evaluate an agent before it goes live
context: agent
actors: [PE-admin, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-18]
state: built
tasks: []
checked_by: [core/domain/model/agent/Agent_test.go, core/application/service/agent/Agent_test.go, test/integration/agent_repository_test.go, apps/webapp/src/lib/data/agents.test.ts, core/application/service/suggestion/CaseLabels_test.go]
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
7. An administrator has a shipped agent answer its own set (`RunAgentEvaluation`) from each case's
   title and notes: a knowledge draft answers `title` and `body`, a CI suggestion and a triage `cis`,
   the names of the chosen hub's CIs the text names (`agents.evaluation_hub_required` without a
   hub); a duplicate review is evaluated by answers somebody sends (`agents.evaluation_by_answers`).
   REST, MCP, hubctl and the web app start it alike.
8. Given a collection, a ticket triage answers `labels` for each case expecting them: the names of
   the collection's labels the workspace's classifier chooses for the case's title and notes, asked
   as the person running the set and recorded as no suggestion; at most 50 such cases a run
   (`agents.evaluation_labels_too_many`), and AI switched off refuses the run as asking would.
   Without a collection no labels are answered and those cases fail.

## Where it ends

* A triage's own run answers labels and CIs; anything else a case expects is recorded from answers
  somebody sends.
* No accuracy, escalation or override rates measured in production.
