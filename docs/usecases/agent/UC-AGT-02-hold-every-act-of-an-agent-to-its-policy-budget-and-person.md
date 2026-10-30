---
id: UC-AGT-02
title: Hold every act of an agent to its policy, its budget and the person it acts for
context: agent
actors: [PE-admin, PE-member, PE-agent, PE-auditor]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-18]
state: built
tasks: []
checked_by: [core/domain/model/agent/Agent_test.go, core/application/service/agent/Agent_test.go, core/application/service/access/Delegation_test.go, core/application/service/agent/Runner_test.go, core/application/service/notification/RecordAgentNotice_test.go, test/integration/agent_repository_test.go, apps/webapp/src/lib/data/agents.test.ts]
---

# Hold every act of an agent to its policy, its budget and the person it acts for

## Goal

Whatever door a registered agent comes through, every act it attempts is judged before it runs: it
never does what the person it acts for may not, a read runs, a write runs, runs with its owner told,
or waits for a person by its policy, an act that cannot be taken back always waits or is never done,
its budget and the workspace's kill switch stop it, and every act is in its ledger.

## Story

*Triage* reads a ticket and proposes to move it; the move waits. Ana sees it among the agent's
requests with what it would change, approves it, and the move happens as hers. At the hundred and
twenty-first write of the hour the agent is told its budget renews at the full hour. During an
incident the admin halts every agent of the workspace at once.

## How to check

1. A registered agent's act passes the gate on the registry every door comes through - REST, MCP,
   hubctl, a rule's action - and an actor that is no registered agent passes untouched.
2. An admitted act runs as the agent with its scopes narrowed to those it may use and the person it
   acts for named; the authoriser answers what both may, so it neither does nor lists more than that
   person, and a refusal for that reason is recorded `denied_by: delegator`.
3. A read always runs; a reversible write runs, runs with its owner told, waits or is never done by
   the agent's policy; a destructive act or one asking a fresh sign-in waits or is never done.
4. An act that waits is answered `agents.approval_required` with its request's identifier and stored
   with its input, never with a proof of a fresh sign-in.
5. Approving performs it as the approving person with their rights - the agent's owner or an
   administrator, never an agent -, once; one their rights refuse is recorded `FAILED` with the
   refusal's code; declining performs nothing.
6. A suspended agent's acts are refused with `agents.suspended`, every agent's with `agents.halted`
   while the workspace's kill switch is on.
7. A write beyond the agent's actions per hour is refused with `agents.budget_spent` and when it
   renews.
8. Every act is a row of the agent's ledger - its use case, class, decision and code, the tokens a
   run spent - and never its input or answer; the metering sums them per agent and day.
9. The web app shows an agent's requests with what they would change for approval, its ledger and
   the metering.

## Where it ends

* *Runs with its owner told* tells the owner by mail at most once an hour; what the agent did is its
  ledger, not the mail.
* The traces of a run - prompts, tool calls, answers - are not kept: the ledger meters, it does not
  trace, and their retention is the owner's decision.
* A held act waits until somebody decides it; it does not expire.
