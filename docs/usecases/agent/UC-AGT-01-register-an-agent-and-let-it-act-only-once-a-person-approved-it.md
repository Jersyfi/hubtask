---
id: UC-AGT-01
title: Register an agent and let it act only once a person approved it
context: agent
actors: [PE-admin, PE-member, PE-agent, PE-auditor]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-18]
state: built
tasks: []
checked_by: [core/domain/model/agent/Agent_test.go, core/application/service/agent/Agent_test.go, test/integration/agent_repository_test.go, apps/webapp/src/lib/data/agents.test.ts]
---

# Register an agent and let it act only once a person approved it

## Goal

An administrator registers an agent - one of the agents the product ships or one built elsewhere -
as a principal of its own acting for a named person, says what it is for, what it may use and what
it may do without a person, and nothing it attempts runs until somebody other than that person
approved it and put it live; its owner or an administrator stops it at once.

## Story

The admin registers *Triage* for Ana: sorting new tickets, the scopes `items:read` and
`items:write`, the workspace's local model. It is a draft. Ana cannot approve it herself; the admin
does and puts it live. When it misbehaves, Ana pulls its switch and it stops mid-afternoon.

## How to check

1. Registering an agent creates a service account it acts as and stores it as a draft with its
   purpose, kind, owner, risk class, scopes, model, policy and budget; it needs `MANAGE_MEMBERS` at
   the workspace, and the trail records it without its purpose.
2. A risk class `HIGH` is refused with `agents.high_risk_not_registrable`; an agent without a
   purpose or an owner is refused.
3. The person an agent acts for does not approve it (`agents.owner_approves_not`), and no agent
   approves or puts live an agent.
4. A draft's acts are refused with `agents.not_active`; an approved agent goes live by
   `ActivateAgent`.
5. A change that lets an agent do more - wider scopes, another model, a looser rule, a lower bar,
   another owner, kind or risk class - takes it back to a draft; a smaller budget does not.
6. Its owner or an administrator suspends it, and from then every act is refused with
   `agents.suspended`; a retired agent never acts again.
7. Agents stay in their workspace and join its archive and its access export.
8. REST, MCP and hubctl register, read, approve, put live, stop and retire agents alike.
9. The web app's administration lists the agents with their state and lets an administrator
   approve, put live, stop and retire them.

## Where it ends

* No agent decides about people: an agent of the high-risk class is not registered at all.
* An agent's account holds roles and tokens as any service account; registering grants it none.
* No agent-to-agent protocol and no orchestration of several agents.
